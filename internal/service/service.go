package service

import (
	"context"
	"fmt"

	"github.com/himanishpuri/AcousticDNA/internal/audio"
	"github.com/himanishpuri/AcousticDNA/internal/fingerprint"
	"github.com/himanishpuri/AcousticDNA/internal/model"
	"github.com/himanishpuri/AcousticDNA/internal/storage"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
)

type AcousticService struct {
	db  *storage.DBClient
	log *logger.Logger
}

func NewAcousticService() (*AcousticService, error) {
	db, err := storage.NewDBClient()
	if err != nil {
		return nil, err
	}

	return &AcousticService{
		db:  db,
		log: logger.GetLogger(),
	}, nil
}

// AddSong adds a song to the database
func (s *AcousticService) AddSong(ctx context.Context, audioPath, title, artist, youtubeID string) (uint32, error) {
	s.log.Infof("Processing song:  %s by %s", title, artist)

	// 1. Convert to mono WAV
	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, "/tmp", audio.ConvertWAVConfig{
		SampleRate: 11025,
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
	songID, err := s.db.RegisterSong(title, artist, youtubeID, int(duration*1000))
	if err != nil {
		return 0, fmt.Errorf("failed to register song: %w", err)
	}

	// 6. Generate fingerprints
	fps := fingerprint.Fingerprint(peaks, songID)
	s.log.Infof("Generated %d unique hashes", len(fps))

	// 7. Store fingerprints
	if err := s.db.StoreFingerprints(fps); err != nil {
		s.db.DeleteSongByID(songID) // Rollback
		return 0, fmt.Errorf("failed to store fingerprints: %w", err)
	}

	s.log.Infof("Successfully added song ID=%d", songID)
	return songID, nil
}

// MatchSong finds matches for a query audio
func (s *AcousticService) MatchSong(ctx context.Context, audioPath string) ([]MatchResult, error) {
	s.log.Infof("Matching audio: %s", audioPath)

	// 1. Convert to mono WAV
	wavPath, err := audio.ConvertToMonoWAV(ctx, audioPath, "/tmp", audio.ConvertWAVConfig{
		SampleRate: 11025,
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
		couples, err := s.db.GetCouplesByHash(hash)
		if err != nil {
			s.log.Warnf("Failed to get couples for hash %d: %v", hash, err)
			continue
		}
		if len(couples) > 0 {
			dbMap[hash] = couples
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

		results = append(results, MatchResult{
			SongID:     match.SongID,
			Title:      song.Title,
			Artist:     song.Artist,
			YouTubeID:  song.YouTubeID,
			Score:      match.Count,
			OffsetMs:   match.OffsetMs,
			Confidence: float64(match.Count) / float64(len(queryPeaks)) * 100,
		})
	}

	s.log.Infof("Returning %d matches", len(results))
	return results, nil
}

type MatchResult struct {
	SongID     uint32
	Title      string
	Artist     string
	YouTubeID  string
	Score      int
	OffsetMs   int32
	Confidence float64
}

// GetSongByID retrieves a song by its ID
func (s *AcousticService) GetSongByID(songID uint32) (*storage.Song, error) {
	var song storage.Song
	if err := s.db.DB.First(&song, songID).Error; err != nil {
		return nil, err
	}
	return &song, nil
}

// ListSongs returns all songs in the database
func (s *AcousticService) ListSongs() ([]storage.Song, error) {
	var songs []storage.Song
	if err := s.db.DB.Find(&songs).Error; err != nil {
		return nil, err
	}
	return songs, nil
}

// DeleteSong deletes a song and its fingerprints by ID
func (s *AcousticService) DeleteSong(songID uint32) error {
	return s.db.DeleteSongByID(songID)
}

func (s *AcousticService) Close() error {
	return s.db.Close()
}
