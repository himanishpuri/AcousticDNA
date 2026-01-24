package main

import (
	"fmt"
	"strconv"
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
	// Hashes is a map where keys are the hash values (as strings from JSON) and
	// values are the anchor times in milliseconds
	Hashes map[string]uint32 `json:"hashes" binding:"required"`
}

// ToHashMap converts the string-keyed hash map to uint32-keyed map
func (r *MatchHashesRequest) ToHashMap() (map[uint32]uint32, error) {
	result := make(map[uint32]uint32, len(r.Hashes))
	invalidCount := 0

	for hashStr, anchorTime := range r.Hashes {
		// Parse string key as uint32
		hash64, err := strconv.ParseUint(hashStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid hash key '%s': %v", hashStr, err)
		}
		hash := uint32(hash64)

		// Validate the hash format and skip invalid ones
		if !isValidHash(hash) {
			invalidCount++
			// Skip invalid hashes instead of failing the entire request
			continue
		}

		result[hash] = anchorTime
	}

	// Log if we skipped invalid hashes
	if invalidCount > 0 {
		fmt.Printf("Warning: Skipped %d invalid hashes out of %d total\n", invalidCount, len(r.Hashes))
	}

	// If all hashes were invalid, that's an error
	if len(result) == 0 {
		return nil, fmt.Errorf("all %d hashes were invalid", len(r.Hashes))
	}

	return result, nil
}

// Validate checks if the request is valid
func (r *MatchHashesRequest) Validate() error {
	if len(r.Hashes) == 0 {
		return fmt.Errorf("hashes cannot be empty")
	}
	if len(r.Hashes) > MaxHashesHardLimit {
		return fmt.Errorf("too many hashes: %d (maximum: %d)", len(r.Hashes), MaxHashesHardLimit)
	}

	// Validation of hash format is now done in ToHashMap() during conversion
	return nil
}

// isValidHash performs lightweight validation of hash structure
// Hash format: [anchorFreq (9 bits) | targetFreq (9 bits) | deltaTime (14 bits)]
func isValidHash(hash uint32) bool {
	// Extract components using the same bit layout as hasher.go
	deltaTime := hash & 0x3FFF         // 14 bits (bits 0-13)
	targetFreq := (hash >> 14) & 0x1FF // 9 bits (bits 14-22)
	anchorFreq := (hash >> 23) & 0x1FF // 9 bits (bits 23-31)

	// Basic sanity checks to catch obviously corrupted data
	// deltaTime should be within valid range (MinDeltaMs=10 to MaxDeltaMs=15000)
	if deltaTime < 10 || deltaTime > 15000 {
		return false
	}

	// At least one frequency should be non-zero (both being 0 indicates corruption)
	// Note: In rare cases, bin 0 (DC component) might be used, so we allow it
	// if the other frequency is valid
	if anchorFreq == 0 && targetFreq == 0 {
		return false
	}

	// Frequencies should fit within FFT bin range (0-511 for 9 bits)
	// This is implicitly true due to the bitmask, but we add the check for clarity
	if anchorFreq > 511 || targetFreq > 511 {
		return false
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
