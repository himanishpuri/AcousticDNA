package fingerprint

import (
	"math"
	"sort"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

func Fingerprint(peaks []Peak, songID string) map[uint32][]models.Couple {
	sort.Slice(peaks, func(i, j int) bool { return peaks[i].Time < peaks[j].Time })

	fp := make(map[uint32][]models.Couple)
	for i := 0; i < len(peaks); i++ {
		anchor := peaks[i]
		paired := 0
		for j := i + 1; j < len(peaks) && paired < FanOut; j++ {
			target := peaks[j]
			addr, ok := createAddress(anchor, target)
			if !ok {
				continue
			}
			cou := models.Couple{SongID: songID, AnchorTimeMs: uint32(math.Round(anchor.Time * 1000.0))}
			fp[addr] = append(fp[addr], cou)
			paired++
		}
	}
	return fp
}

func MergeFingerprints(dst map[uint32][]models.Couple, src map[uint32][]models.Couple) {
	for k, v := range src {
		dst[k] = append(dst[k], v...)
	}
}

func QueryFingerprints(queryPeaks []Peak, db map[uint32][]models.Couple) []models.Match {
	sort.Slice(queryPeaks, func(i, j int) bool { return queryPeaks[i].Time < queryPeaks[j].Time })

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
			if bucket, ok := db[addr]; ok {
				for _, cou := range bucket {
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

	matches := make([]models.Match, 0)
	for songID, offsets := range votes {
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

	sort.Slice(matches, func(i, j int) bool { return matches[i].Count > matches[j].Count })
	return matches
}
