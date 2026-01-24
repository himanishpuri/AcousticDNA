//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

import (
	"context"
	"fmt"
	"math"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/fingerprint"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

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

	if cfg.Logger == nil {
		cfg.Logger = logger.GetLogger()
	}

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

func (s *acousticService) AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (string, error) {
	s.log.Infof("Processing song: %s by %s", title, artist)

	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, s.config.TempDir, audio.ConvertWAVConfig{
		SampleRate: s.config.SampleRate,
	})
	if err != nil {
		return "", fmt.Errorf("audio conversion failed: %w", err)
	}

	samples, sampleRate, err := audio.ReadWavAsFloat64(wavPath)
	if err != nil {
		return "", fmt.Errorf("failed to read WAV file: %w", err)
	}

	spec, _, err := fingerprint.ComputeSpectrogram(wavPath, 0, 0)
	if err != nil {
		return "", fmt.Errorf("spectrogram generation failed: %w", err)
	}

	duration := float64(len(samples)) / float64(sampleRate)
	peaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	s.log.Infof("Extracted %d peaks", len(peaks))

	songID, err := s.storage.RegisterSong(title, artist, youtubeID, int(duration*1000))
	if err != nil {
		return "", fmt.Errorf("failed to register song: %w", err)
	}

	fps := fingerprint.Fingerprint(peaks, songID)
	s.log.Infof("Generated %d unique hashes", len(fps))

	storageFPs := make(map[uint32][]models.Couple)
	for hash, modelCouples := range fps {
		couples := make([]models.Couple, len(modelCouples))
		for i, mc := range modelCouples {
			couples[i] = models.Couple{
				SongID:       mc.SongID,
				AnchorTimeMs: mc.AnchorTimeMs,
			}
		}
		storageFPs[hash] = couples
	}

	if err := s.storage.StoreFingerprints(storageFPs); err != nil {
		s.storage.DeleteSongByID(songID)
		return "", fmt.Errorf("failed to store fingerprints: %w", err)
	}

	s.log.Infof("Successfully added song ID=%s", songID)
	return songID, nil
}

func (s *acousticService) MatchSong(ctx context.Context, audioPath string) ([]models.MatchResult, error) {
	s.log.Infof("Matching audio: %s", audioPath)

	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, s.config.TempDir, audio.ConvertWAVConfig{
		SampleRate: s.config.SampleRate,
	})
	if err != nil {
		return nil, fmt.Errorf("audio conversion failed: %w", err)
	}

	samples, sampleRate, err := audio.ReadWavAsFloat64(wavPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV file: %w", err)
	}

	spec, _, err := fingerprint.ComputeSpectrogram(wavPath, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("spectrogram generation failed: %w", err)
	}

	duration := float64(len(samples)) / float64(sampleRate)
	queryPeaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	s.log.Infof("Query has %d peaks", len(queryPeaks))

	queryFPs := fingerprint.Fingerprint(queryPeaks, "")
	s.log.Infof("Generated %d query hashes", len(queryFPs))

	hashList := make([]uint32, 0, len(queryFPs))
	for hash := range queryFPs {
		hashList = append(hashList, hash)
	}

	dbMap, err := s.storage.GetCouplesByHashes(hashList)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve fingerprints from database: %w", err)
	}
	s.log.Infof("Retrieved couples for %d/%d hashes", len(dbMap), len(queryFPs))

	matches := fingerprint.QueryFingerprints(queryPeaks, dbMap)
	s.log.Infof("Found %d candidate matches", len(matches))

	results := make([]models.MatchResult, 0, len(matches))
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
			dbFingerprintCount = len(queryFPs)
		}

		confidence := s.calculateConfidence(match.Count, len(queryFPs), dbFingerprintCount)

		results = append(results, models.MatchResult{
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

func (s *acousticService) MatchHashes(ctx context.Context, hashes map[uint32]uint32) ([]models.MatchResult, error) {
	s.log.Infof("Matching %d pre-computed hashes", len(hashes))

	if len(hashes) == 0 {
		return []models.MatchResult{}, nil
	}

	hashList := make([]uint32, 0, len(hashes))
	for hash := range hashes {
		hashList = append(hashList, hash)
	}

	dbMap, err := s.storage.GetCouplesByHashes(hashList)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve fingerprints from database: %w", err)
	}
	s.log.Infof("Retrieved couples for %d/%d hashes", len(dbMap), len(hashes))

	// 3. Perform time-coherence voting to find matches
	// Build offset votes: map[songID]map[offset]count
	votes := make(map[string]map[int32]int)

	for hash, queryAnchorTime := range hashes {
		dbCouples, exists := dbMap[hash]
		if !exists {
			continue
		}

		for _, couple := range dbCouples {
			// Calculate time offset: dbTime - queryTime
			offset := int32(couple.AnchorTimeMs) - int32(queryAnchorTime)

			songVotes := votes[couple.SongID]
			if songVotes == nil {
				songVotes = make(map[int32]int)
				votes[couple.SongID] = songVotes
			}
			songVotes[offset]++
		}
	}

	// 4. Find the best (most voted) offset for each song
	matches := make([]models.Match, 0)
	for songID, offsetVotes := range votes {
		bestOffset := int32(0)
		bestCount := 0

		for offset, count := range offsetVotes {
			if count > bestCount {
				bestCount = count
				bestOffset = offset
			}
		}

		if bestCount > 0 {
			matches = append(matches, models.Match{
				SongID:   songID,
				OffsetMs: bestOffset,
				Count:    bestCount,
			})
		}
	}

	s.log.Infof("Found %d candidate matches", len(matches))

	// 5. Convert to results with song metadata
	results := make([]models.MatchResult, 0, len(matches))
	for _, match := range matches {
		song, err := s.GetSongByID(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get song %d: %v", match.SongID, err)
			continue
		}

		// Get database song's fingerprint count for confidence calculation
		dbFingerprintCount, err := s.storage.GetFingerprintCount(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get fingerprint count for song %d: %v", match.SongID, err)
			dbFingerprintCount = len(hashes) // Fallback
		}

		confidence := s.calculateConfidence(match.Count, len(hashes), dbFingerprintCount)

		results = append(results, models.MatchResult{
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
func (s *acousticService) GetSongByID(songID string) (*models.Song, error) {
	return s.storage.GetSongByID(songID)
}

// ListSongs returns all songs in the database.
func (s *acousticService) ListSongs() ([]models.Song, error) {
	return s.storage.ListSongs()
}

// DeleteSong removes a song and all its fingerprints from the database.
func (s *acousticService) DeleteSong(songID string) error {
	return s.storage.DeleteSongByID(songID)
}

// Close releases all resources held by the service.
func (s *acousticService) Close() error {
	return s.storage.Close()
}
