package acousticdna

import (
	"context"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

type Service interface {
	AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (string, error)
	MatchSong(ctx context.Context, audioPath string) ([]models.MatchResult, error)
	MatchHashes(ctx context.Context, hashes map[uint32]uint32) ([]models.MatchResult, error)
	GetSongByID(songID string) (*models.Song, error)
	ListSongs() ([]models.Song, error)
	DeleteSong(songID string) error
	Close() error
}

type Storage interface {
	RegisterSong(title, artist, youtubeID string, durationMs int) (string, error)
	StoreFingerprints(fingerprints map[uint32][]models.Couple) error
	GetCouplesByHash(hash uint32) ([]models.Couple, error)
	GetCouplesByHashes(hashes []uint32) (map[uint32][]models.Couple, error)
	DeleteSongByID(songID string) error
	GetSongByID(songID string) (*models.Song, error)
	GetFingerprintCount(songID string) (int, error)
	ListSongs() ([]models.Song, error)
	Close() error
}

type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Debugf(format string, args ...any)
}
