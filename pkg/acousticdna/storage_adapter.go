//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

import (
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/storage"
	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

// storageAdapter adapts the storage.DBClient to implement the Storage interface.
type storageAdapter struct {
	db *storage.DBClient
}

// NewSQLiteStorage creates a new SQLite storage backend.
func NewSQLiteStorage(dbPath string) (Storage, error) {
	db, err := storage.NewDBClientWithPath(dbPath)
	if err != nil {
		return nil, err
	}
	return &storageAdapter{db: db}, nil
}

func (s *storageAdapter) RegisterSong(title, artist, youtubeID string, durationMs int) (string, error) {
	return s.db.RegisterSong(title, artist, youtubeID, durationMs)
}

func (s *storageAdapter) StoreFingerprints(fingerprints map[uint32][]models.Couple) error {
	// Convert Couple to models.Couple
	modelFPs := make(map[uint32][]models.Couple)
	for hash, couples := range fingerprints {
		modelCouples := make([]models.Couple, len(couples))
		for i, c := range couples {
			modelCouples[i] = models.Couple{
				SongID:       c.SongID,
				AnchorTimeMs: c.AnchorTimeMs,
			}
		}
		modelFPs[hash] = modelCouples
	}
	return s.db.StoreFingerprints(modelFPs)
}

func (s *storageAdapter) GetCouplesByHash(hash uint32) ([]models.Couple, error) {
	modelCouples, err := s.db.GetCouplesByHash(hash)
	if err != nil {
		return nil, err
	}

	// Convert models.Couple to Couple
	couples := make([]models.Couple, len(modelCouples))
	for i, mc := range modelCouples {
		couples[i] = models.Couple{
			SongID:       mc.SongID,
			AnchorTimeMs: mc.AnchorTimeMs,
		}
	}
	return couples, nil
}

func (s *storageAdapter) GetCouplesByHashes(hashes []uint32) (map[uint32][]models.Couple, error) {
	return s.db.GetCouplesByHashes(hashes)
}

func (s *storageAdapter) DeleteSongByID(songID string) error {
	return s.db.DeleteSongByID(songID)
}

func (s *storageAdapter) GetSongByID(songID string) (*models.Song, error) {
	var dbSong storage.Song
	if err := s.db.DB.Where("id = ?", songID).First(&dbSong).Error; err != nil {
		return nil, err
	}

	return &models.Song{
		ID:         dbSong.ID,
		Title:      dbSong.Title,
		Artist:     dbSong.Artist,
		YouTubeID:  dbSong.YouTubeID,
		DurationMs: dbSong.DurationMs,
	}, nil
}

func (s *storageAdapter) GetFingerprintCount(songID string) (int, error) {
	var count int64
	if err := s.db.DB.Model(&storage.Fingerprint{}).Where("song_id = ?", songID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *storageAdapter) ListSongs() ([]models.Song, error) {
	var dbSongs []storage.Song
	if err := s.db.DB.Find(&dbSongs).Error; err != nil {
		return nil, err
	}

	songs := make([]models.Song, len(dbSongs))
	for i, dbSong := range dbSongs {
		songs[i] = models.Song{
			ID:         dbSong.ID,
			Title:      dbSong.Title,
			Artist:     dbSong.Artist,
			YouTubeID:  dbSong.YouTubeID,
			DurationMs: dbSong.DurationMs,
		}
	}

	return songs, nil
}

func (s *storageAdapter) Close() error {
	return s.db.Close()
}
