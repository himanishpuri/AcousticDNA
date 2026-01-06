package fingerprint

import (
	"errors"
	"math"
	"math/cmplx"

	"github.com/himanishpuri/AcousticDNA/internal/audio"
	"github.com/mjibson/go-dsp/fft"
)

// Tunables
const (
	WindowSize = 1024
	HopSize    = 256
)

// Hamming returns a Hamming window of length n.
func Hamming(n int) []float64 {
	w := make([]float64, n)
	for i := 0; i < n; i++ {
		// Hamming: 0.54 - 0.46*cos(2*pi*n/(N-1))
		w[i] = 0.54 - 0.46*mathCos(2*mathPi*float64(i)/float64(n-1))
	}
	return w
}

// small local constants & helpers to avoid importing math multiple times in docs
const (
	mathPi = 3.141592653589793
)

func mathCos(x float64) float64 { return mathCosStd(x) }

// We define a thin wrapper to the standard library's math.Cos to keep the imports
// in one place in this file for readability (and to make the code snippet self-contained).
// In production you can call math.Cos directly and remove these helpers.

func mathCosStd(x float64) float64 { return math.Cos(x) }

// FFTReal wraps the go-dsp FFT function and returns a complex spectrum.
func FFTReal(frame []float64) []complex128 {
	// the library provides FFT on real inputs
	return fft.FFTReal(frame)
}

// MagnitudeSpectrum converts a complex spectrum into a magnitude spectrum (positive freqs only)
func MagnitudeSpectrum(spectrum []complex128) []float64 {
	n := len(spectrum)
	half := n / 2
	mag := make([]float64, half)
	for i := 0; i < half; i++ {
		mag[i] = cmplx.Abs(spectrum[i])
	}
	return mag
}

// STFT computes the short-time FFT (spectrogram) and returns a time-major
// magnitude spectrogram: spectrogram[frameIdx][freqBin].
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
		// apply window
		for i := 0; i < windowSize; i++ {
			frame[i] *= window[i]
		}
		// compute FFT
		spec := FFTReal(frame)
		mag := MagnitudeSpectrum(spec)
		spectrogram = append(spectrogram, mag)
	}
	return spectrogram, nil
}

// ComputeSpectrogram is the top-level helper that reads a 16-bit PCM WAV file,
// converts it to float64 mono samples, builds the Hamming window (if nil),
// runs the STFT and returns the spectrogram. It uses package-level defaults when
// windowSize/hopSize are zero.
func ComputeSpectrogram(wavPath string, windowSizeArg, hopSizeArg int) ([][]float64, int, error) {
	// load samples
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
