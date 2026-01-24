package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
	"github.com/himanishpuri/AcousticDNA/pkg/utils"
)

// Server encapsulates the HTTP server and its dependencies
type Server struct {
	service acousticdna.Service
	config  *ServerConfig
	log     acousticdna.Logger
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port           int
	DBPath         string
	TempDir        string
	SampleRate     int
	AllowedOrigins []string
}

// NewServer creates a new server instance
func NewServer(service acousticdna.Service, config *ServerConfig) *Server {
	return &Server{
		service: service,
		config:  config,
		log:     logger.GetLogger(),
	}
}

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.log.Errorf("Failed to encode JSON response: %v", err)
	}
}

// respondError writes an error response
func (s *Server) respondError(w http.ResponseWriter, statusCode int, message string) {
	s.respondJSON(w, statusCode, ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: message,
		Code:    statusCode,
	})
}

// handleRoot handles GET /
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"service": "AcousticDNA API",
		"version": "1.0.0",
		"endpoints": map[string]string{
			"health":         "GET /health",
			"metrics":        "GET /api/health/metrics",
			"songs":          "GET /api/songs",
			"addSongFile":    "POST /api/songs",
			"addSongYouTube": "POST /api/songs/youtube",
			"getSong":        "GET /api/songs/{id}",
			"deleteSong":     "DELETE /api/songs/{id}",
			"matchFile":      "POST /api/match",
			"matchHashes":    "POST /api/match/hashes",
		},
	})
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleMetrics handles GET /api/health/metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	songs, err := s.service.ListSongs()
	if err != nil {
		s.log.Errorf("Failed to get song count: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to retrieve metrics")
		return
	}

	s.respondJSON(w, http.StatusOK, MetricsResponse{
		Status:       "healthy",
		DatabasePath: s.config.DBPath,
		SongCount:    len(songs),
		SampleRate:   s.config.SampleRate,
	})
}

// handleListSongs handles GET /api/songs
func (s *Server) handleListSongs(w http.ResponseWriter, r *http.Request) {
	songs, err := s.service.ListSongs()
	if err != nil {
		s.log.Errorf("Failed to list songs: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to retrieve songs")
		return
	}

	songDTOs := make([]SongDTO, len(songs))
	for i, song := range songs {
		songDTOs[i] = SongDTO{
			ID:         song.ID,
			Title:      song.Title,
			Artist:     song.Artist,
			YouTubeID:  song.YouTubeID,
			DurationMs: song.DurationMs,
		}
	}

	s.respondJSON(w, http.StatusOK, ListSongsResponse{
		Songs: songDTOs,
		Count: len(songDTOs),
	})
}

// handleGetSong handles GET /api/songs/{id}
func (s *Server) handleGetSong(w http.ResponseWriter, r *http.Request, songID uint32) {
	song, err := s.service.GetSongByID(songID)
	if err != nil {
		s.log.Warnf("Song not found: %d", songID)
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Song with ID %d not found", songID))
		return
	}

	s.respondJSON(w, http.StatusOK, SongDTO{
		ID:         song.ID,
		Title:      song.Title,
		Artist:     song.Artist,
		YouTubeID:  song.YouTubeID,
		DurationMs: song.DurationMs,
	})
}

// handleDeleteSong handles DELETE /api/songs/{id}
func (s *Server) handleDeleteSong(w http.ResponseWriter, r *http.Request, songID uint32) {
	// Get song info before deletion
	song, err := s.service.GetSongByID(songID)
	if err != nil {
		s.log.Warnf("Song not found for deletion: %d", songID)
		s.respondError(w, http.StatusNotFound, fmt.Sprintf("Song with ID %d not found", songID))
		return
	}

	if err := s.service.DeleteSong(songID); err != nil {
		s.log.Errorf("Failed to delete song %d: %v", songID, err)
		s.respondError(w, http.StatusInternalServerError, "Failed to delete song")
		return
	}

	s.log.Infof("Deleted song: %s by %s (ID: %d)", song.Title, song.Artist, songID)
	s.respondJSON(w, http.StatusOK, DeleteSongResponse{
		Message: "Song deleted successfully",
		ID:      songID,
	})
}

// handleAddSongFile handles POST /api/songs (multipart file upload)
func (s *Server) handleAddSongFile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Parse multipart form (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		s.log.Errorf("Failed to parse form: %v", err)
		s.respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get form fields
	title := r.FormValue("title")
	artist := r.FormValue("artist")
	youtubeID := r.FormValue("youtube_id")

	if title == "" || artist == "" {
		s.respondError(w, http.StatusBadRequest, "title and artist are required")
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("audio")
	if err != nil {
		s.log.Errorf("Failed to get audio file: %v", err)
		s.respondError(w, http.StatusBadRequest, "audio file is required")
		return
	}
	defer file.Close()

	// Save to temporary file
	tempFile := filepath.Join(s.config.TempDir, fmt.Sprintf("upload_%d_%s", time.Now().UnixNano(), header.Filename))
	out, err := os.Create(tempFile)
	if err != nil {
		s.log.Errorf("Failed to create temp file: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to process upload")
		return
	}
	defer out.Close()
	defer os.Remove(tempFile)

	if _, err := io.Copy(out, file); err != nil {
		s.log.Errorf("Failed to save file: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to save uploaded file")
		return
	}
	out.Close()

	// Add song to database
	s.log.Infof("Adding song from file: %s by %s", title, artist)
	songID, err := s.service.AddSong(ctx, tempFile, title, artist, youtubeID)
	if err != nil {
		s.log.Errorf("Failed to add song: %v", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add song: %v", err))
		return
	}

	s.log.Infof("Successfully added song: %s by %s (ID: %d)", title, artist, songID)
	s.respondJSON(w, http.StatusCreated, AddSongResponse{
		Message:   "Song added successfully",
		ID:        songID,
		Title:     title,
		Artist:    artist,
		YouTubeID: youtubeID,
	})
}

// handleAddSongYouTube handles POST /api/songs/youtube
func (s *Server) handleAddSongYouTube(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var req AddSongYouTubeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorf("Failed to decode request: %v", err)
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := req.Validate(); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.log.Infof("Adding song from YouTube URL: %s", req.YouTubeURL)

	// Download YouTube audio
	downloadedPath, ytMeta, err := audio.DownloadYouTubeAudio(ctx, req.YouTubeURL, s.config.TempDir, s.config.SampleRate)
	if err != nil {
		s.log.Errorf("Failed to download YouTube video: %v", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to download YouTube video: %v", err))
		return
	}
	defer os.Remove(downloadedPath)

	// Use metadata from YouTube if not provided
	title := req.Title
	artist := req.Artist
	if title == "" {
		title = ytMeta.Title
	}
	if artist == "" {
		artist = ytMeta.Artist
	}

	// Extract YouTube ID
	youtubeID, err := utils.ExtractYouTubeID(req.YouTubeURL)
	if err != nil {
		s.log.Warnf("Failed to extract YouTube ID: %v", err)
		youtubeID = ""
	}

	// Validate we have title and artist
	if title == "" || artist == "" {
		s.respondError(w, http.StatusBadRequest, "Could not determine title or artist from YouTube metadata. Please provide them explicitly.")
		return
	}

	// Add song to database
	s.log.Infof("Adding downloaded song: %s by %s", title, artist)
	songID, err := s.service.AddSong(ctx, downloadedPath, title, artist, youtubeID)
	if err != nil {
		s.log.Errorf("Failed to add song: %v", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to add song: %v", err))
		return
	}

	s.log.Infof("Successfully added song from YouTube: %s by %s (ID: %d)", title, artist, songID)
	s.respondJSON(w, http.StatusCreated, AddSongResponse{
		Message:   "Song added successfully from YouTube",
		ID:        songID,
		Title:     title,
		Artist:    artist,
		YouTubeID: youtubeID,
	})
}

// handleMatchFile handles POST /api/match (multipart file upload)
func (s *Server) handleMatchFile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		s.log.Errorf("Failed to parse form: %v", err)
		s.respondError(w, http.StatusBadRequest, "Failed to parse form data")
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("audio")
	if err != nil {
		s.log.Errorf("Failed to get audio file: %v", err)
		s.respondError(w, http.StatusBadRequest, "audio file is required")
		return
	}
	defer file.Close()

	// Save to temporary file
	tempFile := filepath.Join(s.config.TempDir, fmt.Sprintf("query_%d_%s", time.Now().UnixNano(), header.Filename))
	out, err := os.Create(tempFile)
	if err != nil {
		s.log.Errorf("Failed to create temp file: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to process upload")
		return
	}
	defer out.Close()
	defer os.Remove(tempFile)

	if _, err := io.Copy(out, file); err != nil {
		s.log.Errorf("Failed to save file: %v", err)
		s.respondError(w, http.StatusInternalServerError, "Failed to save uploaded file")
		return
	}
	out.Close()

	// Match song
	s.log.Infof("Matching uploaded file: %s", header.Filename)
	matches, err := s.service.MatchSong(ctx, tempFile)
	if err != nil {
		s.log.Errorf("Failed to match song: %v", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to match song: %v", err))
		return
	}

	// Convert to DTOs
	matchDTOs := make([]MatchResultDTO, len(matches))
	for i, match := range matches {
		matchDTOs[i] = MatchResultDTO{
			SongID:     match.SongID,
			Title:      match.Title,
			Artist:     match.Artist,
			YouTubeID:  match.YouTubeID,
			Score:      match.Score,
			OffsetMs:   match.OffsetMs,
			Confidence: match.Confidence,
		}
	}

	s.log.Infof("Match complete: found %d matches", len(matchDTOs))
	s.respondJSON(w, http.StatusOK, MatchHashesResponse{
		Matches: matchDTOs,
		Count:   len(matchDTOs),
	})
}

// handleMatchHashes handles POST /api/match/hashes (hash-based matching for WASM clients)
func (s *Server) handleMatchHashes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	var req MatchHashesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.log.Errorf("Failed to decode request: %v", err)
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert string-keyed map to uint32-keyed map with validation
	hashMap, err := req.ToHashMap()
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Log warning for large batches
	if len(hashMap) >= HashWarningThreshold {
		s.log.Warnf("Large hash batch received: %d hashes", len(hashMap))
	}

	s.log.Infof("Matching %d hashes from client", len(hashMap))

	// Match hashes
	matches, err := s.service.MatchHashes(ctx, hashMap)
	if err != nil {
		s.log.Errorf("Failed to match hashes: %v", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to match hashes: %v", err))
		return
	}

	// Convert to DTOs
	matchDTOs := make([]MatchResultDTO, len(matches))
	for i, match := range matches {
		matchDTOs[i] = MatchResultDTO{
			SongID:     match.SongID,
			Title:      match.Title,
			Artist:     match.Artist,
			YouTubeID:  match.YouTubeID,
			Score:      match.Score,
			OffsetMs:   match.OffsetMs,
			Confidence: match.Confidence,
		}
	}

	s.log.Infof("Hash match complete: found %d matches", len(matchDTOs))
	s.respondJSON(w, http.StatusOK, MatchHashesResponse{
		Matches: matchDTOs,
		Count:   len(matchDTOs),
	})
}

// handleSongs routes requests to /api/songs
func (s *Server) handleSongs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSongs(w, r)
	case http.MethodPost:
		s.handleAddSongFile(w, r)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleSong routes requests to /api/songs/{id}
func (s *Server) handleSong(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	idStr := r.URL.Path[len("/api/songs/"):]
	if idStr == "" {
		s.respondError(w, http.StatusBadRequest, "Song ID required")
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid song ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetSong(w, r, uint32(id))
	case http.MethodDelete:
		s.handleDeleteSong(w, r, uint32(id))
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleMatch routes requests to /api/match
func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	s.handleMatchFile(w, r)
}

// handleMatchHashesRoute routes requests to /api/match/hashes
func (s *Server) handleMatchHashesRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	s.handleMatchHashes(w, r)
}
