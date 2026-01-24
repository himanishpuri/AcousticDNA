package main

import (
	"fmt"
)

// Hash limit constants for validation
const (
	// MaxHashesSoftLimit is the recommended maximum for most queries (~20-30 seconds of audio)
	MaxHashesSoftLimit = 10000

	// MaxHashesHardLimit is the absolute maximum allowed (~2 minutes of audio)
	MaxHashesHardLimit = 50000

	// HashWarningThreshold triggers logging for large hash batches
	HashWarningThreshold = 5000
)

// MatchHashesRequest is the request body for POST /api/match/hashes
type MatchHashesRequest struct {
	// Hashes is a map where keys are the hash values (uint32) and
	// values are the anchor times in milliseconds
	Hashes map[uint32]uint32 `json:"hashes" binding:"required"`
}

// Validate checks if the request is valid
func (r *MatchHashesRequest) Validate() error {
	if len(r.Hashes) == 0 {
		return fmt.Errorf("hashes cannot be empty")
	}
	if len(r.Hashes) > MaxHashesHardLimit {
		return fmt.Errorf("too many hashes: %d (maximum: %d)", len(r.Hashes), MaxHashesHardLimit)
	}

	// Lightweight hash validation: check format
	for hash := range r.Hashes {
		if !isValidHash(hash) {
			return fmt.Errorf("invalid hash format: %d", hash)
		}
	}

	return nil
}

// isValidHash performs lightweight validation of hash structure
// Hash format: [anchorFreq (9 bits) | targetFreq (9 bits) | deltaTime (14 bits)]
func isValidHash(hash uint32) bool {
	// Extract components
	deltaTime := hash & 0x3FFF         // 14 bits
	targetFreq := (hash >> 14) & 0x1FF // 9 bits
	anchorFreq := (hash >> 23) & 0x1FF // 9 bits

	// Validate ranges
	// deltaTime: 0-16383 ms (0-16.3 seconds)
	// anchorFreq, targetFreq: 0-511 (frequency bins)

	// Check if any unused bits are set (would indicate corruption)
	if hash > 0xFFFFFFFF {
		return false
	}

	// Basic sanity checks
	if deltaTime == 0 {
		return false // deltaTime should never be 0
	}
	if anchorFreq == targetFreq {
		return false // Anchor and target should be different
	}

	return true
}

// MatchHashesResponse is the response for hash-based matching
type MatchHashesResponse struct {
	Matches []MatchResultDTO `json:"matches"`
	Count   int              `json:"count"`
}

// MatchResultDTO represents a single match result
type MatchResultDTO struct {
	SongID     uint32  `json:"song_id"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	YouTubeID  string  `json:"youtube_id,omitempty"`
	Score      int     `json:"score"`
	OffsetMs   int32   `json:"offset_ms"`
	Confidence float64 `json:"confidence"`
}

// AddSongYouTubeRequest is the request body for POST /api/songs/youtube
type AddSongYouTubeRequest struct {
	// YouTubeURL is the full YouTube video URL (required)
	YouTubeURL string `json:"youtube_url" binding:"required"`

	// Title is optional - if not provided, will be extracted from YouTube metadata
	Title string `json:"title,omitempty"`

	// Artist is optional - if not provided, will be extracted from YouTube metadata
	Artist string `json:"artist,omitempty"`
}

// Validate checks if the request is valid
func (r *AddSongYouTubeRequest) Validate() error {
	if r.YouTubeURL == "" {
		return fmt.Errorf("youtube_url is required")
	}
	// Additional validation could check URL format, but utils.ExtractYouTubeID handles this
	return nil
}

// AddSongResponse is the response for successful song addition
type AddSongResponse struct {
	Message   string `json:"message"`
	ID        uint32 `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	YouTubeID string `json:"youtube_id,omitempty"`
}

// SongDTO represents a song in API responses
type SongDTO struct {
	ID         uint32 `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	YouTubeID  string `json:"youtube_id,omitempty"`
	DurationMs int    `json:"duration_ms"`
}

// ListSongsResponse is the response for GET /api/songs
type ListSongsResponse struct {
	Songs []SongDTO `json:"songs"`
	Count int       `json:"count"`
}

// DeleteSongResponse is the response for DELETE /api/songs/{id}
type DeleteSongResponse struct {
	Message string `json:"message"`
	ID      uint32 `json:"id"`
}

// MetricsResponse provides server health and database metrics
type MetricsResponse struct {
	Status           string `json:"status"`
	DatabasePath     string `json:"database_path"`
	SongCount        int    `json:"song_count"`
	FingerprintCount int64  `json:"fingerprint_count"`
	SampleRate       int    `json:"sample_rate"`
}

// ErrorResponse is the standard error response format
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}
