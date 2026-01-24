package fingerprint

import (
	"math"
	"sort"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

// ------------------------ Fingerprinting (build DB entries) ------------------------

// Fingerprint produces a map from hash -> []Couple for the provided peaks and songID.
// It uses a time-windowed fan-out: for each anchor, pair with up to FanOut subsequent
// peaks that are within MaxDeltaMs.
func Fingerprint(peaks []Peak, songID string) map[uint32][]models.Couple {
	// Ensure time-sorted order
	sort.Slice(peaks, func(i, j int) bool { return peaks[i].Time < peaks[j].Time })

	fp := make(map[uint32][]models.Couple)
	for i := 0; i < len(peaks); i++ {
		anchor := peaks[i]
		paired := 0
		for j := i + 1; j < len(peaks) && paired < FanOut; j++ {
			target := peaks[j]
			// createAddress will enforce Min/Max delta and bit fit
			addr, ok := createAddress(anchor, target)
			if !ok {
				// skip if not representable or delta out of range
				continue
			}
			cou := models.Couple{SongID: songID, AnchorTimeMs: uint32(math.Round(anchor.Time * 1000.0))}
			fp[addr] = append(fp[addr], cou)
			paired++
		}
	}
	return fp
}

// MergeFingerprints merges src into dst (appends couples for same hash keys).
func MergeFingerprints(dst map[uint32][]models.Couple, src map[uint32][]models.Couple) {
	for k, v := range src {
		dst[k] = append(dst[k], v...)
	}
}

// ------------------------ Querying / Matching ------------------------

// QueryFingerprints takes query peaks and a fingerprint database (map[hash][]Couple)
// and returns ranked matches (by vote count). The top results are returned in descending
// order of vote count. The function uses the same pairing/fanout rules for the query.
func QueryFingerprints(queryPeaks []Peak, db map[uint32][]models.Couple) []models.Match {
	// Build query hashes (pairs) using same policy as Fingerprint
	sort.Slice(queryPeaks, func(i, j int) bool { return queryPeaks[i].Time < queryPeaks[j].Time })

	// votes[songID][offsetMs] = count
	votes := make(map[string]map[int32]int)

	for i := 0; i < len(queryPeaks); i++ {
		anchor := queryPeaks[i]
		paired := 0
		for j := i + 1; j < len(queryPeaks) && paired < FanOut; j++ {
			target := queryPeaks[j]
			addr, ok := createAddress(anchor, target)
			if !ok {
				continue
			}
			paired++
			// For every db occurrence of this hash, increment vote for (songID, offset)
			if bucket, ok := db[addr]; ok {
				for _, cou := range bucket {
					// offset = dbAnchorTimeMs - queryAnchorTimeMs
					offset := int32(cou.AnchorTimeMs) - int32(math.Round(anchor.Time*1000.0))
					m, ok := votes[cou.SongID]
					if !ok {
						m = make(map[int32]int)
						votes[cou.SongID] = m
					}
					m[offset]++
				}
			}
		}
	}

	// Flatten votes into []Match and find top results
	matches := make([]models.Match, 0)
	for songID, offsets := range votes {
		// find best offset for this song
		bestOffset := int32(0)
		bestCount := 0
		for off, cnt := range offsets {
			if cnt > bestCount {
				bestCount = cnt
				bestOffset = off
			}
		}
		if bestCount > 0 {
			matches = append(matches, models.Match{SongID: songID, OffsetMs: bestOffset, Count: bestCount})
		}
	}

	// sort descending by Count
	sort.Slice(matches, func(i, j int) bool { return matches[i].Count > matches[j].Count })
	return matches
}
