package audio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/utils"
)

type ConvertWAVConfig struct {
	SampleRate int
}

func ConvertToMonoWAV(
	ctx context.Context,
	inputPath string,
	outputDir string,
	cfg ConvertWAVConfig,
) (string, error) {

	if cfg.SampleRate == 0 {
		cfg.SampleRate = 11025
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	if err := utils.MakeDir(outputDir); err != nil {
		return "", err
	}

	baseName := filepath.Base(inputPath)
	outputPath := filepath.Join(outputDir, baseName)

	tmpPath := outputPath + ".tmp.wav"
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-v", "quiet",
		"-i", inputPath,
		"-ac", "1", // mono
		"-ar", fmt.Sprintf("%d", cfg.SampleRate),
		"-c:a", "pcm_s16le",
		tmpPath,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("ffmpeg failed: %v (%s)", err, out)
	}

	if err := utils.MoveFile(tmpPath, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

// YTMetadata contains metadata extracted from YouTube video
type YTMetadata struct {
	ID         string  `json:"id"`          // YouTube video ID
	Title      string  `json:"title"`       // Video title
	Artist     string  `json:"artist"`      // Artist (if available)
	Track      string  `json:"track"`       // Track name (if available)
	Uploader   string  `json:"uploader"`    // Channel uploader
	Channel    string  `json:"channel"`     // Channel name
	Duration   float64 `json:"duration"`    // Duration in seconds
	WebpageURL string  `json:"webpage_url"` // Canonical YouTube URL
}

func pickArtist(meta YTMetadata) string {
	if strings.TrimSpace(meta.Artist) != "" {
		return meta.Artist
	}
	if strings.TrimSpace(meta.Channel) != "" {
		return meta.Channel
	}
	if strings.TrimSpace(meta.Uploader) != "" {
		return meta.Uploader
	}
	return "Unknown Artist"
}

func DownloadYouTubeAudio(ctx context.Context, youtubeURL string, outputDir string, sampleRate int) (audioPath string, metadata *YTMetadata, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
	}

	if sampleRate == 0 {
		sampleRate = 11025
	}

	if err := utils.MakeDir(outputDir); err != nil {
		return "", nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Step 1: Extract metadata using yt-dlp JSON output
	metaCmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"-J",                                // Dump JSON metadata
		"--no-warnings",                     // Suppress warnings
		"--no-playlist",                     // Don't download playlists
		"--cookies-from-browser", "firefox", // Use Firefox cookies for authentication
		"--js-runtimes", "deno", // Use Deno for JavaScript execution
		"--remote-components", "ejs:github", // Use GitHub for remote components
		youtubeURL,
	)

	var stdout, stderr bytes.Buffer
	metaCmd.Stdout = &stdout
	metaCmd.Stderr = &stderr

	if err := metaCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, fmt.Errorf("yt-dlp metadata extraction failed: %v\nstderr: %s", err, stderr.String())
	}

	// Parse metadata
	var ytMeta YTMetadata
	if err := json.Unmarshal(stdout.Bytes(), &ytMeta); err != nil {
		return "", nil, fmt.Errorf("failed to parse yt-dlp JSON: %w", err)
	}

	// Validate required fields
	if strings.TrimSpace(ytMeta.ID) == "" {
		return "", nil, fmt.Errorf("missing video ID in yt-dlp output")
	}
	if strings.TrimSpace(ytMeta.Title) == "" {
		return "", nil, fmt.Errorf("missing title in yt-dlp output")
	}

	// Set artist using fallback chain if not present
	if ytMeta.Artist == "" {
		ytMeta.Artist = pickArtist(ytMeta)
	}

	// Step 2: Download best audio stream (will be converted to proper WAV by service)
	outputTemplate := filepath.Join(outputDir, fmt.Sprintf("%s.%%(ext)s", ytMeta.ID))

	downloadCmd := exec.CommandContext(
		ctx,
		"yt-dlp",
		"-f", "ba", // Best audio stream
		"--no-warnings",                     // Suppress warnings
		"--no-playlist",                     // Don't download playlists
		"--cookies-from-browser", "firefox", // Use Firefox cookies for authentication
		"--js-runtimes", "deno", // Use Deno for JavaScript execution
		"--remote-components", "ejs:github", // Use GitHub for remote components
		"-o", outputTemplate, // Output template
		youtubeURL,
	)

	var dlStderr bytes.Buffer
	downloadCmd.Stderr = &dlStderr

	if err := downloadCmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, fmt.Errorf("yt-dlp download failed: %v\nstderr: %s", err, dlStderr.String())
	}

	// Step 3: Find the downloaded audio file by checking common audio extensions
	audioExtensions := []string{".m4a", ".webm", ".opus", ".mp3", ".aac", ".ogg"}
	var downloadedPath string

	for _, ext := range audioExtensions {
		candidate := filepath.Join(outputDir, ytMeta.ID+ext)
		if _, err := os.Stat(candidate); err == nil {
			downloadedPath = candidate
			break
		}
	}

	if downloadedPath == "" {
		return "", nil, fmt.Errorf("downloaded audio file not found for video %s (checked extensions: %v)", ytMeta.ID, audioExtensions)
	}

	// Return the downloaded audio path - service will convert it to proper WAV format
	return downloadedPath, &ytMeta, nil
}
