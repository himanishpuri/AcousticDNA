package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// WavFormat holds the format information from the fmt chunk
type WavFormat struct {
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	BitsPerSample uint16
}

// WavData holds the complete WAV file data
type WavData struct {
	Format WavFormat
	Data   []byte
}

// readRIFFHeader reads and validates the RIFF/WAVE header (12 bytes)
func readRIFFHeader(f *os.File) error {
	var riff [4]byte
	var fileSize uint32
	var wave [4]byte

	if err := binary.Read(f, binary.LittleEndian, &riff); err != nil {
		return fmt.Errorf("reading RIFF header: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &fileSize); err != nil {
		return fmt.Errorf("reading RIFF size: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &wave); err != nil {
		return fmt.Errorf("reading WAVE id: %w", err)
	}

	if string(riff[:]) != "RIFF" || string(wave[:]) != "WAVE" {
		return errors.New("not a WAV/RIFF file")
	}

	return nil
}

// readFmtChunk reads the fmt chunk and returns format information
func readFmtChunk(f *os.File, chunkSize uint32) (*WavFormat, error) {
	var audioFormat uint16
	var numChannels uint16
	var sampleRate uint32
	var byteRate uint32
	var blockAlign uint16
	var bitsPerSample uint16

	if err := binary.Read(f, binary.LittleEndian, &audioFormat); err != nil {
		return nil, fmt.Errorf("reading fmt audioFormat: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &numChannels); err != nil {
		return nil, fmt.Errorf("reading fmt numChannels: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &sampleRate); err != nil {
		return nil, fmt.Errorf("reading fmt sampleRate: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &byteRate); err != nil {
		return nil, fmt.Errorf("reading fmt byteRate: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &blockAlign); err != nil {
		return nil, fmt.Errorf("reading fmt blockAlign: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &bitsPerSample); err != nil {
		return nil, fmt.Errorf("reading fmt bitsPerSample: %w", err)
	}

	// If there are extra bytes in fmt chunk, skip them
	remaining := int(chunkSize) - 16
	if remaining > 0 {
		if _, err := f.Seek(int64(remaining), io.SeekCurrent); err != nil {
			return nil, fmt.Errorf("seeking past fmt extras: %w", err)
		}
	}

	return &WavFormat{
		AudioFormat:   audioFormat,
		NumChannels:   numChannels,
		SampleRate:    sampleRate,
		BitsPerSample: bitsPerSample,
	}, nil
}

// readDataChunk reads the data chunk and returns raw PCM data
func readDataChunk(f *os.File, chunkSize uint32) ([]byte, error) {
	dataChunk := make([]byte, chunkSize)
	if _, err := io.ReadFull(f, dataChunk); err != nil {
		return nil, fmt.Errorf("reading data chunk: %w", err)
	}
	return dataChunk, nil
}

// skipChunk skips an unknown chunk
func skipChunk(f *os.File, chunkSize uint32) error {
	skip := int64(chunkSize)
	if _, err := f.Seek(skip, io.SeekCurrent); err != nil {
		return err
	}
	return nil
}

// scanWavChunks scans through WAV chunks to find fmt and data chunks
func scanWavChunks(f *os.File) (*WavData, error) {
	var format WavFormat
	var dataChunk []byte
	fmtFound := false
	dataFound := false

	for {
		// Read next chunk header: ID (4) + Size (4)
		var chunkID [4]byte
		var chunkSize uint32

		if err := binary.Read(f, binary.LittleEndian, &chunkID); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("reading chunk header: %w", err)
		}
		if err := binary.Read(f, binary.LittleEndian, &chunkSize); err != nil {
			return nil, fmt.Errorf("reading chunk size: %w", err)
		}

		id := string(chunkID[:])

		switch id {
		case "fmt ":
			fmt, err := readFmtChunk(f, chunkSize)
			if err != nil {
				return nil, err
			}
			format = *fmt
			fmtFound = true

		case "data":
			data, err := readDataChunk(f, chunkSize)
			if err != nil {
				return nil, err
			}
			dataChunk = data
			dataFound = true

		default:
			// Unknown chunk (e.g., LIST, INFO, junk). Skip it.
			if err := skipChunk(f, chunkSize); err != nil {
				return nil, fmt.Errorf("skipping chunk %s: %w", id, err)
			}
		}

		// If chunk size is odd, skip pad byte
		if chunkSize%2 == 1 {
			if _, err := f.Seek(1, io.SeekCurrent); err != nil {
				return nil, fmt.Errorf("seeking pad byte: %w", err)
			}
		}

		// We can stop early if we have both fmt and data
		if fmtFound && dataFound {
			break
		}
	}

	if !fmtFound {
		return nil, errors.New("fmt chunk not found")
	}
	if !dataFound {
		return nil, errors.New("data chunk not found")
	}

	return &WavData{
		Format: format,
		Data:   dataChunk,
	}, nil
}

// convertToInt16Samples converts byte data to int16 samples
func convertToInt16Samples(data []byte) ([]int16, error) {
	sampleCount := len(data) / 2
	int16Buf := make([]int16, sampleCount)
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, int16Buf); err != nil {
		return nil, fmt.Errorf("decoding PCM samples: %w", err)
	}
	return int16Buf, nil
}

// convertMonoToFloat64 converts mono int16 samples to float64
func convertMonoToFloat64(samples []int16, scale float64) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = float64(s) * scale
	}
	return out
}

// convertStereoToMono converts stereo int16 samples to mono float64 by averaging channels
func convertStereoToMono(samples []int16, scale float64) []float64 {
	frames := len(samples) / 2
	out := make([]float64, frames)
	for i := 0; i < frames; i++ {
		l := float64(samples[2*i]) * scale
		r := float64(samples[2*i+1]) * scale
		out[i] = (l + r) * 0.5
	}
	return out
}

// convertToMonoFloat64 converts int16 samples to mono float64 samples normalized to [-1, 1]
func convertToMonoFloat64(samples []int16, numChannels uint16) ([]float64, error) {
	const scale = 1.0 / 32768.0 // to ensure 16 bit PCM

	switch numChannels {
	case 1:
		return convertMonoToFloat64(samples, scale), nil
	case 2:
		return convertStereoToMono(samples, scale), nil
	default:
		return nil, errors.New("unsupported channel count: only mono/stereo supported")
	}
}

// ReadWavAsFloat64 reads a 16-bit PCM WAV file and returns mono, normalized
// samples in the range [-1,1] and the sample rate.
// Does not assume a canonical 44-byte PCM WAV header.
func ReadWavAsFloat64(path string) ([]float64, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	// Read and validate RIFF header
	if err := readRIFFHeader(f); err != nil {
		return nil, 0, err
	}

	// Scan chunks to find fmt and data
	wavData, err := scanWavChunks(f)
	if err != nil {
		return nil, 0, err
	}

	// Validate format
	if wavData.Format.AudioFormat != 1 {
		return nil, 0, errors.New("unsupported WAV audio format: only PCM (1) supported")
	}
	if wavData.Format.BitsPerSample != 16 {
		return nil, 0, errors.New("unsupported bits per sample: only 16-bit supported")
	}

	// Convert data chunk to int16 samples
	int16Samples, err := convertToInt16Samples(wavData.Data)
	if err != nil {
		return nil, 0, err
	}

	// Convert to mono float64
	monoSamples, err := convertToMonoFloat64(int16Samples, wavData.Format.NumChannels)
	if err != nil {
		return nil, 0, err
	}

	return monoSamples, int(wavData.Format.SampleRate), nil
}
