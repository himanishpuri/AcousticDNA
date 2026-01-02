package main

import (
	"fmt"
	"image"
	"image/draw"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/eligwz/spectrogram"
	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
)

func main() {
	inputDir := "test/convertedtestdata"
	outputDir := "test/spectrograms"

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatal(err)
	}

	// Process all WAV files in the input directory
	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || filepath.Ext(path) != ".wav" {
			return nil
		}

		fmt.Printf("Processing %s...\n", path)

		// Read WAV file
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Error opening %s: %v", path, err)
			return nil
		}
		defer file.Close()

		decoder := wav.NewDecoder(file)
		if !decoder.IsValidFile() {
			log.Printf("Invalid WAV file: %s", path)
			return nil
		}

		// Get duration to allocate buffer
		duration, err := decoder.Duration()
		if err != nil {
			log.Printf("Error getting duration from %s: %v", path, err)
			return nil
		}

		totalSamples := int(duration.Seconds() * float64(decoder.SampleRate))
		if totalSamples == 0 {
			log.Printf("No samples in %s", path)
			return nil
		}

		// Read all samples
		buf := &audio.IntBuffer{
			Format: &audio.Format{
				NumChannels: int(decoder.NumChans),
				SampleRate:  int(decoder.SampleRate),
			},
			Data:           make([]int, totalSamples*int(decoder.NumChans)),
			SourceBitDepth: int(decoder.BitDepth),
		}

		_, err = decoder.PCMBuffer(buf)
		if err != nil {
			log.Printf("Error reading samples from %s: %v", path, err)
			return nil
		}

		// Convert to float64 and normalize to [-1.0, 1.0]
		samples := make([]float64, len(buf.Data))
		maxVal := float64(int(1) << (uint(decoder.BitDepth) - 1))
		for i, v := range buf.Data {
			samples[i] = float64(v) / maxVal
		}

		fmt.Printf("Read %d samples at %d Hz\n", len(samples), decoder.SampleRate)

		// Create image for spectrogram
		width := 2048
		height := 512
		img := spectrogram.NewImage128(image.Rect(0, 0, width, height))

		// Fill with black background first
		black := spectrogram.ParseColor("000000")
		draw.Draw(img, img.Bounds(), image.NewUniform(black), image.Point{}, draw.Src)

		// Generate spectrogram using FFT (not DFT, as it's much faster)
		// RECTANGLE: false = use Hamming window
		// DFT: false = use FFT (faster)
		// MAG: true = magnitude
		// LOG10: false = linear scale (LOG10 causes the issue)
		spectrogram.Drawfft(
			img,
			samples,
			uint32(decoder.SampleRate),
			uint32(height), // bins
			false,          // RECTANGLE (use Hamming window)
			false,          // DFT (use FFT instead)
			true,           // MAG (magnitude)
			false,          // LOG10 (linear scale - LOG10 causes issues)
		)

		// Create output file path
		baseName := filepath.Base(path)
		outputPath := filepath.Join(outputDir, baseName+".png")

		// Save spectrogram as PNG
		if err := spectrogram.SavePng(img, outputPath); err != nil {
			log.Printf("Error saving PNG for %s: %v", outputPath, err)
			return nil
		}

		fmt.Printf("Saved spectrogram to %s\n", outputPath)
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Done!")
}
