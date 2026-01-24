//go:build !js && !wasm
// +build !js,!wasm

package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/model"
	customlogger "github.com/himanishpuri/AcousticDNA/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Default DB file name (can be overridden with env var ACOUSTIC_DB_PATH)
const DefaultDBFile = "acousticdna.sqlite3"

// Error messages
const errDBClientNil = "db client is nil"

// DBClient wraps a GORM DB handle.
type DBClient struct {
	DB *gorm.DB
	db *sql.DB // underlying sql.DB for Close
}

// Song model stores canonical metadata and external IDs.
type Song struct {
	ID         uint   `gorm:"primaryKey;autoIncrement"`
	Title      string `gorm:"uniqueIndex:idx_song_unique,priority:1;index:idx_song_meta,priority:1" json:"title"`
	Artist     string `gorm:"uniqueIndex:idx_song_unique,priority:2;index:idx_song_meta,priority:2" json:"artist"`
	YouTubeID  string `gorm:"index:idx_youtube_id" json:"youtube_id"`
	SpotifyID  string `gorm:"index:idx_spotify_id" json:"spotify_id"`
	DurationMs int    `json:"duration_ms"`
	CreatedAt  time.Time
}

// Fingerprint row: a hash entry pointing to a song and an anchor time (ms).
// We index on `Hash` for fast lookup by query.
type Fingerprint struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	Hash         uint32 `gorm:"index:idx_hash" json:"hash"`
	SongID       uint   `gorm:"index:idx_song" json:"song_id"`
	AnchorTimeMs uint32 `json:"anchor_time_ms"`
}

// NewDBClient opens (or creates) the SQLite database, runs migrations and returns a client.
// If the env var ACOUSTIC_DB_PATH is set, it will use that path, otherwise it uses DefaultDBFile.
func NewDBClient() (*DBClient, error) {
	dbPath := os.Getenv("ACOUSTIC_DB_PATH")
	if dbPath == "" {
		dbPath = DefaultDBFile
	}
	return NewDBClientWithPath(dbPath)
}

// NewDBClientWithPath opens (or creates) the SQLite database at the specified path.
func NewDBClientWithPath(dbPath string) (*DBClient, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil && !os.IsExist(err) {
		// if DefaultDBFile is in working dir, Dir() == "." which is fine
		if filepath.Dir(dbPath) != "." {
			return nil, fmt.Errorf("creating db dir: %w", err)
		}
	}

	// Use a quiet logger unless debug is required
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	db, err := gorm.Open(sqlite.Open(dbPath+"?_foreign_keys=on"), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting sql.DB from gorm: %w", err)
	}

	// Tune connection pool for concurrency (safe defaults for local use)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Auto-migrate schema
	if err := db.AutoMigrate(&Song{}, &Fingerprint{}); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &DBClient{DB: db, db: sqlDB}, nil
}

// Close closes underlying DB connection.
func (c *DBClient) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// RegisterSong inserts a new song record and returns the generated song ID.
// If a song with the same title+artist already exists, it returns the existing ID (idempotent-ish).
// This function is safe for concurrent use thanks to unique constraint on title+artist.
func (c *DBClient) RegisterSong(title, artist, youtubeID string, durationMs int) (uint32, error) {
	if c == nil || c.DB == nil {
		return 0, errors.New(errDBClientNil)
	}

	var song Song

	// Try to find existing song first
	err := c.DB.Where("title = ? AND artist = ?", title, artist).First(&song).Error
	if err == nil {
		// Song exists, update YouTubeID if needed
		if song.YouTubeID == "" && youtubeID != "" {
			if err := c.DB.Model(&song).Update("YouTubeID", youtubeID).Error; err != nil {
				return 0, fmt.Errorf("updating youtube_id: %w", err)
			}
			song.YouTubeID = youtubeID
		}
		return uint32(song.ID), nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("querying existing song: %w", err)
	}

	// Song doesn't exist, try to create it
	song = Song{Title: title, Artist: artist, YouTubeID: youtubeID, DurationMs: durationMs}
	err = c.DB.Create(&song).Error
	if err != nil {
		// Check if error is due to unique constraint violation (concurrent insert)
		// If so, retry the lookup to get the song created by another goroutine
		if errors.Is(err, gorm.ErrDuplicatedKey) ||
			(err.Error() != "" && (strings.Contains(err.Error(), "UNIQUE constraint failed") ||
				strings.Contains(err.Error(), "constraint failed"))) {
			// Another goroutine created it, fetch again
			if fetchErr := c.DB.Where("title = ? AND artist = ?", title, artist).First(&song).Error; fetchErr != nil {
				return 0, fmt.Errorf("fetching song after constraint violation: %w", fetchErr)
			}
			return uint32(song.ID), nil
		}
		return 0, fmt.Errorf("creating song: %w", err)
	}

	return uint32(song.ID), nil
}

// DeleteSongByID deletes a song and all its fingerprints in a transaction.
func (c *DBClient) DeleteSongByID(songID uint32) error {
	if c == nil || c.DB == nil {
		return errors.New(errDBClientNil)
	}
	return c.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("song_id = ?", songID).Delete(&Fingerprint{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&Song{}, songID).Error; err != nil {
			return err
		}
		return nil
	})
}

// StoreFingerprints stores a map[hash][]audio.Couple into the database in batches.
// It is efficient and uses GORM CreateInBatches under the hood.
func (c *DBClient) StoreFingerprints(fp map[uint32][]model.Couple) error {
	if c == nil || c.DB == nil {
		return errors.New(errDBClientNil)
	}

	// Prepare slices for batch insertion
	entries := make([]Fingerprint, 0, 1024)
	for hash, couples := range fp {
		for _, cou := range couples {
			entries = append(entries, Fingerprint{
				Hash:         hash,
				SongID:       uint(cou.SongID),
				AnchorTimeMs: uint32(cou.AnchorTimeMs),
			})
			// flush periodically to avoid huge memory spikes
			if len(entries) >= 1000 {
				if err := c.DB.CreateInBatches(entries, 500).Error; err != nil {
					return fmt.Errorf("batch insert fingerprints: %w", err)
				}
				entries = entries[:0]
			}
		}
	}
	if len(entries) > 0 {
		if err := c.DB.CreateInBatches(entries, 500).Error; err != nil {
			return fmt.Errorf("batch insert last fingerprints: %w", err)
		}
	}
	return nil
}

// GetCouplesByHash returns a slice of model.Couple for a given hash from DB.
func (c *DBClient) GetCouplesByHash(hash uint32) ([]model.Couple, error) {
	if c == nil || c.DB == nil {
		return nil, errors.New(errDBClientNil)
	}
	var rows []Fingerprint
	if err := c.DB.Where("hash = ?", hash).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying fingerprints: %w", err)
	}
	out := make([]model.Couple, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.Couple{SongID: uint32(r.SongID), AnchorTimeMs: r.AnchorTimeMs})
	}
	return out, nil
}

// GetCouplesByHashes retrieves couples for multiple hashes in a single query.
// This is significantly more efficient than calling GetCouplesByHash in a loop
// as it uses a single SQL query with an IN clause.
func (c *DBClient) GetCouplesByHashes(hashes []uint32) (map[uint32][]model.Couple, error) {
	if c == nil || c.DB == nil {
		return nil, errors.New(errDBClientNil)
	}
	if len(hashes) == 0 {
		return make(map[uint32][]model.Couple), nil
	}

	// Convert []uint32 to []interface{} for GORM's IN clause
	hashesInterface := make([]interface{}, len(hashes))
	for i, h := range hashes {
		hashesInterface[i] = h
	}

	var rows []Fingerprint
	if err := c.DB.Where("hash IN ?", hashesInterface).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("batch querying fingerprints: %w", err)
	}

	// Group results by hash
	result := make(map[uint32][]model.Couple)
	for _, r := range rows {
		result[r.Hash] = append(result[r.Hash], model.Couple{
			SongID:       uint32(r.SongID),
			AnchorTimeMs: r.AnchorTimeMs,
		})
	}

	return result, nil
}

// QueryTopMatches is a convenience wrapper that fetches all couple lists for query hashes and
// performs in-memory voting. It expects queryHashes in the same packed form your hash.go creates.
// This mirrors earlier QueryFingerprints logic but uses the DB for bucket lookup.
func (c *DBClient) QueryTopMatches(queryHashes []uint32) ([]model.Match, error) {
	// Efficient lookup: fetch buckets for each hash and perform voting
	votes := make(map[uint32]map[int32]int)

	for _, h := range queryHashes {
		var rows []Fingerprint
		if err := c.DB.Where("hash = ?", h).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("querying hash %d: %w", h, err)
		}
		for _, r := range rows {
			// We cannot compute offset here because we need query anchor times.
			// This helper assumes calling code tracks query anchor times and computes offsets.
			// For convenience, we return a map-like structure via GetCouplesByHash in the other method.
			m := votes[uint32(r.SongID)]
			if m == nil {
				m = make(map[int32]int)
				votes[uint32(r.SongID)] = m
			}
			// placeholder: we don't have query time here; caller should compute offsets.
			// store counts keyed by offset if computed in caller.
			_ = m
		}
	}
	// This function is intentionally left limited — prefer using GetCouplesByHash and doing
	// the offset voting in your existing in-memory code for flexibility.
	return nil, fmt.Errorf("QueryTopMatches is a partial helper; use GetCouplesByHash + in-memory voting")
}

// Convenience: helper to build a DB client and log errors. Use utils.GetLogger if needed.
func MustNewDBClient() *DBClient {
	cli, err := NewDBClient()
	if err != nil {
		customlogger.GetLogger().Error("failed to open DB: %v", err)
		panic(err)
	}
	return cli
}
