package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/himanishpuri/AcousticDNA/pkg/logger"
	"github.com/himanishpuri/AcousticDNA/pkg/utils"
)

// Global flags
var (
	dbPath     string
	tempDir    string
	sampleRate int
)

func init() {
	// Global flags that can be used with any command
	flag.StringVar(&dbPath, "db", getEnvOrDefault("ACOUSTIC_DB_PATH", "acousticdna.sqlite3"), "Path to the SQLite database file")
	flag.StringVar(&tempDir, "temp", getEnvOrDefault("ACOUSTIC_TEMP_DIR", "/tmp"), "Directory for temporary audio conversion files")
	flag.IntVar(&sampleRate, "rate", 11025, "Audio sample rate for processing")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// createService creates a new AcousticDNA service with configured options
func createService() (acousticdna.Service, error) {
	return acousticdna.NewService(
		acousticdna.WithDBPath(dbPath),
		acousticdna.WithTempDir(tempDir),
		acousticdna.WithSampleRate(sampleRate),
	)
}

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
	title := addCmd.String("title", "", "Song title (required unless using --youtube-url)")
	artist := addCmd.String("artist", "", "Artist name (required unless using --youtube-url)")
	youtube := addCmd.String("youtube", "", "YouTube ID (optional)")
	youtubeURL := addCmd.String("youtube-url", "", "YouTube URL to download and add (alternative to audio file)")

	addCmd.Parse(flagArgs)

	// Determine if we're using YouTube URL or local file
	var isYouTubeMode bool
	if *youtubeURL != "" {
		isYouTubeMode = true
		if audioPath != "" {
			fmt.Println("Error: cannot specify both audio file and --youtube-url")
			log.Error("Both audio file and --youtube-url specified")
			os.Exit(1)
		}
	} else if audioPath == "" {
		fmt.Println("Error: audio file path or --youtube-url required")
		fmt.Println("Usage: acousticDNA add <audio_file> --title <title> --artist <artist> [--youtube <id>]")
		fmt.Println("   OR: acousticDNA add --youtube-url <url> [--title <title>] [--artist <artist>]")
		os.Exit(1)
	}

	// Declare service and error variables for use throughout function
	var svc acousticdna.Service
	var err error

	// Handle YouTube download mode
	if isYouTubeMode {
		log.Infof("YouTube mode: downloading from URL: %s", *youtubeURL)

		fmt.Println("\n🔧 Initializing service...")
		svc, err = createService()
		if err != nil {
			fmt.Printf("❌ Failed to create service: %v\n", err)
			log.Errorf("Service initialization failed: %v", err)
			os.Exit(1)
		}
		defer svc.Close()

		fmt.Println("📥 Downloading audio from YouTube...")
		fmt.Println("   This may take a few moments depending on video length")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Download YouTube audio (service will convert to WAV)
		downloadedPath, ytMeta, err := audio.DownloadYouTubeAudio(ctx, *youtubeURL, tempDir, sampleRate)
		if err != nil {
			fmt.Printf("\n❌ Failed to download YouTube video: %v\n", err)
			log.Errorf("YouTube download failed: %v", err)
			os.Exit(1)
		}

		// Use metadata from YouTube if not provided by user
		if *title == "" {
			*title = ytMeta.Title
			log.Infof("Using YouTube title: %s", *title)
		}
		if *artist == "" {
			*artist = ytMeta.Artist
			log.Infof("Using YouTube artist: %s", *artist)
		}

		// Extract YouTube ID from URL
		if *youtube == "" {
			ytID, err := utils.ExtractYouTubeID(*youtubeURL)
			if err != nil {
				log.Warnf("Failed to extract YouTube ID: %v", err)
			} else {
				*youtube = ytID
				log.Infof("Extracted YouTube ID: %s", ytID)
			}
		}

		// Validate we have title and artist
		if *title == "" || *artist == "" {
			fmt.Println("Error: Could not determine title or artist from YouTube metadata")
			fmt.Println("Please provide --title and --artist explicitly")
			log.Error("Missing title or artist after YouTube download")
			os.Exit(1)
		}

		audioPath = downloadedPath
		fmt.Printf("✅ Downloaded: %s by %s\n", *title, *artist)
	} else {
		// Local file mode - validate required fields
		if *title == "" || *artist == "" {
			fmt.Println("Error: --title and --artist are required")
			log.Warn("Missing required arguments: title and artist")
			os.Exit(1)
		}
	}

	log.Infof("Adding song: '%s' by '%s' from file: %s", *title, *artist, audioPath)

	// Create service for local file mode (YouTube mode already created it)
	if !isYouTubeMode {
		fmt.Println("\n🔧 Initializing service...")
		svc, err = createService()
		if err != nil {
			fmt.Printf("❌ Failed to create service: %v\n", err)
			log.Errorf("Service initialization failed: %v", err)
			os.Exit(1)
		}
		defer svc.Close()
	}

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
	svc, err := createService()
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

	svc, err := createService()
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

	svc, err := createService()
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
	fmt.Println("\nGlobal Options:")
	fmt.Println("  --db <path>        Path to SQLite database (env: ACOUSTIC_DB_PATH, default: acousticdna.sqlite3)")
	fmt.Println("  --temp <dir>       Temporary directory for audio conversion (env: ACOUSTIC_TEMP_DIR, default: /tmp)")
	fmt.Println("  --rate <hz>        Audio sample rate (default: 11025)")
	fmt.Println("\nUsage:")
	fmt.Println("  acousticDNA [global-options] add <audio_file> --title <title> --artist <artist> [--youtube <id>]")
	fmt.Println("  acousticDNA [global-options] add --youtube-url <url> [--title <title>] [--artist <artist>]")
	fmt.Println("  acousticDNA [global-options] match <audio_file>")
	fmt.Println("  acousticDNA [global-options] list")
	fmt.Println("  acousticDNA [global-options] delete <song_id>")
	fmt.Println("\nExamples:")
	fmt.Println("  # Add from local file")
	fmt.Println("  acousticDNA --db mydb.sqlite3 add song.mp3 --title \"Song\" --artist \"Artist\"")
	fmt.Println()
	fmt.Println("  # Add from YouTube URL (auto-detects metadata)")
	fmt.Println("  acousticDNA add --youtube-url \"https://youtube.com/watch?v=dQw4w9WgXcQ\"")
	fmt.Println()
	fmt.Println("  # Add from YouTube URL with custom metadata")
	fmt.Println("  acousticDNA add --youtube-url \"https://youtu.be/dQw4w9WgXcQ\" --title \"Custom Title\" --artist \"Custom Artist\"")
	fmt.Println()
	fmt.Println("  # Match audio file")
	fmt.Println("  acousticDNA --rate 22050 match query.mp3")
}
