package fingerprint

import (
	"path/filepath"
	"testing"
)

func TestExtractPeaks(t *testing.T) {
	testFile := filepath.Join("..", "..", "test", "convertedtestdata", "Sandstorm-Darude.wav")

	spec, sr, err := ComputeSpectrogram(testFile, 0, 0)
	if err != nil {
		t.Fatalf("Failed to compute spectrogram: %v", err)
	}

	numFrames := len(spec)
	frameTime := float64(HopSize) / float64(sr)
	duration := float64(numFrames) * frameTime

	peaks := ExtractPeaks(spec, duration, sr)

	if len(peaks) == 0 {
		t.Error("No peaks extracted")
	}

	// Check peaks are sorted by time
	for i := 1; i < len(peaks); i++ {
		if peaks[i].TimeIdx < peaks[i-1].TimeIdx {
			t.Error("Peaks not sorted by time index")
			break
		}
		if peaks[i].TimeIdx == peaks[i-1].TimeIdx {
			if peaks[i].FreqIdx < peaks[i-1].FreqIdx {
				t.Error("Peaks not sorted by frequency within same time")
				break
			}
		}
	}

	// Check peak values are reasonable
	for i, p := range peaks {
		if p.TimeIdx < 0 || p.TimeIdx >= numFrames {
			t.Errorf("Peak %d has invalid time index: %d", i, p.TimeIdx)
		}
		if p.FreqIdx < 0 || p.FreqIdx >= len(spec[0]) {
			t.Errorf("Peak %d has invalid freq index: %d", i, p.FreqIdx)
		}
		if p.Time < 0 || p.Time > duration {
			t.Errorf("Peak %d has invalid time: %f", i, p.Time)
		}
		if p.Freq < 0 {
			t.Errorf("Peak %d has negative frequency: %f", i, p.Freq)
		}
	}

	// Check peak density is reasonable (adjust based on your audio)
	peakDensity := float64(len(peaks)) / duration
	if peakDensity < 1.0 || peakDensity > 1000.0 {
		t.Logf("Warning: Peak density seems unusual: %.2f peaks/second", peakDensity)
	}

	t.Logf("Extracted %d peaks (%.2f peaks/sec)", len(peaks), peakDensity)
}

func TestExtractPeaksEmptySpectrogram(t *testing.T) {
	var emptySpec [][]float64

	peaks := ExtractPeaks(emptySpec, 1.0, 11025)

	if peaks != nil && len(peaks) > 0 {
		t.Error("Expected no peaks from empty spectrogram")
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{5, 10, 5},
		{10, 5, 5},
		{7, 7, 7},
		{-5, 3, -5},
	}

	for _, tt := range tests {
		result := minInt(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("minInt(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
