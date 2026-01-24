//go:build !js && !wasm
// +build !js,!wasm
package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna"
)

var (
	port           int
	dbPath         string
	tempDir        string
	sampleRate     int
	allowedOrigins string
)

func init() {
	flag.IntVar(&port, "port", 8080, "HTTP server port")
	flag.StringVar(&dbPath, "db", getEnvOrDefault("ACOUSTIC_DB_PATH", "acousticdna.sqlite3"), "Path to SQLite database")
	flag.StringVar(&tempDir, "temp", getEnvOrDefault("ACOUSTIC_TEMP_DIR", "/tmp"), "Temporary directory")
	flag.IntVar(&sampleRate, "rate", 11025, "Audio sample rate")
	flag.StringVar(&allowedOrigins, "origins", "*", "Comma-separated list of allowed CORS origins (use * for all)")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	flag.Parse()

	// Parse allowed origins
	var origins []string
	if allowedOrigins == "*" {
		origins = []string{"*"}
	} else {
		origins = strings.Split(allowedOrigins, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}

	// Create AcousticDNA service
	service, err := acousticdna.NewService(
		acousticdna.WithDBPath(dbPath),
		acousticdna.WithTempDir(tempDir),
		acousticdna.WithSampleRate(sampleRate),
	)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}
	defer service.Close()

	// Create server configuration
	config := &ServerConfig{
		Port:           port,
		DBPath:         dbPath,
		TempDir:        tempDir,
		SampleRate:     sampleRate,
		AllowedOrigins: origins,
	}

	// Create and start server
	server := NewServer(service, config)
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
