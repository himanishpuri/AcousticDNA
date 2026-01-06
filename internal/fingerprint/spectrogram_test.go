package fingerprint

import (
	"path/filepath"
	"testing"
)

func getTestFile(t *testing.T) string {
	return filepath.Join("..", "..", "test", "convertedtestdata", "Sandstorm-Darude.wav")
}

func TestHamming(t *testing.T) {
	sizes := []int{128, 256, 512, 1024}

	for _, size := range sizes {
		window := Hamming(size)

		if len(window) != size {
			t.Errorf("Expected window size %d, got %d", size, len(window))
		}

		// Check window values are in reasonable range
		for i, val := range window {
			if val < 0 || val > 1 {
				t.Errorf("Window value %d out of range [0,1]: %f", i, val)
			}
		}

		// Hamming window should have lower values at edges
		if window[0] >= window[size/2] {
			t.Error("Hamming window should be lower at edges")
		}
	}
}

func TestFFTReal(t *testing.T) {
	// Simple test signal
	signal := make([]float64, 128)
	for i := range signal {
		signal[i] = 1.0 // DC signal
	}

	spectrum := FFTReal(signal)

	if len(spectrum) != len(signal) {
		t.Errorf("Expected spectrum length %d, got %d", len(signal), len(spectrum))
	}
}

func TestMagnitudeSpectrum(t *testing.T) {
	spectrum := []complex128{
		complex(1.0, 0.0),
		complex(0.0, 1.0),
		complex(3.0, 4.0),
		complex(0.0, 0.0),
	}

	mag := MagnitudeSpectrum(spectrum)

	expectedLen := len(spectrum) / 2
	if len(mag) != expectedLen {
		t.Errorf("Expected magnitude length %d, got %d", expectedLen, len(mag))
	}

	// |1+0i| = 1
	if mag[0] != 1.0 {
		t.Errorf("Expected magnitude 1.0, got %f", mag[0])
	}

	// |0+1i| = 1
	if mag[1] != 1.0 {
		t.Errorf("Expected magnitude 1.0, got %f", mag[1])
	}
}

func TestSTFT(t *testing.T) {
	windowSize := 128
	hopSize := 64
	sampleRate := 11025

	// Create test signal (1 second of silence)
	samples := make([]float64, sampleRate)
	window := Hamming(windowSize)

	spec, err := STFT(samples, sampleRate, windowSize, hopSize, window)
	if err != nil {
		t.Fatalf("STFT failed: %v", err)
	}

	if len(spec) == 0 {
		t.Error("Empty spectrogram")
	}

	expectedFrames := (len(samples)-windowSize)/hopSize + 1
	if len(spec) < expectedFrames-1 || len(spec) > expectedFrames+1 {
		t.Logf("Expected ~%d frames, got %d", expectedFrames, len(spec))
	}

	// Each frame should have windowSize/2 bins
	expectedBins := windowSize / 2
	if len(spec[0]) != expectedBins {
		t.Errorf("Expected %d frequency bins, got %d", expectedBins, len(spec[0]))
	}
}

func TestSTFTInvalidInput(t *testing.T) {
	windowSize := 128
	hopSize := 64
	sampleRate := 11025

	// Test with too short samples
	samples := make([]float64, 50)
	window := Hamming(windowSize)

	_, err := STFT(samples, sampleRate, windowSize, hopSize, window)
	if err == nil {
		t.Error("Expected error with samples shorter than window")
	}

	// Test with wrong window size
	samples = make([]float64, 1000)
	wrongWindow := Hamming(64)

	_, err = STFT(samples, sampleRate, windowSize, hopSize, wrongWindow)
	if err == nil {
		t.Error("Expected error with mismatched window size")
	}
}

func TestComputeSpectrogram(t *testing.T) {
	testFile := getTestFile(t)

	spec, sampleRate, err := ComputeSpectrogram(testFile, 0, 0)
	if err != nil {
		t.Fatalf("ComputeSpectrogram failed: %v", err)
	}

	if len(spec) == 0 {
		t.Error("Empty spectrogram")
	}

	if sampleRate != 11025 {
		t.Logf("Expected sample rate 11025, got %d", sampleRate)
	}

	// Check dimensions
	numFrames := len(spec)
	numBins := len(spec[0])

	expectedBins := WindowSize / 2
	if numBins != expectedBins {
		t.Errorf("Expected %d bins, got %d", expectedBins, numBins)
	}

	t.Logf("Spectrogram: %d frames × %d bins", numFrames, numBins)
}

func TestComputeSpectrogramCustomParams(t *testing.T) {
	testFile := getTestFile(t)

	customWindow := 512
	customHop := 128

	spec, _, err := ComputeSpectrogram(testFile, customWindow, customHop)
	if err != nil {
		t.Fatalf("ComputeSpectrogram failed: %v", err)
	}

	expectedBins := customWindow / 2
	if len(spec[0]) != expectedBins {
		t.Errorf("Expected %d bins with custom window, got %d", expectedBins, len(spec[0]))
	}
}
