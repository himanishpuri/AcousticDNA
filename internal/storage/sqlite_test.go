package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/himanishpuri/AcousticDNA/internal/model"
)

// Helper function to create a temporary test database
func setupTestDB(t *testing.T) (*DBClient, string) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_acoustic.sqlite3")

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

	client, err := NewDBClient()
	if err != nil {
		t.Fatalf("Failed to create test DB client: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client, dbPath
}

// TestNewDBClient tests database initialization
func TestNewDBClient(t *testing.T) {
	client, dbPath := setupTestDB(t)

	if client == nil {
		t.Fatal("Expected non-nil DB client")
	}

	if client.DB == nil {
		t.Fatal("Expected non-nil GORM DB handle")
	}

	if client.db == nil {
		t.Fatal("Expected non-nil sql.DB handle")
	}

	// Verify the database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created at %s", dbPath)
	}
}

// TestNewDBClientWithCustomPath tests database creation with custom path
func TestNewDBClientWithCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	customPath := filepath.Join(tmpDir, "subdir", "custom.db")

	oldPath := os.Getenv("ACOUSTIC_DB_PATH")
	os.Setenv("ACOUSTIC_DB_PATH", customPath)
	defer func() {
		if oldPath == "" {
			os.Unsetenv("ACOUSTIC_DB_PATH")
		} else {
			os.Setenv("ACOUSTIC_DB_PATH", oldPath)
		}
	}()

	client, err := NewDBClient()
	if err != nil {
		t.Fatalf("Failed to create DB with custom path: %v", err)
	}
	defer client.Close()

	if _, err := os.Stat(customPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created at custom path %s", customPath)
	}
}

// TestRegisterSong tests song registration
func TestRegisterSong(t *testing.T) {
	client, _ := setupTestDB(t)

	songID, err := client.RegisterSong("Test Song", "Test Artist", "youtube123", 180000)
	if err != nil {
		t.Fatalf("Failed to register song: %v", err)
	}

	if songID == 0 {
		t.Error("Expected non-zero song ID")
	}

	// Verify the song was stored
	var song Song
	result := client.DB.First(&song, songID)
	if result.Error != nil {
		t.Fatalf("Failed to retrieve registered song: %v", result.Error)
	}

	if song.Title != "Test Song" {
		t.Errorf("Expected title 'Test Song', got '%s'", song.Title)
	}
	if song.Artist != "Test Artist" {
		t.Errorf("Expected artist 'Test Artist', got '%s'", song.Artist)
	}
	if song.YouTubeID != "youtube123" {
		t.Errorf("Expected YouTubeID 'youtube123', got '%s'", song.YouTubeID)
	}
	if song.DurationMs != 180000 {
		t.Errorf("Expected duration 180000, got %d", song.DurationMs)
	}
}

// TestRegisterSongIdempotent tests that registering the same song twice returns the same ID
func TestRegisterSongIdempotent(t *testing.T) {
	client, _ := setupTestDB(t)

	songID1, err := client.RegisterSong("Duplicate Song", "Duplicate Artist", "yt1", 120000)
	if err != nil {
		t.Fatalf("Failed to register song first time: %v", err)
	}

	songID2, err := client.RegisterSong("Duplicate Song", "Duplicate Artist", "yt2", 120000)
	if err != nil {
		t.Fatalf("Failed to register song second time: %v", err)
	}

	if songID1 != songID2 {
		t.Errorf("Expected same song ID for duplicate registration, got %d and %d", songID1, songID2)
	}

	// Verify only one song exists
	var count int64
	client.DB.Model(&Song{}).Where("title = ? AND artist = ?", "Duplicate Song", "Duplicate Artist").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 song in database, found %d", count)
	}
}

// TestRegisterSongUpdatesYouTubeID tests that missing YouTube ID gets updated
func TestRegisterSongUpdatesYouTubeID(t *testing.T) {
	client, _ := setupTestDB(t)

	// Register without YouTube ID
	songID1, err := client.RegisterSong("Update Test", "Update Artist", "", 150000)
	if err != nil {
		t.Fatalf("Failed to register song: %v", err)
	}

	// Register again with YouTube ID
	songID2, err := client.RegisterSong("Update Test", "Update Artist", "newYouTubeID", 150000)
	if err != nil {
		t.Fatalf("Failed to register song with YouTube ID: %v", err)
	}

	if songID1 != songID2 {
		t.Errorf("Expected same song ID, got %d and %d", songID1, songID2)
	}

	// Verify YouTube ID was updated
	var song Song
	client.DB.First(&song, songID1)
	if song.YouTubeID != "newYouTubeID" {
		t.Errorf("Expected YouTubeID to be updated to 'newYouTubeID', got '%s'", song.YouTubeID)
	}
}

// TestDeleteSongByID tests song deletion
func TestDeleteSongByID(t *testing.T) {
	client, _ := setupTestDB(t)

	// Register a song
	songID, err := client.RegisterSong("To Delete", "Delete Artist", "yt_del", 100000)
	if err != nil {
		t.Fatalf("Failed to register song: %v", err)
	}

	// Delete the song
	err = client.DeleteSongByID(songID)
	if err != nil {
		t.Fatalf("Failed to delete song: %v", err)
	}

	// Verify the song was deleted
	var song Song
	result := client.DB.First(&song, songID)
	if result.Error == nil {
		t.Error("Expected song to be deleted, but it still exists")
	}
}

// TestDeleteSongWithFingerprints tests cascading deletion of fingerprints
func TestDeleteSongWithFingerprints(t *testing.T) {
	client, _ := setupTestDB(t)

	// Register a song
	songID, err := client.RegisterSong("Song With Prints", "Print Artist", "yt_fp", 200000)
	if err != nil {
		t.Fatalf("Failed to register song: %v", err)
	}

	// Store some fingerprints for this song
	fingerprints := map[uint32][]model.Couple{
		12345: {
			{SongID: songID, AnchorTimeMs: 1000},
			{SongID: songID, AnchorTimeMs: 2000},
		},
		67890: {
			{SongID: songID, AnchorTimeMs: 3000},
		},
	}

	err = client.StoreFingerprints(fingerprints)
	if err != nil {
		t.Fatalf("Failed to store fingerprints: %v", err)
	}

	// Verify fingerprints exist
	var fpCount int64
	client.DB.Model(&Fingerprint{}).Where("song_id = ?", songID).Count(&fpCount)
	if fpCount != 3 {
		t.Errorf("Expected 3 fingerprints, found %d", fpCount)
	}

	// Delete the song
	err = client.DeleteSongByID(songID)
	if err != nil {
		t.Fatalf("Failed to delete song: %v", err)
	}

	// Verify fingerprints were also deleted
	client.DB.Model(&Fingerprint{}).Where("song_id = ?", songID).Count(&fpCount)
	if fpCount != 0 {
		t.Errorf("Expected 0 fingerprints after song deletion, found %d", fpCount)
	}
}

// TestStoreFingerprints tests storing fingerprints
func TestStoreFingerprints(t *testing.T) {
	client, _ := setupTestDB(t)

	songID, _ := client.RegisterSong("Fingerprint Song", "FP Artist", "yt_fp1", 160000)

	fingerprints := map[uint32][]model.Couple{
		100: {
			{SongID: songID, AnchorTimeMs: 500},
		},
		200: {
			{SongID: songID, AnchorTimeMs: 1500},
			{SongID: songID, AnchorTimeMs: 2500},
		},
		300: {
			{SongID: songID, AnchorTimeMs: 3500},
		},
	}

	err := client.StoreFingerprints(fingerprints)
	if err != nil {
		t.Fatalf("Failed to store fingerprints: %v", err)
	}

	// Verify fingerprints were stored
	var count int64
	client.DB.Model(&Fingerprint{}).Where("song_id = ?", songID).Count(&count)
	if count != 4 {
		t.Errorf("Expected 4 fingerprints, found %d", count)
	}
}

// TestStoreFingerprintsLargeBatch tests batch insertion with large dataset
func TestStoreFingerprintsLargeBatch(t *testing.T) {
	client, _ := setupTestDB(t)

	songID, _ := client.RegisterSong("Large Batch", "Batch Artist", "yt_batch", 300000)

	// Create a large batch of fingerprints (> 1000 to test batching)
	fingerprints := make(map[uint32][]model.Couple)
	for i := uint32(1); i <= 1500; i++ {
		fingerprints[i] = []model.Couple{
			{SongID: songID, AnchorTimeMs: i * 100},
		}
	}

	err := client.StoreFingerprints(fingerprints)
	if err != nil {
		t.Fatalf("Failed to store large batch of fingerprints: %v", err)
	}

	// Verify all fingerprints were stored
	var count int64
	client.DB.Model(&Fingerprint{}).Where("song_id = ?", songID).Count(&count)
	if count != 1500 {
		t.Errorf("Expected 1500 fingerprints, found %d", count)
	}
}

// TestGetCouplesByHash tests retrieving couples by hash
func TestGetCouplesByHash(t *testing.T) {
	client, _ := setupTestDB(t)

	songID1, _ := client.RegisterSong("Song 1", "Artist 1", "yt1", 120000)
	songID2, _ := client.RegisterSong("Song 2", "Artist 2", "yt2", 130000)

	// Store fingerprints with the same hash for different songs
	const testHash uint32 = 99999
	fingerprints := map[uint32][]model.Couple{
		testHash: {
			{SongID: songID1, AnchorTimeMs: 1000},
			{SongID: songID1, AnchorTimeMs: 2000},
			{SongID: songID2, AnchorTimeMs: 1500},
		},
	}

	err := client.StoreFingerprints(fingerprints)
	if err != nil {
		t.Fatalf("Failed to store fingerprints: %v", err)
	}

	// Retrieve couples by hash
	couples, err := client.GetCouplesByHash(testHash)
	if err != nil {
		t.Fatalf("Failed to get couples by hash: %v", err)
	}

	if len(couples) != 3 {
		t.Errorf("Expected 3 couples, got %d", len(couples))
	}

	// Verify the couples contain correct data
	foundSong1Count := 0
	foundSong2Count := 0
	for _, couple := range couples {
		if couple.SongID == songID1 {
			foundSong1Count++
		} else if couple.SongID == songID2 {
			foundSong2Count++
		}
	}

	if foundSong1Count != 2 {
		t.Errorf("Expected 2 couples for song 1, found %d", foundSong1Count)
	}
	if foundSong2Count != 1 {
		t.Errorf("Expected 1 couple for song 2, found %d", foundSong2Count)
	}
}

// TestGetCouplesByHashNotFound tests retrieving non-existent hash
func TestGetCouplesByHashNotFound(t *testing.T) {
	client, _ := setupTestDB(t)

	couples, err := client.GetCouplesByHash(88888)
	if err != nil {
		t.Fatalf("Expected no error for non-existent hash, got: %v", err)
	}

	if len(couples) != 0 {
		t.Errorf("Expected 0 couples for non-existent hash, got %d", len(couples))
	}
}

// TestMultipleSongs tests working with multiple songs
func TestMultipleSongs(t *testing.T) {
	client, _ := setupTestDB(t)

	// Register multiple songs
	id1, _ := client.RegisterSong("Song A", "Artist A", "yta", 100000)
	id2, _ := client.RegisterSong("Song B", "Artist B", "ytb", 110000)
	id3, _ := client.RegisterSong("Song C", "Artist C", "ytc", 120000)

	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Error("Expected unique IDs for different songs")
	}

	// Verify all songs are in database
	var count int64
	client.DB.Model(&Song{}).Count(&count)
	if count != 3 {
		t.Errorf("Expected 3 songs in database, found %d", count)
	}
}

// TestClose tests closing the database connection
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "close_test.sqlite3")

	os.Setenv("ACOUSTIC_DB_PATH", dbPath)
	defer os.Unsetenv("ACOUSTIC_DB_PATH")

	client, err := NewDBClient()
	if err != nil {
		t.Fatalf("Failed to create DB client: %v", err)
	}

	// Close the connection
	err = client.Close()
	if err != nil {
		t.Errorf("Failed to close DB connection: %v", err)
	}

	// Closing again should be safe (nil check)
	err = client.Close()
	if err != nil {
		t.Errorf("Second close should not error: %v", err)
	}
}

// TestNilClientMethods tests that methods handle nil client gracefully
func TestNilClientMethods(t *testing.T) {
	var client *DBClient

	// Test RegisterSong
	_, err := client.RegisterSong("Test", "Test", "yt", 100000)
	if err == nil {
		t.Error("Expected error for nil client in RegisterSong")
	}

	// Test DeleteSongByID
	err = client.DeleteSongByID(1)
	if err == nil {
		t.Error("Expected error for nil client in DeleteSongByID")
	}

	// Test StoreFingerprints
	err = client.StoreFingerprints(nil)
	if err == nil {
		t.Error("Expected error for nil client in StoreFingerprints")
	}

	// Test GetCouplesByHash
	_, err = client.GetCouplesByHash(123)
	if err == nil {
		t.Error("Expected error for nil client in GetCouplesByHash")
	}

	// Test Close (should not panic)
	err = client.Close()
	if err != nil {
		t.Errorf("Close on nil client should return nil, got: %v", err)
	}
}

// TestEmptyFingerprints tests storing empty fingerprint map
func TestEmptyFingerprints(t *testing.T) {
	client, _ := setupTestDB(t)

	emptyMap := make(map[uint32][]model.Couple)
	err := client.StoreFingerprints(emptyMap)
	if err != nil {
		t.Errorf("Expected no error for empty fingerprint map, got: %v", err)
	}
}

// TestConcurrentOperations tests thread safety of DB operations
func TestConcurrentOperations(t *testing.T) {
	client, _ := setupTestDB(t)

	// Register multiple songs concurrently
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func(idx int) {
			_, err := client.RegisterSong(
				"Concurrent Song",
				"Concurrent Artist",
				"",
				100000+idx*1000,
			)
			if err != nil {
				t.Errorf("Failed to register song concurrently: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Since all songs have the same title+artist, we should have only 1 song
	var count int64
	client.DB.Model(&Song{}).Where("title = ? AND artist = ?", "Concurrent Song", "Concurrent Artist").Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 song after concurrent operations, found %d", count)
	}
}
