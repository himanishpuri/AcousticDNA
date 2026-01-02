package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/utils"
)

type ConvertWAVConfig struct {
	SampleRate int // e.g. 11025, 22050, 44100
}

// ConvertToMonoWAV converts an audio file to mono PCM WAV
// and saves it to outputDir, preserving the filename.
func ConvertToMonoWAV(
	ctx context.Context,
	inputPath string,
	outputDir string,
	cfg ConvertWAVConfig,
) (string, error) {

	if cfg.SampleRate == 0 {
		cfg.SampleRate = 11025
	}

	// Defensive timeout
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	baseName := filepath.Base(inputPath)
	outputPath := filepath.Join(outputDir, baseName)

	tmpPath := outputPath + ".tmp.wav"
	defer os.Remove(tmpPath)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-v", "quiet",
		"-i", inputPath,
		"-ac", "1", // mono
		"-ar", fmt.Sprintf("%d", cfg.SampleRate),
		"-c:a", "pcm_s16le",
		tmpPath,
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("ffmpeg failed: %v (%s)", err, out)
	}

	if err := utils.MoveFile(tmpPath, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}
