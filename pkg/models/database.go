package models

// Couple is the stored value for a hash bucket entry.
// AnchorTimeMs is the time (in ms) of the anchor peak in the source audio.
type Couple struct {
	SongID       string // UUID of the song
	AnchorTimeMs uint32
}

// Match represents a candidate match returned by the query matcher.
type Match struct {
	SongID   string // UUID of the song
	OffsetMs int32  // dbAnchorTimeMs - queryAnchorTimeMs
	Count    int
}
