package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/himanishpuri/AcousticDNA/pkg/logger"
)

func (s *Server) setupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Serve static web files
	fs := http.FileServer(http.Dir("./web/public"))
	mux.Handle("/", fs)

	// Health endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/health/metrics", s.handleMetrics)

	// Song management endpoints
	mux.HandleFunc("/api/songs", s.handleSongs)
	mux.HandleFunc("/api/songs/", s.handleSong)
	mux.HandleFunc("/api/songs/youtube", s.handleAddSongYouTube)

	// Match endpoints
	mux.HandleFunc("/api/match", s.handleMatch)
	mux.HandleFunc("/api/match/hashes", s.handleMatchHashesRoute)

	// Wrap with CORS middleware
	return corsMiddleware(s.config.AllowedOrigins)(mux)
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			allowed := false
			if len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*") {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				allowed = true
			} else {
				for _, allowedOrigin := range allowedOrigins {
					if allowedOrigin == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						allowed = true
						break
					}
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				w.Header().Set("Access-Control-Max-Age", "3600")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		logger := logger.GetLogger()
		logger.Infof("%s %s from %s", r.Method, r.URL.Path, getClientIP(r))

		next.ServeHTTP(wrapped, r)

		logger.Infof("%s %s -> %d", r.Method, r.URL.Path, wrapped.statusCode)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func (s *Server) Start() error {
	handler := s.setupRoutes()

	// Optionally wrap with logging middleware
	// handler = loggingMiddleware(handler)

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.log.Infof("🚀 AcousticDNA server starting on %s", addr)
	s.log.Infof("   Database: %s", s.config.DBPath)
	s.log.Infof("   Sample Rate: %d Hz", s.config.SampleRate)
	s.log.Infof("   CORS Origins: %v", s.config.AllowedOrigins)
	s.log.Infof("\nEndpoints:")
	s.log.Infof("   GET    /health                  - Health check")
	s.log.Infof("   GET    /api/health/metrics      - Server metrics")
	s.log.Infof("   GET    /api/songs               - List all songs")
	s.log.Infof("   POST   /api/songs               - Add song from file")
	s.log.Infof("   POST   /api/songs/youtube       - Add song from YouTube URL")
	s.log.Infof("   GET    /api/songs/{id}          - Get song by ID")
	s.log.Infof("   DELETE /api/songs/{id}          - Delete song by ID")
	s.log.Infof("   POST   /api/match               - Match audio file")
	s.log.Infof("   POST   /api/match/hashes        - Match pre-computed hashes (WASM)")

	return http.ListenAndServe(addr, handler)
}
