//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

import (
	"context"
	"fmt"
	"math"
	"sort"

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

	spec, err := fingerprint.ComputeSpectrogramFromSamples(samples, sampleRate, 0, 0)
	if err != nil {
		return "", fmt.Errorf("spectrogram generation failed: %w", err)
	}

	duration := float64(len(samples)) / float64(sampleRate)
	peaks := fingerprint.ExtractPeaks(spec, sampleRate)
	s.log.Infof("Extracted %d peaks", len(peaks))

	songID, err := s.storage.RegisterSong(title, artist, youtubeID, int(duration*1000))
	if err != nil {
		return "", fmt.Errorf("failed to register song: %w", err)
	}

	fps := fingerprint.Fingerprint(peaks, songID)
	s.log.Infof("Generated %d unique hashes", len(fps))

	if err := s.storage.StoreFingerprints(fps); err != nil {
		if delErr := s.storage.DeleteSongByID(songID); delErr != nil {
			s.log.Warnf("Failed to roll back song %s after fingerprint store error: %v", songID, delErr)
		}
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

	spec, err := fingerprint.ComputeSpectrogramFromSamples(samples, sampleRate, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("spectrogram generation failed: %w", err)
	}

	queryPeaks := fingerprint.ExtractPeaks(spec, sampleRate)
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
	for i, match := range matches {
		song, err := s.GetSongByID(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get song %s: %v", match.SongID, err)
			continue
		}

		confidence := s.calculateConfidence(match.Count, match.SecondBestCount, competingScore(matches, i))

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

	// 4. Find the best (most voted) offset for each song, plus the runner-up
	// bin (peak sharpness signal). Mirrors QueryFingerprints exactly.
	matches := make([]models.Match, 0)
	for songID, offsetVotes := range votes {
		bestOffset := int32(0)
		bestCount := 0
		secondBest := 0

		for offset, count := range offsetVotes {
			if count > bestCount {
				secondBest = bestCount
				bestCount = count
				bestOffset = offset
			} else if count > secondBest {
				secondBest = count
			}
		}

		if bestCount > 0 {
			matches = append(matches, models.Match{
				SongID:          songID,
				OffsetMs:        bestOffset,
				Count:           bestCount,
				SecondBestCount: secondBest,
			})
		}
	}

	// Sort best-first so the WASM /api/match/hashes endpoint returns
	// deterministic descending-Count order (mirrors QueryFingerprints).
	sort.Slice(matches, func(i, j int) bool { return matches[i].Count > matches[j].Count })

	s.log.Infof("Found %d candidate matches", len(matches))

	// 5. Convert to results with song metadata
	results := make([]models.MatchResult, 0, len(matches))
	for i, match := range matches {
		song, err := s.GetSongByID(match.SongID)
		if err != nil {
			s.log.Warnf("Failed to get song %s: %v", match.SongID, err)
			continue
		}

		confidence := s.calculateConfidence(match.Count, match.SecondBestCount, competingScore(matches, i))

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

// competingScore returns the best vote count among songs OTHER than the one at
// index i, given matches sorted by Count desc. For the top match that is the
// runner-up (matches[1]); for any lower match it is the leader (matches[0]).
// Used as the cross-song separation signal in calculateConfidence.
func competingScore(matches []models.Match, i int) int {
	if len(matches) < 2 {
		return 0
	}
	if i == 0 {
		return matches[1].Count
	}
	return matches[0].Count
}

// calculateConfidence scores a match (0-100) from the two signals that actually
// distinguish a true hit from noise, independent of query/song length:
//
//   - peakiness: how much the winning time-offset bin dominates the song's
//     second-tallest bin. A real match spikes at one offset (bestCount >>
//     secondBest); noise scatters votes across offsets (bestCount ~= secondBest).
//   - margin: how far this song's score beats the best competing song. A
//     confident hit stands out from the field; ambiguous noise ties with it.
//
// The old model (matchCount / total-query-hashes through a sigmoid) is abandoned:
// aligned votes are always a tiny fraction of total hashes, so every match —
// true or false — collapsed to the sigmoid floor (~5%).
func (s *acousticService) calculateConfidence(bestCount, secondBestOffsetCount, competing int) float64 {
	if bestCount == 0 {
		return 0.0
	}

	best := float64(bestCount)
	peakiness := 1.0 - float64(secondBestOffsetCount)/best // 1 = single dominant offset
	if peakiness < 0 {
		peakiness = 0
	}
	margin := (best - float64(competing)) / best // 1 = no competing song
	if margin < 0 {
		margin = 0
	}

	// Equal blend of the two signals, spread across 0-100 by a logistic centered
	// where a clearly separated, peaked match sits well above a flat/tied one.
	const (
		steepness = 9.0  // slope
		midpoint  = 0.35 // blended-signal value mapping to 50%
	)
	signal := 0.5*peakiness + 0.5*margin
	confidence := 100.0 / (1.0 + math.Exp(-steepness*(signal-midpoint)))

	// Absolute-reliability floor: a handful of votes can't be highly confident
	// no matter how peaked/separated. Quadratic so a 2-3 vote spurious hit is
	// strongly damped even when it happens to be peaked with no competitor.
	const minReliable = 8
	if bestCount < minReliable {
		f := float64(bestCount) / float64(minReliable)
		confidence *= f * f
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

// CountFingerprints returns the total number of stored fingerprint rows.
func (s *acousticService) CountFingerprints() (int64, error) {
	return s.storage.GetTotalFingerprintCount()
}

// DeleteSong removes a song and all its fingerprints from the database.
func (s *acousticService) DeleteSong(songID string) error {
	return s.storage.DeleteSongByID(songID)
}

// Close releases all resources held by the service.
func (s *acousticService) Close() error {
	return s.storage.Close()
}
