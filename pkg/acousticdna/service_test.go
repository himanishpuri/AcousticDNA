//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

import (
	"math"
	"testing"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

// TestCalculateConfidence locks the confidence model. Inputs are
// (bestCount, secondBestOffsetCount, competingSongScore); expected values are
// pre-computed against the exact literals (steepness 9, midpoint 0.35,
// minReliable 8 with a quadratic floor). Changing any constant moves these.
func TestCalculateConfidence(t *testing.T) {
	const eps = 1e-2 // confidence is a display %, 2 dp is enough

	s := &acousticService{}

	cases := []struct {
		name                        string
		best, secondBest, competing int
		want                        float64
	}{
		// Zero guard.
		{"zero best count", 0, 0, 0, 0.0},

		// Real match: sharp peak (2 vs 20), clear margin over runner-up (8).
		// This is the Benson Boone case that used to read ~5%.
		{"true match peaked+separated", 20, 2, 8, 97.3403},

		// Noise: flat distribution (second bin 5 ~ best 6) and tied with the
		// competing song (6). The moosy_test.wav case — must stay low.
		{"noise flat+tied", 6, 5, 6, 4.6763},

		// Weak vote counts are damped by the quadratic floor even when peaked
		// with no competitor.
		{"weak 3 votes floored", 3, 0, 0, 14.0242},
		{"weak 2 votes floored", 2, 0, 0, 6.2330},

		// Solid, mid-range match.
		{"mid match", 10, 4, 6, 79.4130},

		// Peaked with no competing song (single candidate) reads high.
		{"single strong candidate", 20, 2, 0, 99.5504},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.calculateConfidence(tc.best, tc.secondBest, tc.competing)
			if math.Abs(got-tc.want) > eps {
				t.Fatalf("calculateConfidence(%d, %d, %d) = %v, want %v",
					tc.best, tc.secondBest, tc.competing, got, tc.want)
			}
		})
	}
}

// TestCalculateConfidenceThresholds is the plan's acceptance check: a genuine
// match reads high, a noise match reads low.
func TestCalculateConfidenceThresholds(t *testing.T) {
	s := &acousticService{}

	if got := s.calculateConfidence(20, 2, 8); got <= 60 {
		t.Fatalf("true match should be >60%%, got %v", got)
	}
	if got := s.calculateConfidence(6, 5, 6); got >= 25 {
		t.Fatalf("noise match should be <25%%, got %v", got)
	}
}

// TestCalculateConfidenceMonotonic: sharper peak (lower second-best) never
// lowers confidence, and larger margin (lower competing) never lowers it.
func TestCalculateConfidenceMonotonic(t *testing.T) {
	s := &acousticService{}

	// Increasing peak sharpness (secondBest falling) is non-decreasing.
	prev := -1.0
	for sb := 20; sb >= 0; sb-- {
		got := s.calculateConfidence(20, sb, 4)
		if got < prev {
			t.Fatalf("confidence decreased as peak sharpened (secondBest=%d): %v < %v", sb, got, prev)
		}
		prev = got
	}

	// Increasing margin (competing falling) is non-decreasing.
	prev = -1.0
	for c := 20; c >= 0; c-- {
		got := s.calculateConfidence(20, 2, c)
		if got < prev {
			t.Fatalf("confidence decreased as margin grew (competing=%d): %v < %v", c, got, prev)
		}
		prev = got
	}
}

// TestCompetingScore locks the runner-up selection: top match sees the
// second-place score; lower matches see the leader's score.
func TestCompetingScore(t *testing.T) {
	matches := []models.Match{{Count: 20}, {Count: 8}, {Count: 5}}

	if got := competingScore(matches, 0); got != 8 {
		t.Fatalf("top match competing = %d, want 8 (runner-up)", got)
	}
	if got := competingScore(matches, 1); got != 20 {
		t.Fatalf("second match competing = %d, want 20 (leader)", got)
	}
	if got := competingScore([]models.Match{{Count: 20}}, 0); got != 0 {
		t.Fatalf("sole match competing = %d, want 0", got)
	}
}
