package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/himanishpuri/AcousticDNA/internal/storage"
)

// setupTestService creates a test service with a temporary database
func setupTestService(t *testing.T) *AcousticService {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_service_acoustic.sqlite3")

	// Set the environment variable to use our test database
	oldPath := os.Getenv("ACOUSTIC_DB_PATH")
	os.Setenv("ACOUSTIC_DB_PATH", dbPath)
	t.Cleanup(func() {
		if oldPath == "" {
			os.Unsetenv("ACOUSTIC_DB_PATH")
		} else {
			os.Setenv("ACOUSTIC_DB_PATH", oldPath)
		}
	})

	service, err := NewAcousticService()
	if err != nil {
		t.Fatalf("Failed to create test service: %v", err)
	}

	t.Cleanup(func() {
		service.Close()
	})

	return service
}

// getTestAudioFile returns the path to a test audio file
func getTestAudioFile(t *testing.T) string {
	t.Helper()

	// Try to find a test file in the test data directory
	testFile := filepath.Join("..", "..", "test", "convertedtestdata", "Sandstorm-Darude.wav")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test file not found: %s. Run conversion first.", testFile)
	}
	return testFile
}

// getSmallTestAudioFile returns a test audio file
// Note: Currently uses the same file as getTestAudioFile since only Sandstorm-Darude.wav is available
// TODO: Add smaller test files to speed up tests
func getSmallTestAudioFile(t *testing.T) string {
	t.Helper()
	return getTestAudioFile(t)
}

// TestNewAcousticService tests service initialization
func TestNewAcousticService(t *testing.T) {
	service := setupTestService(t)

	if service == nil {
		t.Fatal("Expected non-nil service")
	}

	if service.db == nil {
		t.Fatal("Expected non-nil database client")
	}

	if service.log == nil {
		t.Fatal("Expected non-nil logger")
	}
}

// TestAddSong tests the complete flow of adding a song
func TestAddSong(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	songID, err := service.AddSong(ctx, testFile, "Test Song", "Test Artist", "test_youtube_id")
	if err != nil {
		t.Fatalf("AddSong failed: %v", err)
	}

	if songID == 0 {
		t.Fatal("Expected non-zero song ID")
	}

	// Verify song was registered in database
	var song storage.Song
	if err := service.db.DB.First(&song, songID).Error; err != nil {
		t.Fatalf("Failed to retrieve song from database: %v", err)
	}

	if song.Title != "Test Song" {
		t.Errorf("Expected title 'Test Song', got '%s'", song.Title)
	}

	if song.Artist != "Test Artist" {
		t.Errorf("Expected artist 'Test Artist', got '%s'", song.Artist)
	}

	if song.YouTubeID != "test_youtube_id" {
		t.Errorf("Expected YouTube ID 'test_youtube_id', got '%s'", song.YouTubeID)
	}

	if song.DurationMs <= 0 {
		t.Errorf("Expected positive duration, got %d", song.DurationMs)
	}

	// Verify fingerprints were stored
	var fpCount int64
	service.db.DB.Model(&storage.Fingerprint{}).Where("song_id = ?", songID).Count(&fpCount)
	if fpCount == 0 {
		t.Error("Expected fingerprints to be stored, but found none")
	}

	t.Logf("Successfully added song ID=%d with %d fingerprints", songID, fpCount)
}

// TestAddSongWithContext tests context cancellation
func TestAddSongWithContext(t *testing.T) {
	service := setupTestService(t)
	testFile := getTestAudioFile(t)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.AddSong(ctx, testFile, "Canceled Song", "Canceled Artist", "")
	if err == nil {
		t.Error("Expected error when using canceled context")
	}
}

// TestAddSongInvalidFile tests error handling for invalid audio files
func TestAddSongInvalidFile(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	_, err := service.AddSong(ctx, "/nonexistent/file.wav", "Invalid", "Artist", "")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestAddMultipleSongs tests adding multiple songs to the database
func TestAddMultipleSongs(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	songs := []struct {
		title     string
		artist    string
		youtubeID string
	}{
		{"Song One", "Artist A", "yt_001"},
		{"Song Two", "Artist B", "yt_002"},
		{"Song Three", "Artist A", "yt_003"},
	}

	addedIDs := make([]uint32, 0, len(songs))

	for _, s := range songs {
		songID, err := service.AddSong(ctx, testFile, s.title, s.artist, s.youtubeID)
		if err != nil {
			t.Fatalf("Failed to add song '%s': %v", s.title, err)
		}
		addedIDs = append(addedIDs, songID)
		t.Logf("Added song '%s' with ID=%d", s.title, songID)
	}

	// Verify all songs are in the database
	var songCount int64
	service.db.DB.Model(&storage.Song{}).Count(&songCount)
	if songCount != int64(len(songs)) {
		t.Errorf("Expected %d songs in database, found %d", len(songs), songCount)
	}

	// Verify all IDs are unique
	uniqueIDs := make(map[uint32]bool)
	for _, id := range addedIDs {
		if uniqueIDs[id] {
			t.Errorf("Duplicate song ID found: %d", id)
		}
		uniqueIDs[id] = true
	}
}

// TestMatchSong tests the complete matching flow
func TestMatchSong(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// First, add a song to the database
	songID, err := service.AddSong(ctx, testFile, "Original Song", "Original Artist", "orig_yt_id")
	if err != nil {
		t.Fatalf("Failed to add song: %v", err)
	}

	t.Logf("Added song with ID=%d", songID)

	// Now try to match the same audio file
	results, err := service.MatchSong(ctx, testFile)
	if err != nil {
		t.Fatalf("MatchSong failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Expected at least one match result")
	}

	// The first result should be our song
	topMatch := results[0]
	if topMatch.SongID != songID {
		t.Errorf("Expected top match to be song ID=%d, got ID=%d", songID, topMatch.SongID)
	}

	if topMatch.Title != "Original Song" {
		t.Errorf("Expected title 'Original Song', got '%s'", topMatch.Title)
	}

	if topMatch.Artist != "Original Artist" {
		t.Errorf("Expected artist 'Original Artist', got '%s'", topMatch.Artist)
	}

	if topMatch.Score <= 0 {
		t.Errorf("Expected positive score, got %d", topMatch.Score)
	}

	if topMatch.Confidence <= 0 {
		t.Errorf("Expected positive confidence, got %f", topMatch.Confidence)
	}

	t.Logf("Match result: Score=%d, Confidence=%.2f%%, OffsetMs=%d",
		topMatch.Score, topMatch.Confidence, topMatch.OffsetMs)
}

// TestMatchSongWithoutDatabase tests matching when database is empty
func TestMatchSongWithoutDatabase(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Try to match without adding any songs first
	results, err := service.MatchSong(ctx, testFile)
	if err != nil {
		t.Fatalf("MatchSong failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected no matches with empty database, got %d", len(results))
	}
}

// TestMatchSongInvalidFile tests error handling for invalid audio files
func TestMatchSongInvalidFile(t *testing.T) {
	service := setupTestService(t)

	ctx := context.Background()

	_, err := service.MatchSong(ctx, "/nonexistent/file.wav")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

// TestEndToEndFlow tests the complete flow: Add → Match → Verify
func TestEndToEndFlow(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Step 1: Add a song
	t.Log("Step 1: Adding song to database...")
	songID, err := service.AddSong(ctx, testFile, "E2E Test Song", "E2E Artist", "e2e_yt")
	if err != nil {
		t.Fatalf("Failed to add song: %v", err)
	}
	t.Logf("✓ Song added with ID=%d", songID)

	// Step 2: Verify fingerprints were stored
	var fpCount int64
	service.db.DB.Model(&storage.Fingerprint{}).Where("song_id = ?", songID).Count(&fpCount)
	t.Logf("✓ Stored %d fingerprints", fpCount)

	if fpCount == 0 {
		t.Fatal("No fingerprints stored")
	}

	// Step 3: Match the same audio
	t.Log("Step 2: Matching audio...")
	results, err := service.MatchSong(ctx, testFile)
	if err != nil {
		t.Fatalf("Failed to match song: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("No matches found")
	}
	t.Logf("✓ Found %d matches", len(results))

	// Step 4: Verify the match
	topMatch := results[0]
	t.Logf("Top match: ID=%d, Title='%s', Score=%d, Confidence=%.2f%%",
		topMatch.SongID, topMatch.Title, topMatch.Score, topMatch.Confidence)

	if topMatch.SongID != songID {
		t.Errorf("Top match ID mismatch: expected %d, got %d", songID, topMatch.SongID)
	}

	// High confidence threshold for exact match
	if topMatch.Confidence < 10.0 {
		t.Errorf("Expected high confidence for exact match, got %.2f%%", topMatch.Confidence)
	}

	t.Log("✓ End-to-end flow completed successfully")
}

// TestGetSongByID tests the internal getSongByID method
func TestGetSongByID(t *testing.T) {
	service := setupTestService(t)
	testFile := getSmallTestAudioFile(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Add a song
	songID, err := service.AddSong(ctx, testFile, "GetByID Test", "GetByID Artist", "getbyid_yt")
	if err != nil {
		t.Fatalf("Failed to add song: %v", err)
	}

	// Retrieve it using the internal method
	song, err := service.GetSongByID(songID)
	if err != nil {
		t.Fatalf("getSongByID failed: %v", err)
	}

	if song == nil {
		t.Fatal("Expected non-nil song")
	}

	if song.Title != "GetByID Test" {
		t.Errorf("Expected title 'GetByID Test', got '%s'", song.Title)
	}

	// Test with non-existent ID
	_, err = service.GetSongByID(99999)
	if err == nil {
		t.Error("Expected error for non-existent song ID")
	}
}

// TestAddSongRollback tests that fingerprints are rolled back when storage fails
func TestAddSongRollback(t *testing.T) {
	// This test would require mocking the database to simulate a storage failure
	// For now, we'll test the happy path and verify the rollback method exists
	service := setupTestService(t)

	// Verify the DeleteSongByID method exists by checking we can call it
	// (even though it will fail for a non-existent ID)
	err := service.db.DeleteSongByID(99999)
	// We expect this to complete without panic, error is acceptable
	t.Logf("DeleteSongByID test completed (error expected): %v", err)
}

// TestMatchResultStructure tests the MatchResult structure
func TestMatchResultStructure(t *testing.T) {
	result := MatchResult{
		SongID:     123,
		Title:      "Test Title",
		Artist:     "Test Artist",
		YouTubeID:  "test_yt_id",
		Score:      100,
		OffsetMs:   5000,
		Confidence: 85.5,
	}

	if result.SongID != 123 {
		t.Errorf("Expected SongID 123, got %d", result.SongID)
	}

	if result.Confidence != 85.5 {
		t.Errorf("Expected Confidence 85.5, got %f", result.Confidence)
	}
}

// BenchmarkAddSong benchmarks the AddSong operation
func BenchmarkAddSong(b *testing.B) {
	// Setup
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench_acoustic.sqlite3")
	os.Setenv("ACOUSTIC_DB_PATH", dbPath)
	defer os.Unsetenv("ACOUSTIC_DB_PATH")

	service, err := NewAcousticService()
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}
	defer service.Close()

	testFile := filepath.Join("..", "..", "test", "convertedtestdata", "CityBGM-kimurasukuru.wav")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skip("Test file not found")
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		songID, err := service.AddSong(ctx, testFile, "Bench Song", "Bench Artist", "bench_yt")
		if err != nil {
			b.Fatalf("AddSong failed: %v", err)
		}
		// Clean up after each iteration (not counted in benchmark)
		b.StopTimer()
		service.db.DeleteSongByID(songID)
		b.StartTimer()
	}
}

// BenchmarkMatchSong benchmarks the MatchSong operation
func BenchmarkMatchSong(b *testing.B) {
	// Setup
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench_match_acoustic.sqlite3")
	os.Setenv("ACOUSTIC_DB_PATH", dbPath)
	defer os.Unsetenv("ACOUSTIC_DB_PATH")

	service, err := NewAcousticService()
	if err != nil {
		b.Fatalf("Failed to create service: %v", err)
	}
	defer service.Close()

	testFile := filepath.Join("..", "..", "test", "convertedtestdata", "CityBGM-kimurasukuru.wav")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		b.Skip("Test file not found")
	}

	ctx := context.Background()

	// Add a song to the database first
	_, err = service.AddSong(ctx, testFile, "Match Bench Song", "Match Bench Artist", "match_bench_yt")
	if err != nil {
		b.Fatalf("Failed to add song: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.MatchSong(ctx, testFile)
		if err != nil {
			b.Fatalf("MatchSong failed: %v", err)
		}
	}
}
