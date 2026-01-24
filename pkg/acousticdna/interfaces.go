package acousticdna

import (
	"context"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
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
	//   - title: models.Song title
	//   - artist: Artist name
	//   - youtubeID: YouTube video ID (optional, can be empty)
	//
	// Returns the database ID of the added song, or an error.
	AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (string, error)

	// MatchSong finds matches for a query audio file by comparing its fingerprint
	// against the database.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - audioPath: Path to the query audio file
	//
	// Returns a slice of match results sorted by confidence, or an error.
	MatchSong(ctx context.Context, audioPath string) ([]models.MatchResult, error)

	// MatchHashes finds matches for pre-computed hashes (useful for WASM clients).
	// This allows clients to generate fingerprints locally and only send hashes
	// to the server for matching, preserving privacy.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - hashes: Map of hash values to their anchor times in milliseconds
	//
	// Returns a slice of match results sorted by confidence, or an error.
	MatchHashes(ctx context.Context, hashes map[uint32]uint32) ([]models.MatchResult, error)

	// GetSongByID retrieves a song's metadata by its database ID.
	GetSongByID(songID string) (*models.Song, error)

	// ListSongs returns all songs in the database.
	ListSongs() ([]models.Song, error)

	// DeleteSong removes a song and all its fingerprints from the database.
	DeleteSong(songID string) error

	// Close releases all resources held by the service (database connections, etc).
	// Should be called when the service is no longer needed.
	Close() error
}

// Storage defines the persistence layer interface for songs and fingerprints.
// Implementations must handle concurrent access safely.
type Storage interface {
	// RegisterSong adds or updates a song in the database.
	// Returns the song ID.
	RegisterSong(title, artist, youtubeID string, durationMs int) (string, error)

	// StoreFingerprints saves fingerprint hashes and their associated metadata.
	// The map key is the hash, values are the couples (songID + anchor time).
	StoreFingerprints(fingerprints map[uint32][]models.Couple) error

	// GetCouplesByHash retrieves all couples for a given hash.
	GetCouplesByHash(hash uint32) ([]models.Couple, error)

	// GetCouplesByHashes retrieves couples for multiple hashes in a single query.
	// This is much more efficient than calling GetCouplesByHash in a loop.
	// Returns a map where keys are hashes and values are the couples for that hash.
	GetCouplesByHashes(hashes []uint32) (map[uint32][]models.Couple, error)

	// DeleteSongByID removes a song and all its fingerprints.
	DeleteSongByID(songID string) error

	// GetSongByID retrieves a song's metadata.
	GetSongByID(songID string) (*models.Song, error)

	// GetFingerprintCount returns the number of fingerprints for a song.
	GetFingerprintCount(songID string) (int, error)

	// ListSongs returns all songs in the database.
	ListSongs() ([]models.Song, error)

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
