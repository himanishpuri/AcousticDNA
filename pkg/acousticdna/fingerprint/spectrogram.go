package fingerprint

import (
	"errors"
	"math"
	"math/cmplx"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/mjibson/go-dsp/fft"
)

const (
	WindowSize = 1024
	HopSize    = 256
)

func Hamming(n int) []float64 {
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		w[i] = 0.54 - 0.46*mathCos(2*mathPi*float64(i)/float64(n-1))
	}
	return w
}

// small local constants & helpers to avoid importing math multiple times in docs
const (
	mathPi = 3.141592653589793
)

func mathCos(x float64) float64 { return mathCosStd(x) }

func mathCosStd(x float64) float64 { return math.Cos(x) }

func FFTReal(frame []float64) []complex128 {
	return fft.FFTReal(frame)
}

func MagnitudeSpectrum(spectrum []complex128) []float64 {
	n := len(spectrum)
	half := n / 2
	mag := make([]float64, half)
	for i := 0; i < half; i++ {
		mag[i] = cmplx.Abs(spectrum[i])
	}
	return mag
}

func STFT(samples []float64, sampleRate, windowSize, hopSize int, window []float64) ([][]float64, error) {
	if len(window) != windowSize {
		return nil, errors.New("window length must equal windowSize")
	}
	if len(samples) < windowSize {
		return nil, errors.New("input shorter than window size")
	}

	spectrogram := make([][]float64, 0)
	for start := 0; start+windowSize <= len(samples); start += hopSize {
		end := start + windowSize
		frame := make([]float64, windowSize)
		copy(frame, samples[start:end])
		for i := 0; i < windowSize; i++ {
			frame[i] *= window[i]
		}
		spec := FFTReal(frame)
		mag := MagnitudeSpectrum(spec)
		spectrogram = append(spectrogram, mag)
	}
	return spectrogram, nil
}

func ComputeSpectrogram(wavPath string, windowSizeArg, hopSizeArg int) ([][]float64, int, error) {
	samples, sr, err := audio.ReadWavAsFloat64(wavPath)
	if err != nil {
		return nil, 0, err
	}

	ws := windowSizeArg
	if ws == 0 {
		ws = WindowSize
	}
	hs := hopSizeArg
	if hs == 0 {
		hs = HopSize
	}

	win := Hamming(ws)

	spectrogram, err := STFT(samples, sr, ws, hs, win)
	if err != nil {
		return nil, 0, err
	}
	return spectrogram, sr, nil
}

func ComputeSpectrogramFromSamples(samples []float64, sampleRate, windowSizeArg, hopSizeArg int) ([][]float64, error) {
	if len(samples) == 0 {
		return nil, errors.New("samples cannot be empty")
	}
	if sampleRate <= 0 {
		return nil, errors.New("sample rate must be positive")
	}

	ws := windowSizeArg
	if ws == 0 {
		ws = WindowSize
	}
	hs := hopSizeArg
	if hs == 0 {
		hs = HopSize
	}

	if len(samples) < ws {
		return nil, errors.New("audio too short for window size")
	}

	win := Hamming(ws)

	spectrogram, err := STFT(samples, sampleRate, ws, hs, win)
	if err != nil {
		return nil, err
	}
	return spectrogram, nil
}
