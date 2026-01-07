package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/himanishpuri/AcousticDNA/internal/service"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
)

func main() {
	// Initialize logger
	log := logger.GetLogger()

	// Print banner
	printBanner()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	log.Infof("Executing command: %s", command)

	switch command {
	case "add":
		handleAdd()
	case "match":
		handleMatch()
	case "list":
		handleList()
	case "delete":
		handleDelete()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printBanner() {
	banner := `
   _                      _   _      ____  _   _    _    
  / \   ___ ___  _   _ ___| |_(_) ___|  _ \| \ | |  / \   
 / _ \ / __/ _ \| | | / __| __| |/ __| | | |  \| | / _ \  
/ ___ \ (_| (_) | |_| \__ \ |_| | (__| |_| | |\  |/ ___ \ 
\_/   \_/___\___/ \__,_|___/\__|_|\___|____/|_| \_/_/   \_/
                                                            
           Audio Fingerprinting CLI Tool
`
	fmt.Println(banner)
}

func handleAdd() {
	log := logger.GetLogger()

	// Manually extract audio file and flags
	args := os.Args[2:]
	var audioPath string
	var flagArgs []string

	// Separate the audio file path from flags
	for i, arg := range args {
		if !strings.HasPrefix(arg, "-") && audioPath == "" {
			audioPath = arg
		} else {
			flagArgs = append(flagArgs, args[i:]...)
			break
		}
	}

	// Parse flags
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	title := addCmd.String("title", "", "Song title (required)")
	artist := addCmd.String("artist", "", "Artist name (required)")
	youtube := addCmd.String("youtube", "", "YouTube ID (optional)")

	addCmd.Parse(flagArgs)

	if audioPath == "" {
		fmt.Println("Error: audio file path required")
		fmt.Println("Usage: acousticDNA add <audio_file> --title <title> --artist <artist> [--youtube <id>]")
		os.Exit(1)
	}

	if *title == "" || *artist == "" {
		fmt.Println("Error: --title and --artist are required")
		log.Warn("Missing required arguments: title and artist")
		os.Exit(1)
	}

	log.Infof("Adding song: '%s' by '%s' from file: %s", *title, *artist, audioPath)

	// Create service
	fmt.Println("\n🔧 Initializing service...")
	svc, err := service.NewAcousticService()
	if err != nil {
		fmt.Printf("❌ Failed to create service: %v\n", err)
		log.Errorf("Service initialization failed: %v", err)
		os.Exit(1)
	}
	defer svc.Close()

	// Add song
	fmt.Println("🎵 Processing audio file...")
	fmt.Println("   This may take a few moments for large files")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	songID, err := svc.AddSong(ctx, audioPath, *title, *artist, *youtube)
	if err != nil {
		fmt.Printf("\n❌ Failed to add song: %v\n", err)
		log.Errorf("AddSong failed: %v", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Successfully added song to database!")
	fmt.Printf("   ID:      %d\n", songID)
	fmt.Printf("   Title:   %s\n", *title)
	fmt.Printf("   Artist:  %s\n", *artist)
	if *youtube != "" {
		fmt.Printf("   YouTube: %s\n", *youtube)
	}
	log.Infof("Successfully added song ID=%d", songID)
}

func handleMatch() {
	log := logger.GetLogger()

	if len(os.Args) < 3 {
		fmt.Println("Usage: acousticDNA match <audio_file>")
		os.Exit(1)
	}

	audioPath := os.Args[2]
	log.Infof("Matching audio file: %s", audioPath)

	fmt.Println("\n🔧 Initializing service...")
	svc, err := service.NewAcousticService()
	if err != nil {
		fmt.Printf("❌ Failed to create service: %v\n", err)
		log.Errorf("Service initialization failed: %v", err)
		os.Exit(1)
	}
	defer svc.Close()

	fmt.Println("🔍 Analyzing audio file...")
	fmt.Println("   Generating fingerprints and searching database")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	results, err := svc.MatchSong(ctx, audioPath)
	if err != nil {
		fmt.Printf("\n❌ Failed to match song: %v\n", err)
		log.Errorf("MatchSong failed: %v", err)
		os.Exit(1)
	}

	log.Infof("Match complete: found %d results", len(results))

	if len(results) == 0 {
		fmt.Println("\n❌ No matches found in database")
		log.Info("No matches found")
		return
	}

	fmt.Printf("\n✅ Found %d match(es)!\n", len(results))
	fmt.Println("\n🎵 Top Matches:")
	fmt.Println()

	maxDisplay := 10
	if len(results) < maxDisplay {
		maxDisplay = len(results)
	}

	for i := 0; i < maxDisplay; i++ {
		result := results[i]
		fmt.Printf("%d. \"%s\" by %s\n", i+1, result.Title, result.Artist)
		fmt.Printf("   Score: %d | Confidence: %.1f%% | Offset: %dms\n",
			result.Score, result.Confidence, result.OffsetMs)
		if result.YouTubeID != "" {
			fmt.Printf("   YouTube: https://youtube.com/watch?v=%s\n", result.YouTubeID)
		}
		fmt.Println()
	}

	if len(results) > maxDisplay {
		fmt.Printf("... and %d more matches\n", len(results)-maxDisplay)
	}
}

func handleList() {
	log := logger.GetLogger()

	svc, err := service.NewAcousticService()
	if err != nil {
		fmt.Printf("❌ Failed to create service: %v\n", err)
		log.Errorf("Service initialization failed: %v", err)
		os.Exit(1)
	}
	defer svc.Close()

	songs, err := svc.ListSongs()
	if err != nil {
		fmt.Printf("❌ Failed to list songs: %v\n", err)
		log.Errorf("ListSongs failed: %v", err)
		os.Exit(1)
	}

	if len(songs) == 0 {
		fmt.Println("\n📭 No songs in database")
		log.Info("No songs in database")
		return
	}

	fmt.Printf("\n📚 Found %d song(s):\n\n", len(songs))
	for i, song := range songs {
		fmt.Printf("%d. \"%s\" by %s (ID: %d)\n", i+1, song.Title, song.Artist, song.ID)
		if song.YouTubeID != "" {
			fmt.Printf("   YouTube: https://youtube.com/watch?v=%s\n", song.YouTubeID)
		}
		if song.DurationMs > 0 {
			duration := song.DurationMs / 1000
			fmt.Printf("   Duration: %d:%02d\n", duration/60, duration%60)
		}
		fmt.Println()
	}
	log.Infof("Listed %d songs", len(songs))
}

func handleDelete() {
	log := logger.GetLogger()

	if len(os.Args) < 3 {
		fmt.Println("Usage: acousticDNA delete <song_id>")
		os.Exit(1)
	}

	songID, err := strconv.ParseUint(os.Args[2], 10, 32)
	if err != nil {
		fmt.Printf("❌ Invalid song ID: %v\n", err)
		log.Errorf("Invalid song ID: %v", err)
		os.Exit(1)
	}

	svc, err := service.NewAcousticService()
	if err != nil {
		fmt.Printf("❌ Failed to create service: %v\n", err)
		log.Errorf("Service initialization failed: %v", err)
		os.Exit(1)
	}
	defer svc.Close()

	// Get song info before deletion
	song, err := svc.GetSongByID(uint32(songID))
	if err != nil {
		fmt.Printf("❌ Song not found (ID: %d)\n", songID)
		log.Warnf("Song %d not found: %v", songID, err)
		os.Exit(1)
	}

	// Delete
	if err := svc.DeleteSong(uint32(songID)); err != nil {
		fmt.Printf("❌ Failed to delete song: %v\n", err)
		log.Errorf("DeleteSong failed: %v", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Successfully deleted song:\n")
	fmt.Printf("   ID:     %d\n", song.ID)
	fmt.Printf("   Title:  %s\n", song.Title)
	fmt.Printf("   Artist: %s\n", song.Artist)
	log.Infof("Deleted song ID=%d ('%s' by '%s')", song.ID, song.Title, song.Artist)
}

func printUsage() {
	fmt.Println("AcousticDNA - Audio Fingerprinting CLI")
	fmt.Println("\nUsage:")
	fmt.Println("  acousticDNA add <audio_file> --title <title> --artist <artist> [--youtube <id>]")
	fmt.Println("  acousticDNA match <audio_file>")
	fmt.Println("  acousticDNA list")
	fmt.Println("  acousticDNA delete <song_id>")
}
