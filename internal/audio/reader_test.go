package audio

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper to get test file path
func getTestFile(t *testing.T) string {
	testFile := filepath.Join("..", "..", "test", "convertedtestdata", "Sandstorm-Darude.wav")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skipf("Test file not found: %s. Run conversion first.", testFile)
	}
	return testFile
}

func TestReadRIFFHeader(t *testing.T) {
	testFile := getTestFile(t)
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer f.Close()

	err = readRIFFHeader(f)
	if err != nil {
		t.Errorf("readRIFFHeader failed: %v", err)
	}
}

func TestReadRIFFHeaderInvalidFile(t *testing.T) {
	// Create a temporary invalid file
	tmpFile, err := os.CreateTemp("", "invalid-*.wav")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Write invalid data
	tmpFile.Write([]byte("INVALID HEADER DATA"))
	tmpFile.Seek(0, 0)

	err = readRIFFHeader(tmpFile)
	if err == nil {
		t.Error("readRIFFHeader should fail on invalid file")
	}
}

func TestScanWavChunks(t *testing.T) {
	testFile := getTestFile(t)
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer f.Close()

	// Skip RIFF header first
	if err := readRIFFHeader(f); err != nil {
		t.Fatalf("Failed to read RIFF header: %v", err)
	}

	wavData, err := scanWavChunks(f)
	if err != nil {
		t.Fatalf("scanWavChunks failed: %v", err)
	}

	if wavData == nil {
		t.Fatal("wavData is nil")
	}

	// Validate format
	if wavData.Format.AudioFormat != 1 {
		t.Errorf("Expected PCM format (1), got %d", wavData.Format.AudioFormat)
	}
	if wavData.Format.SampleRate == 0 {
		t.Error("Sample rate is 0")
	}
	if wavData.Format.NumChannels == 0 {
		t.Error("Number of channels is 0")
	}
	if len(wavData.Data) == 0 {
		t.Error("No data in WAV file")
	}

	t.Logf("Format: %d-bit, %d channels, %d Hz",
		wavData.Format.BitsPerSample,
		wavData.Format.NumChannels,
		wavData.Format.SampleRate)
}

func TestConvertToInt16Samples(t *testing.T) {
	// Create test data (4 bytes = 2 int16 samples)
	testData := []byte{0x00, 0x01, 0xFF, 0x7F} // Little-endian int16: 256, 32767

	samples, err := convertToInt16Samples(testData)
	if err != nil {
		t.Fatalf("convertToInt16Samples failed: %v", err)
	}

	if len(samples) != 2 {
		t.Errorf("Expected 2 samples, got %d", len(samples))
	}

	if samples[0] != 256 {
		t.Errorf("Expected first sample to be 256, got %d", samples[0])
	}
	if samples[1] != 32767 {
		t.Errorf("Expected second sample to be 32767, got %d", samples[1])
	}
}

func TestConvertMonoToFloat64(t *testing.T) {
	samples := []int16{0, 16384, -16384, 32767, -32768}
	scale := 1.0 / 32768.0

	result := convertMonoToFloat64(samples, scale)

	if len(result) != len(samples) {
		t.Errorf("Expected %d samples, got %d", len(samples), len(result))
	}

	// Check normalization
	if result[0] != 0.0 {
		t.Errorf("Expected 0.0 for zero sample, got %f", result[0])
	}

	// Check that values are in [-1, 1] range
	for i, val := range result {
		if val < -1.0 || val > 1.0 {
			t.Errorf("Sample %d out of range [-1, 1]: %f", i, val)
		}
	}
}

func TestConvertStereoToMono(t *testing.T) {
	// Stereo samples: [L, R, L, R]
	samples := []int16{16384, 16384, -16384, -16384}
	scale := 1.0 / 32768.0

	result := convertStereoToMono(samples, scale)

	expectedFrames := len(samples) / 2
	if len(result) != expectedFrames {
		t.Errorf("Expected %d frames, got %d", expectedFrames, len(result))
	}

	// Both channels same, so average should be same
	expected0 := float64(16384) * scale
	if result[0] != expected0 {
		t.Errorf("Expected %f for first frame, got %f", expected0, result[0])
	}
}

func TestConvertToMonoFloat64(t *testing.T) {
	tests := []struct {
		name        string
		samples     []int16
		numChannels uint16
		expectError bool
	}{
		{
			name:        "Mono conversion",
			samples:     []int16{0, 16384, -16384},
			numChannels: 1,
			expectError: false,
		},
		{
			name:        "Stereo conversion",
			samples:     []int16{0, 0, 16384, 16384},
			numChannels: 2,
			expectError: false,
		},
		{
			name:        "Unsupported channels",
			samples:     []int16{0, 0, 0, 0},
			numChannels: 4,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertToMonoFloat64(tt.samples, tt.numChannels)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(result) == 0 {
				t.Error("Result is empty")
			}

			// Check normalization
			for i, val := range result {
				if val < -1.0 || val > 1.0 {
					t.Errorf("Sample %d out of range [-1, 1]: %f", i, val)
				}
			}
		})
	}
}

func TestReadWavAsFloat64(t *testing.T) {
	testFile := getTestFile(t)

	samples, sampleRate, err := ReadWavAsFloat64(testFile)
	if err != nil {
		t.Fatalf("ReadWavAsFloat64 failed: %v", err)
	}

	if len(samples) == 0 {
		t.Error("No samples returned")
	}

	if sampleRate == 0 {
		t.Error("Sample rate is 0")
	}

	// For converted test file, should be 11025 Hz
	if sampleRate != 11025 {
		t.Logf("Warning: Expected sample rate 11025, got %d", sampleRate)
	}

	// Check all samples are normalized
	outOfRange := 0
	for _, sample := range samples {
		if sample < -1.0 || sample > 1.0 {
			outOfRange++
			if outOfRange == 1 {
				t.Errorf("Sample out of range [-1, 1]: %f", sample)
			}
		}
	}

	if outOfRange > 0 {
		t.Errorf("Total samples out of range: %d / %d", outOfRange, len(samples))
	}

	t.Logf("Successfully read %d samples at %d Hz", len(samples), sampleRate)
}

func TestReadWavAsFloat64NonExistent(t *testing.T) {
	_, _, err := ReadWavAsFloat64("nonexistent-file.wav")
	if err == nil {
		t.Error("Expected error when reading non-existent file")
	}
}
