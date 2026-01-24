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
	customlogger "github.com/himanishpuri/AcousticDNA/pkg/logger"
	"github.com/himanishpuri/AcousticDNA/pkg/models"
	"github.com/himanishpuri/AcousticDNA/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const DefaultDBFile = "acousticdna.sqlite3"
const errDBClientNil = "db client is nil"

type DBClient struct {
	DB *gorm.DB
	db *sql.DB
}

type Song struct {
	ID         string `gorm:"primaryKey;type:varchar(36)"`
	Title      string `gorm:"uniqueIndex:idx_song_unique,priority:1;index:idx_song_meta,priority:1" json:"title"`
	Artist     string `gorm:"uniqueIndex:idx_song_unique,priority:2;index:idx_song_meta,priority:2" json:"artist"`
	YouTubeID  string `gorm:"index:idx_youtube_id" json:"youtube_id"`
	SpotifyID  string `gorm:"index:idx_spotify_id" json:"spotify_id"`
	DurationMs int    `json:"duration_ms"`
	CreatedAt  time.Time
}

type Fingerprint struct {
	ID           uint   `gorm:"primaryKey;autoIncrement"`
	Hash         uint32 `gorm:"index:idx_hash" json:"hash"`
	SongID       string `gorm:"type:varchar(36);index:idx_song" json:"song_id"`
	AnchorTimeMs uint32 `json:"anchor_time_ms"`
}

func NewDBClient() (*DBClient, error) {
	dbPath := os.Getenv("ACOUSTIC_DB_PATH")
	if dbPath == "" {
		dbPath = DefaultDBFile
	}
	return NewDBClientWithPath(dbPath)
}

func NewDBClientWithPath(dbPath string) (*DBClient, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil && !os.IsExist(err) {
		if filepath.Dir(dbPath) != "." {
			return nil, fmt.Errorf("creating db dir: %w", err)
		}
	}

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

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(&Song{}, &Fingerprint{}); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	return &DBClient{DB: db, db: sqlDB}, nil
}

func (c *DBClient) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *DBClient) RegisterSong(title, artist, youtubeID string, durationMs int) (string, error) {
	if c == nil || c.DB == nil {
		return "", errors.New(errDBClientNil)
	}

	var song Song

	err := c.DB.Where("title = ? AND artist = ?", title, artist).First(&song).Error
	if err == nil {
		if song.YouTubeID == "" && youtubeID != "" {
			if err := c.DB.Model(&song).Update("YouTubeID", youtubeID).Error; err != nil {
				return "", fmt.Errorf("updating youtube_id: %w", err)
			}
			song.YouTubeID = youtubeID
		}
		return song.ID, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("querying existing song: %w", err)
	}

	uuid := utils.GenerateUUID()
	song = Song{ID: uuid, Title: title, Artist: artist, YouTubeID: youtubeID, DurationMs: durationMs}
	err = c.DB.Create(&song).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) ||
			(err.Error() != "" && (strings.Contains(err.Error(), "UNIQUE constraint failed") ||
				strings.Contains(err.Error(), "constraint failed"))) {
			if fetchErr := c.DB.Where("title = ? AND artist = ?", title, artist).First(&song).Error; fetchErr != nil {
				return "", fmt.Errorf("fetching song after constraint violation: %w", fetchErr)
			}
			return song.ID, nil
		}
		return "", fmt.Errorf("creating song: %w", err)
	}

	return song.ID, nil
}

func (c *DBClient) DeleteSongByID(songID string) error {
	if c == nil || c.DB == nil {
		return errors.New(errDBClientNil)
	}
	return c.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("song_id = ?", songID).Delete(&Fingerprint{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", songID).Delete(&Song{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (c *DBClient) StoreFingerprints(fp map[uint32][]models.Couple) error {
	if c == nil || c.DB == nil {
		return errors.New(errDBClientNil)
	}

	entries := make([]Fingerprint, 0, 1024)
	for hash, couples := range fp {
		for _, cou := range couples {
			entries = append(entries, Fingerprint{
				Hash:         hash,
				SongID:       cou.SongID,
				AnchorTimeMs: uint32(cou.AnchorTimeMs),
			})
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

func (c *DBClient) GetCouplesByHash(hash uint32) ([]models.Couple, error) {
	if c == nil || c.DB == nil {
		return nil, errors.New(errDBClientNil)
	}
	var rows []Fingerprint
	if err := c.DB.Where("hash = ?", hash).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("querying fingerprints: %w", err)
	}
	out := make([]models.Couple, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.Couple{SongID: r.SongID, AnchorTimeMs: r.AnchorTimeMs})
	}
	return out, nil
}

func (c *DBClient) GetCouplesByHashes(hashes []uint32) (map[uint32][]models.Couple, error) {
	if c == nil || c.DB == nil {
		return nil, errors.New(errDBClientNil)
	}
	if len(hashes) == 0 {
		return make(map[uint32][]models.Couple), nil
	}

	hashesInterface := make([]interface{}, len(hashes))
	for i, h := range hashes {
		hashesInterface[i] = h
	}

	var rows []Fingerprint
	if err := c.DB.Where("hash IN ?", hashesInterface).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("batch querying fingerprints: %w", err)
	}

	result := make(map[uint32][]models.Couple)
	for _, r := range rows {
		result[r.Hash] = append(result[r.Hash], models.Couple{
			SongID:       r.SongID,
			AnchorTimeMs: r.AnchorTimeMs,
		})
	}

	return result, nil
}

// QueryTopMatches is a convenience wrapper that fetches all couple lists for query hashes and
// performs in-memory voting. It expects queryHashes in the same packed form your hash.go creates.
// This mirrors earlier QueryFingerprints logic but uses the DB for bucket lookup.
func (c *DBClient) QueryTopMatches(queryHashes []uint32) ([]models.Match, error) {
	// Efficient lookup: fetch buckets for each hash and perform voting
	votes := make(map[string]map[int32]int)

	for _, h := range queryHashes {
		var rows []Fingerprint
		if err := c.DB.Where("hash = ?", h).Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("querying hash %d: %w", h, err)
		}
		for _, r := range rows {
			// We cannot compute offset here because we need query anchor times.
			// This helper assumes calling code tracks query anchor times and computes offsets.
			// For convenience, we return a map-like structure via GetCouplesByHash in the other method.
			m := votes[r.SongID]
			if m == nil {
				m = make(map[int32]int)
				votes[r.SongID] = m
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
