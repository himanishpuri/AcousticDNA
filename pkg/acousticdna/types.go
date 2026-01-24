package acousticdna

// MatchResult represents a song match result with metadata and scoring.
type MatchResult struct {
	SongID     uint32  // Database ID of the matched song
	Title      string  // Song title
	Artist     string  // Artist name
	YouTubeID  string  // YouTube video ID (if available)
	Score      int     // Number of matching fingerprint hashes
	OffsetMs   int32   // Time offset in milliseconds
	Confidence float64 // Match confidence as a percentage (0-100)
}

// Song represents a song entry in the database.
type Song struct {
	ID         uint32 // Database ID
	Title      string // Song title
	Artist     string // Artist name
	YouTubeID  string // YouTube video ID (if available)
	DurationMs int    // Duration in milliseconds
}
