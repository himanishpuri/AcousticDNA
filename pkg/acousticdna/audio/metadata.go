package audio

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type Metadata struct {
	Filename    string
	Title       string
	Artist      string
	Album       string
	Encoder     string
	DurationSec float64
	SampleRate  int
	Channels    int
	BitDepth    int
	Format      string
}

type ffprobeOutput struct {
	Format struct {
		Filename string            `json:"filename"`
		Duration string            `json:"duration"`
		Format   string            `json:"format_name"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	CodecType     string `json:"codec_type"`
	SampleRate    string `json:"sample_rate"`
	Channels      int    `json:"channels"`
	BitsPerSample int    `json:"bits_per_sample"`
}

func (p *ffprobeOutput) firstAudioStream() *ffprobeStream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "audio" {
			return &p.Streams[i]
		}
	}
	return nil
}

func ReadMetadataFFmpeg(ctx context.Context, path string) (*Metadata, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}

	audioStream := probe.firstAudioStream()
	if audioStream == nil {
		return nil, errors.New("no audio stream found")
	}

	duration, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	sampleRate, _ := strconv.Atoi(audioStream.SampleRate)

	meta := &Metadata{
		Filename:    filepath.Base(path),
		DurationSec: duration,
		SampleRate:  sampleRate,
		Channels:    audioStream.Channels,
		BitDepth:    audioStream.BitsPerSample,
		Format:      probe.Format.Format,
	}

	if probe.Format.Tags != nil {
		meta.Title = probe.Format.Tags["title"]
		meta.Artist = probe.Format.Tags["artist"]
		meta.Album = probe.Format.Tags["album"]
		meta.Encoder = probe.Format.Tags["encoder"]
	}

	return meta, nil
}
