package utils

import (
	"fmt"
	"net/url"
	"strings"
)

// ExtractYouTubeID extracts the video ID from a YouTube URL.
// Supports various YouTube URL formats:
//   - https://www.youtube.com/watch?v=VIDEO_ID
//   - https://youtube.com/watch?v=VIDEO_ID
//   - https://youtu.be/VIDEO_ID
//   - https://www.youtube.com/embed/VIDEO_ID
//   - https://m.youtube.com/watch?v=VIDEO_ID
func ExtractYouTubeID(youtubeURL string) (string, error) {
	// Parse the URL
	u, err := url.Parse(youtubeURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Handle youtu.be short links
	if strings.Contains(u.Host, "youtu.be") {
		// Path is /VIDEO_ID
		id := strings.TrimPrefix(u.Path, "/")
		// Remove any query parameters
		if idx := strings.Index(id, "?"); idx != -1 {
			id = id[:idx]
		}
		if id != "" {
			return id, nil
		}
		return "", fmt.Errorf("no video ID found in youtu.be URL")
	}

	// Handle youtube.com URLs
	if strings.Contains(u.Host, "youtube.com") {
		// Check for /watch?v=VIDEO_ID format
		if u.Path == "/watch" || strings.HasPrefix(u.Path, "/watch") {
			query := u.Query()
			if videoID := query.Get("v"); videoID != "" {
				return videoID, nil
			}
		}

		// Check for /embed/VIDEO_ID format
		if strings.HasPrefix(u.Path, "/embed/") {
			id := strings.TrimPrefix(u.Path, "/embed/")
			if id != "" {
				return id, nil
			}
		}

		// Check for /v/VIDEO_ID format
		if strings.HasPrefix(u.Path, "/v/") {
			id := strings.TrimPrefix(u.Path, "/v/")
			if id != "" {
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("unable to extract video ID from URL: %s", youtubeURL)
}

// IsYouTubeURL checks if a URL is a valid YouTube URL
func IsYouTubeURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	return strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be")
}
