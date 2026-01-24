package acousticdna

import (
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/model"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/storage"
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

func (s *storageAdapter) RegisterSong(title, artist, youtubeID string, durationMs int) (uint32, error) {
	return s.db.RegisterSong(title, artist, youtubeID, durationMs)
}

func (s *storageAdapter) StoreFingerprints(fingerprints map[uint32][]model.Couple) error {
	// Convert Couple to model.Couple
	modelFPs := make(map[uint32][]model.Couple)
	for hash, couples := range fingerprints {
		modelCouples := make([]model.Couple, len(couples))
		for i, c := range couples {
			modelCouples[i] = model.Couple{
				SongID:       c.SongID,
				AnchorTimeMs: c.AnchorTimeMs,
			}
		}
		modelFPs[hash] = modelCouples
	}
	return s.db.StoreFingerprints(modelFPs)
}

func (s *storageAdapter) GetCouplesByHash(hash uint32) ([]model.Couple, error) {
	modelCouples, err := s.db.GetCouplesByHash(hash)
	if err != nil {
		return nil, err
	}

	// Convert model.Couple to Couple
	couples := make([]model.Couple, len(modelCouples))
	for i, mc := range modelCouples {
		couples[i] = model.Couple{
			SongID:       mc.SongID,
			AnchorTimeMs: mc.AnchorTimeMs,
		}
	}
	return couples, nil
}

func (s *storageAdapter) DeleteSongByID(songID uint32) error {
	return s.db.DeleteSongByID(songID)
}

func (s *storageAdapter) GetSongByID(songID uint32) (*Song, error) {
	var dbSong storage.Song
	if err := s.db.DB.First(&dbSong, songID).Error; err != nil {
		return nil, err
	}

	return &Song{
		ID:         uint32(dbSong.ID),
		Title:      dbSong.Title,
		Artist:     dbSong.Artist,
		YouTubeID:  dbSong.YouTubeID,
		DurationMs: dbSong.DurationMs,
	}, nil
}

func (s *storageAdapter) GetFingerprintCount(songID uint32) (int, error) {
	var count int64
	if err := s.db.DB.Model(&storage.Fingerprint{}).Where("song_id = ?", songID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *storageAdapter) ListSongs() ([]Song, error) {
	var dbSongs []storage.Song
	if err := s.db.DB.Find(&dbSongs).Error; err != nil {
		return nil, err
	}

	songs := make([]Song, len(dbSongs))
	for i, dbSong := range dbSongs {
		songs[i] = Song{
			ID:         uint32(dbSong.ID),
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
