package acousticdna

import (
	"context"
	"fmt"
	"math"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/fingerprint"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/model"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
)

// acousticService is the default implementation of the Service interface.
type acousticService struct {
	storage Storage
	log     Logger
	config  *Config
}

func NewService(opts ...Option) (Service, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	// Set default logger if none provided
	if cfg.Logger == nil {
		cfg.Logger = logger.GetLogger()
	}

	// Create or use provided storage
	var stor Storage
	var err error
	if cfg.Storage != nil {
		stor = cfg.Storage
	} else {
		stor, err = NewSQLiteStorage(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create storage: %w", err)
		}
	}

	return &acousticService{
		storage: stor,
		log:     cfg.Logger,
		config:  cfg,
	}, nil
}

// AddSong processes an audio file and stores its fingerprint in the database.
func (s *acousticService) AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (uint32, error) {
	s.log.Infof("Processing song: %s by %s", title, artist)

	// 1. Convert to mono WAV
	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, s.config.TempDir, audio.ConvertWAVConfig{
		SampleRate: s.config.SampleRate,
	})
	if err != nil {
		return 0, fmt.Errorf("audio conversion failed: %w", err)
	}

	// 2. Read WAV file to get samples for duration calculation
	samples, sampleRate, err := audio.ReadWavAsFloat64(wavPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read WAV file: %w", err)
	}

	// 3. Generate spectrogram
	spec, _, err := fingerprint.ComputeSpectrogram(wavPath, 0, 0)
	if err != nil {
		return 0, fmt.Errorf("spectrogram generation failed: %w", err)
	}

	// 4. Extract peaks
	duration := float64(len(samples)) / float64(sampleRate)
	peaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	s.log.Infof("Extracted %d peaks", len(peaks))

	// 5. Register song in DB
	songID, err := s.storage.RegisterSong(title, artist, youtubeID, int(duration*1000))
	if err != nil {
		return 0, fmt.Errorf("failed to register song: %w", err)
	}

	// 6. Generate fingerprints
	fps := fingerprint.Fingerprint(peaks, songID)
	s.log.Infof("Generated %d unique hashes", len(fps))

	// Convert model.Couple to Couple for storage
	storageFPs := make(map[uint32][]model.Couple)
	for hash, modelCouples := range fps {
		couples := make([]model.Couple, len(modelCouples))
		for i, mc := range modelCouples {
			couples[i] = model.Couple{
				SongID:       mc.SongID,
				AnchorTimeMs: mc.AnchorTimeMs,
			}
		}
		storageFPs[hash] = couples
	}

	// 7. Store fingerprints
	if err := s.storage.StoreFingerprints(storageFPs); err != nil {
		s.storage.DeleteSongByID(songID) // Rollback
		return 0, fmt.Errorf("failed to store fingerprints: %w", err)
	}

	s.log.Infof("Successfully added song ID=%d", songID)
	return songID, nil
}

// MatchSong finds matches for a query audio file.
func (s *acousticService) MatchSong(ctx context.Context, audioPath string) ([]MatchResult, error) {
	s.log.Infof("Matching audio: %s", audioPath)

	// 1. Convert to mono WAV
	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, s.config.TempDir, audio.ConvertWAVConfig{
		SampleRate: s.config.SampleRate,
	})
	if err != nil {
		return nil, fmt.Errorf("audio conversion failed: %w", err)
	}

	// 2. Read WAV for duration calculation
	samples, sampleRate, err := audio.ReadWavAsFloat64(wavPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV file: %w", err)
	}

	// 3. Generate spectrogram
	spec, _, err := fingerprint.ComputeSpectrogram(wavPath, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("spectrogram generation failed: %w", err)
	}

	// 4. Extract peaks
	duration := float64(len(samples)) / float64(sampleRate)
	queryPeaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	s.log.Infof("Query has %d peaks", len(queryPeaks))

	// 5. Generate query fingerprints
	queryFPs := fingerprint.Fingerprint(queryPeaks, 0) // songID=0 for query
	s.log.Infof("Generated %d query hashes", len(queryFPs))

	// 6. Build database fingerprint map by querying each hash
	dbMap := make(map[uint32][]model.Couple)
	for hash := range queryFPs {
		couples, err := s.storage.GetCouplesByHash(hash)
		if err != nil {
			s.log.Warnf("Failed to get couples for hash %d: %v", hash, err)
			continue
		}
		if len(couples) > 0 {
			// Convert Couple to model.Couple
			modelCouples := make([]model.Couple, len(couples))
			for i, c := range couples {
				modelCouples[i] = model.Couple{
					SongID:       c.SongID,
					AnchorTimeMs: c.AnchorTimeMs,
				}
			}
			dbMap[hash] = modelCouples
		}
	}
	s.log.Infof("Retrieved couples for %d/%d hashes", len(dbMap), len(queryFPs))

	// 7. Perform in-memory matching
	matches := fingerprint.QueryFingerprints(queryPeaks, dbMap)
	s.log.Infof("Found %d candidate matches", len(matches))

	// 8. Convert to results with song info
	results := make([]MatchResult, 0, len(matches))
	for _, match := range matches {
		song, err := s.GetSongByID(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get song %d: %v", match.SongID, err)
			continue
		}

		// Get database song's fingerprint count for better confidence calculation
		dbFingerprintCount, err := s.storage.GetFingerprintCount(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get fingerprint count for song %d: %v", match.SongID, err)
			dbFingerprintCount = len(queryFPs) // Fallback to query count
		}

		confidence := s.calculateConfidence(match.Count, len(queryFPs), dbFingerprintCount)

		results = append(results, MatchResult{
			SongID:     match.SongID,
			Title:      song.Title,
			Artist:     song.Artist,
			YouTubeID:  song.YouTubeID,
			Score:      match.Count,
			OffsetMs:   match.OffsetMs,
			Confidence: confidence,
		})
	}

	s.log.Infof("Returning %d matches", len(results))
	return results, nil
}

// calculateConfidence computes a more meaningful confidence score.
// It considers:
// - Match count (number of aligned fingerprints)
// - Query size and database song size (uses smaller as reference)
// - Sigmoid scaling to emphasize strong matches
// - Statistical significance (minimum threshold)
func (s *acousticService) calculateConfidence(matchCount, queryFPCount, dbFPCount int) float64 {
	if matchCount == 0 || queryFPCount == 0 || dbFPCount == 0 {
		return 0.0
	}

	// Use the minimum fingerprint count as the reference
	// This ensures fair comparison between short queries and long songs
	minFPCount := queryFPCount
	if dbFPCount < minFPCount {
		minFPCount = dbFPCount
	}

	// Base ratio: how many matched out of possible matches
	ratio := float64(matchCount) / float64(minFPCount)

	// Apply sigmoid-like scaling to make the confidence more meaningful:
	// - Low matches (< 5% of min): very low confidence (0-20%)
	// - Medium matches (5-20% of min): medium confidence (20-70%)
	// - High matches (> 20% of min): high confidence (70-100%)

	// Use a scaled and shifted logistic function
	// confidence = 100 / (1 + e^(-k*(ratio - threshold)))
	// Adjusted to give reasonable values

	const (
		// Steepness of the sigmoid curve
		steepness = 20.0
		// Midpoint of the sigmoid (50% confidence point)
		midpoint = 0.15 // 15% match ratio gives 50% confidence
	)

	// Sigmoid transformation
	exponent := -steepness * (ratio - midpoint)
	confidence := 100.0 / (1.0 + math.Exp(exponent))

	// Boost confidence for very strong matches (> 30% overlap)
	if ratio > 0.30 {
		boost := (ratio - 0.30) * 50 // Additional boost for exceptional matches
		confidence = math.Min(100.0, confidence+boost)
	}

	// Statistical significance filter: very low match counts are unreliable
	if matchCount < 5 {
		// Penalize very low match counts
		confidence *= float64(matchCount) / 5.0
	}

	return confidence
}

// GetSongByID retrieves a song's metadata by its database ID.
func (s *acousticService) GetSongByID(songID uint32) (*Song, error) {
	return s.storage.GetSongByID(songID)
}

// ListSongs returns all songs in the database.
func (s *acousticService) ListSongs() ([]Song, error) {
	return s.storage.ListSongs()
}

// DeleteSong removes a song and all its fingerprints from the database.
func (s *acousticService) DeleteSong(songID uint32) error {
	return s.storage.DeleteSongByID(songID)
}

// Close releases all resources held by the service.
func (s *acousticService) Close() error {
	return s.storage.Close()
}
