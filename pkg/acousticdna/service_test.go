//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

import (
	"math"
	"testing"
)

// TestCalculateConfidence locks the confidence math. The expected values are
// pre-computed against the exact literals in calculateConfidence; changing
// steepness, midpoint, the boost threshold/gain, or the min-reliable count
// will move these numbers and fail the test.
func TestCalculateConfidence(t *testing.T) {
	const eps = 1e-9

	s := &acousticService{}

	cases := []struct {
		name                      string
		matchCount, queryFP, dbFP int
		want                      float64
	}{
		// Zero guards: any zero input yields 0 confidence.
		{"zero match count", 0, 100, 100, 0.0},
		{"zero query size", 15, 0, 100, 0.0},
		{"zero db size", 15, 100, 0, 0.0},

		// Sigmoid midpoint: ratio == midpoint (0.15) => exactly 50%.
		{"midpoint ratio", 15, 100, 100, 50.0},

		// Min-count selection: min(1000,100)=100, so ratio is identical to the
		// midpoint case above -> proves the smaller of query/db is used.
		{"min-count selection uses smaller", 15, 1000, 100, 50.0},

		// Statistical-significance penalty for matchCount < 5.
		{"penalty at 3", 3, 100, 100, 4.990361789635342},
		{"penalty at boundary 4", 4, 100, 100, 7.9800391295748145},
		// matchCount == 5 is the first count with NO penalty (locks < vs <=).
		{"no penalty at boundary 5", 5, 100, 100, 11.920292202211758},

		// Boost for ratio > 0.30 (32/100 = 0.32).
		{"boost above 0.30", 32, 100, 100, 97.77045353015495},
		// Boost cannot exceed 100 (40/100 = 0.40 would overshoot).
		{"boost capped at 100", 40, 100, 100, 100.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.calculateConfidence(tc.matchCount, tc.queryFP, tc.dbFP)
			if math.Abs(got-tc.want) > eps {
				t.Fatalf("calculateConfidence(%d, %d, %d) = %v, want %v",
					tc.matchCount, tc.queryFP, tc.dbFP, got, tc.want)
			}
		})
	}
}

// TestCalculateConfidenceMonotonic is a light property check: within the
// no-penalty, pre-boost region, more matches never lowers confidence.
func TestCalculateConfidenceMonotonic(t *testing.T) {
	s := &acousticService{}
	prev := -1.0
	for m := 5; m <= 25; m++ {
		got := s.calculateConfidence(m, 100, 100)
		if got < prev {
			t.Fatalf("confidence decreased at matchCount=%d: %v < %v", m, got, prev)
		}
		prev = got
	}
}
