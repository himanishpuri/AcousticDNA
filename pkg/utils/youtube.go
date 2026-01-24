package utils

import (
	"fmt"
	"net/url"
	"strings"
)

func ExtractYouTubeID(youtubeURL string) (string, error) {
	u, err := url.Parse(youtubeURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if strings.Contains(u.Host, "youtu.be") {
		id := strings.TrimPrefix(u.Path, "/")
		if idx := strings.Index(id, "?"); idx != -1 {
			id = id[:idx]
		}
		if id != "" {
			return id, nil
		}
		return "", fmt.Errorf("no video ID found in youtu.be URL")
	}

	if strings.Contains(u.Host, "youtube.com") {
		if u.Path == "/watch" || strings.HasPrefix(u.Path, "/watch") {
			query := u.Query()
			if videoID := query.Get("v"); videoID != "" {
				return videoID, nil
			}
		}

		if strings.HasPrefix(u.Path, "/embed/") {
			id := strings.TrimPrefix(u.Path, "/embed/")
			if id != "" {
				return id, nil
			}
		}

		if strings.HasPrefix(u.Path, "/v/") {
			id := strings.TrimPrefix(u.Path, "/v/")
			if id != "" {
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("unable to extract video ID from URL: %s", youtubeURL)
}

func IsYouTubeURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	return strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be")
}
