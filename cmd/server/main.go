package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna"
)

const (
	contentTypeJSON   = "application/json"
	headerContentType = "Content-Type"
	methodNotAllowed  = "Method not allowed"
)

var (
	port       int
	dbPath     string
	tempDir    string
	sampleRate int
	service    acousticdna.Service
)

func init() {
	flag.IntVar(&port, "port", 8080, "HTTP server port")
	flag.StringVar(&dbPath, "db", getEnvOrDefault("ACOUSTIC_DB_PATH", "acousticdna.sqlite3"), "Path to SQLite database")
	flag.StringVar(&tempDir, "temp", getEnvOrDefault("ACOUSTIC_TEMP_DIR", "/tmp"), "Temporary directory")
	flag.IntVar(&sampleRate, "rate", 11025, "Audio sample rate")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	flag.Parse()

	// Create service
	var err error
	service, err = acousticdna.NewService(
		acousticdna.WithDBPath(dbPath),
		acousticdna.WithTempDir(tempDir),
		acousticdna.WithSampleRate(sampleRate),
	)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	defer service.Close()

	// Setup routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/api/songs", songsHandler)
	http.HandleFunc("/api/songs/", songHandler)
	http.HandleFunc("/api/match", matchHandler)
	http.HandleFunc("/", rootHandler)

	// Start server
	addr := fmt.Sprintf(":%d", port)
	log.Printf("🚀 AcousticDNA server starting on %s", addr)
	log.Printf("   Database: %s", dbPath)
	log.Printf("   Sample Rate: %d Hz", sampleRate)
	log.Printf("\nEndpoints:")
	log.Printf("   GET    /health            - Health check")
	log.Printf("   GET    /api/songs         - List all songs")
	log.Printf("   POST   /api/songs         - Add a new song")
	log.Printf("   GET    /api/songs/{id}    - Get song by ID")
	log.Printf("   DELETE /api/songs/{id}    - Delete song by ID")
	log.Printf("   POST   /api/match         - Match audio file")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": "AcousticDNA API",
		"version": "1.0.0",
		"endpoints": map[string]string{
			"health": "/health",
			"songs":  "/api/songs",
		},
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func songsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listSongs(w, r)
	case http.MethodPost:
		addSong(w, r)
	default:
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
	}
}

func songHandler(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path
	idStr := r.URL.Path[len("/api/songs/"):]
	if idStr == "" {
		http.Error(w, "Song ID required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid song ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getSong(w, r, uint32(id))
	case http.MethodDelete:
		deleteSong(w, r, uint32(id))
	default:
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
	}
}

func listSongs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_ = ctx // Use context if needed in future
	songs, err := service.ListSongs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list songs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"songs": songs,
		"count": len(songs),
	})
}

func getSong(w http.ResponseWriter, r *http.Request, songID uint32) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_ = ctx // Use context if needed in future
	song, err := service.GetSongByID(songID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Song not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(song)
}

func deleteSong(w http.ResponseWriter, r *http.Request, songID uint32) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_ = ctx // Use context if needed in future
	err := service.DeleteSong(songID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete song: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Song deleted successfully",
		"id":      songID,
	})
}

func addSong(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Parse multipart form (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get form fields
	title := r.FormValue("title")
	artist := r.FormValue("artist")
	youtubeID := r.FormValue("youtube_id")

	if title == "" || artist == "" {
		http.Error(w, "title and artist are required", http.StatusBadRequest)
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get audio file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to temporary file
	tempFile := filepath.Join(tempDir, header.Filename)
	out, err := os.Create(tempFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer out.Close()
	defer os.Remove(tempFile)

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save file: %v", err), http.StatusInternalServerError)
		return
	}
	out.Close()

	// Add song to database
	songID, err := service.AddSong(ctx, tempFile, title, artist, youtubeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to add song: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Song added successfully",
		"id":      songID,
		"title":   title,
		"artist":  artist,
	})
}

func matchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// Parse multipart form (max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get uploaded file
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get audio file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save to temporary file
	tempFile := filepath.Join(tempDir, "query_"+header.Filename)
	out, err := os.Create(tempFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer out.Close()
	defer os.Remove(tempFile)

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save file: %v", err), http.StatusInternalServerError)
		return
	}
	out.Close()

	// Match song
	matches, err := service.MatchSong(ctx, tempFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to match song: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"matches": matches,
		"count":   len(matches),
	})
}
