package acousticdna

import (
	"context"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/model"
)

// Service defines the core AcousticDNA audio fingerprinting operations.
// It provides methods for adding songs, matching audio queries, and managing
// the song database.
type Service interface {
	// AddSong processes an audio file and stores its fingerprint in the database.
	// It returns the song ID on success.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - audioPath: Path to the audio file to process
	//   - title: Song title
	//   - artist: Artist name
	//   - youtubeID: YouTube video ID (optional, can be empty)
	//
	// Returns the database ID of the added song, or an error.
	AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (uint32, error)

	// MatchSong finds matches for a query audio file by comparing its fingerprint
	// against the database.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - audioPath: Path to the query audio file
	//
	// Returns a slice of match results sorted by confidence, or an error.
	MatchSong(ctx context.Context, audioPath string) ([]MatchResult, error)

	// GetSongByID retrieves a song's metadata by its database ID.
	GetSongByID(songID uint32) (*Song, error)

	// ListSongs returns all songs in the database.
	ListSongs() ([]Song, error)

	// DeleteSong removes a song and all its fingerprints from the database.
	DeleteSong(songID uint32) error

	// Close releases all resources held by the service (database connections, etc).
	// Should be called when the service is no longer needed.
	Close() error
}

// Storage defines the persistence layer interface for songs and fingerprints.
// Implementations must handle concurrent access safely.
type Storage interface {
	// RegisterSong adds or updates a song in the database.
	// Returns the song ID.
	RegisterSong(title, artist, youtubeID string, durationMs int) (uint32, error)

	// StoreFingerprints saves fingerprint hashes and their associated metadata.
	// The map key is the hash, values are the couples (songID + anchor time).
	StoreFingerprints(fingerprints map[uint32][]model.Couple) error

	// GetCouplesByHash retrieves all couples for a given hash.
	GetCouplesByHash(hash uint32) ([]model.Couple, error)

	// DeleteSongByID removes a song and all its fingerprints.
	DeleteSongByID(songID uint32) error

	// GetSongByID retrieves a song's metadata.
	GetSongByID(songID uint32) (*Song, error)

	// GetFingerprintCount returns the number of fingerprints for a song.
	GetFingerprintCount(songID uint32) (int, error)

	// ListSongs returns all songs in the database.
	ListSongs() ([]Song, error)

	// Close releases database resources.
	Close() error
}

// Logger defines the logging interface used by the service.
// This allows users to provide their own logger implementation.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
	Debugf(format string, args ...any)
}
