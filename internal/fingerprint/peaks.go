package fingerprint

import (
	"math"
	"sort"
)

// Peak represents a spectral landmark used for fingerprinting.
// It contains both index and physical units for convenience.
type Peak struct {
	TimeIdx int     // frame index in the spectrogram
	FreqIdx int     // frequency bin index
	Time    float64 // time in seconds
	Freq    float64 // frequency in Hz
	MagDB   float64 // magnitude in dB (useful for debugging/tuning)
}

// ExtractPeaks finds robust spectral peaks (constellation points) from a
// magnitude spectrogram. The function assumes spectrogram[frameIdx][freqBin]
// contains linear magnitude values (not complex) and that the STFT used a
// window length of WindowSize and hop length of HopSize (package-level
// constants defined in spectrogram.go).
//
// Parameters:
//   - spectrogram: time-major magnitude spectrogram
//   - audioDuration: total audio length in seconds (not strictly required;
//     time is computed from hop size and sample rate)
//   - sampleRate: sample rate (Hz) of the audio used to compute the STFT
//
// Returns a slice of Peak sorted by (time then frequency) in appearance order.
func ExtractPeaks(spectrogram [][]float64, audioDuration float64, sampleRate int) []Peak {
	if len(spectrogram) == 0 || len(spectrogram[0]) == 0 {
		return nil
	}

	nFrames := len(spectrogram)
	nBins := len(spectrogram[0])

	// Frequency resolution: Hz per FFT bin (no dspRatio here).
	freqRes := float64(sampleRate) / float64(WindowSize)
	// Time per frame using the package-level HopSize.
	frameTime := float64(HopSize) / float64(sampleRate)

	// Parameters you can tune
	const (
		// size of the 2D local neighborhood for local-max check
		freqNeighbour = 3 // +/- bins in frequency
		timeNeighbour = 1 // +/- frames in time
		// minimum dB above local band average to accept a peak
		minDbAboveAvg = 3.0
		// floor to avoid log(0)
		eps = 1e-10
	)

	// Build simple log-ish frequency bands (clamped to nBins).
	// If nBins is small, this will still behave sensibly.
	bands := [][]int{{0, minInt(10, nBins)}}
	for start := 10; start < nBins; start *= 2 {
		end := minInt(start*2, nBins)
		bands = append(bands, []int{start, end})
		if end == nBins {
			break
		}
	}

	peaks := make([]Peak, 0, nFrames*2) // rough capacity guess

	// For each frame, pick the strongest bin per band, then apply local checks
	for t := 0; t < nFrames; t++ {
		frame := spectrogram[t]

		// collect band maxima
		bandMaxMag := make([]float64, 0, len(bands))
		bandMaxIdx := make([]int, 0, len(bands))
		for _, b := range bands {
			minBin := b[0]
			maxBin := b[1]
			if minBin >= nBins {
				bandMaxMag = append(bandMaxMag, 0)
				bandMaxIdx = append(bandMaxIdx, minBin)
				continue
			}
			if maxBin > nBins {
				maxBin = nBins
			}
			maxMag := 0.0
			maxIdx := minBin
			for i := minBin; i < maxBin; i++ {
				m := frame[i]
				if m > maxMag {
					maxMag = m
					maxIdx = i
				}
			}
			bandMaxMag = append(bandMaxMag, maxMag)
			bandMaxIdx = append(bandMaxIdx, maxIdx)
		}

		// compute average magnitude in dB across band maxima for adaptive thresholding
		var sumDb float64
		for _, mag := range bandMaxMag {
			sumDb += 20.0 * math.Log10(mag+eps)
		}
		avgDb := sumDb / float64(len(bandMaxMag))

		// For each band candidate, check local 2D neighborhood and threshold
		for bi, mag := range bandMaxMag {
			if mag <= 0 {
				continue
			}
			bin := bandMaxIdx[bi]
			magDb := 20.0 * math.Log10(mag+eps)

			// quick threshold: must be above average by a few dB
			if magDb < avgDb+minDbAboveAvg {
				continue
			}

			// local neighborhood check in time and frequency
			isLocalMax := true
			for dt := -timeNeighbour; dt <= timeNeighbour; dt++ {
				tIdx := t + dt
				if tIdx < 0 || tIdx >= nFrames {
					continue
				}
				for df := -freqNeighbour; df <= freqNeighbour; df++ {
					fIdx := bin + df
					if fIdx < 0 || fIdx >= nBins {
						continue
					}
					if dt == 0 && df == 0 {
						continue
					}
					if spectrogram[tIdx][fIdx] > mag {
						isLocalMax = false
						break
					}
				}
				if !isLocalMax {
					break
				}
			}

			if !isLocalMax {
				continue
			}

			// Passed checks — add peak
			p := Peak{
				TimeIdx: t,
				FreqIdx: bin,
				Time:    float64(t) * frameTime,
				Freq:    float64(bin) * freqRes,
				MagDB:   magDb,
			}
			peaks = append(peaks, p)
		}
	}

	// At end of ExtractPeaks, before return:
	sort.Slice(peaks, func(i, j int) bool {
		if peaks[i].TimeIdx == peaks[j].TimeIdx {
			return peaks[i].FreqIdx < peaks[j].FreqIdx
		}
		return peaks[i].TimeIdx < peaks[j].TimeIdx
	})

	return peaks
}

// helper: minInt
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
