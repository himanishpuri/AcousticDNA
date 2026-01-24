package reference_scripts

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type YTDLPInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`   // sometimes present
	Track    string `json:"track"`    // sometimes present
	Uploader string `json:"uploader"` // usually present
	Channel  string `json:"channel"`  // sometimes present
}

type Song struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	YTID   string `json:"yt_id"`
}

// stable internal id generator (you can replace with UUID if you want)
func makeSongID(ytid string) string {
	h := sha1.Sum([]byte("song:" + ytid))
	return hex.EncodeToString(h[:])
}

func pickArtist(info YTDLPInfo) string {
	// best -> worst fallback
	if strings.TrimSpace(info.Artist) != "" {
		return info.Artist
	}
	if strings.TrimSpace(info.Channel) != "" {
		return info.Channel
	}
	if strings.TrimSpace(info.Uploader) != "" {
		return info.Uploader
	}
	return "Unknown Artist"
}

func fetchSongFromYT(url string) (Song, error) {
	cmd := exec.Command("yt-dlp",
		"-J",
		"--no-warnings",
		"--no-playlist",
		url,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return Song{}, fmt.Errorf("yt-dlp failed: %v\nstderr:\n%s", err, stderr.String())
	}

	var info YTDLPInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return Song{}, fmt.Errorf("failed parsing yt-dlp JSON: %v", err)
	}

	// guarantee yt_id + title exist
	if strings.TrimSpace(info.ID) == "" {
		return Song{}, fmt.Errorf("missing yt id in yt-dlp output")
	}
	if strings.TrimSpace(info.Title) == "" {
		return Song{}, fmt.Errorf("missing title in yt-dlp output")
	}

	s := Song{
		ID:     makeSongID(info.ID), // your internal ID
		Title:  info.Title,
		Artist: pickArtist(info),
		YTID:   info.ID,
	}

	return s, nil
}

func main() {
	url := "https://www.youtube.com/watch?v=E3Vlhj21ep0"

	song, err := fetchSongFromYT(url)
	if err != nil {
		panic(err)
	}

	out, _ := json.MarshalIndent(song, "", "  ")
	fmt.Println(string(out))
}
