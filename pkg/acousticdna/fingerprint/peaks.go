package fingerprint

import (
	"math"
	"sort"
)

type Peak struct {
	TimeIdx int
	FreqIdx int
	Time    float64
	Freq    float64
	MagDB   float64
}

func ExtractPeaks(spectrogram [][]float64, audioDuration float64, sampleRate int) []Peak {
	if len(spectrogram) == 0 || len(spectrogram[0]) == 0 {
		return nil
	}

	nFrames := len(spectrogram)
	nBins := len(spectrogram[0])

	freqRes := float64(sampleRate) / float64(WindowSize)
	frameTime := float64(HopSize) / float64(sampleRate)

	const (
		freqNeighbour = 3
		timeNeighbour = 1
		minDbAboveAvg = 3.0
		eps           = 1e-10
	)

	bands := [][]int{{0, minInt(10, nBins)}}
	for start := 10; start < nBins; start *= 2 {
		end := minInt(start*2, nBins)
		bands = append(bands, []int{start, end})
		if end == nBins {
			break
		}
	}

	peaks := make([]Peak, 0, nFrames*2)

	// For each frame, pick the strongest bin per band, then apply local checks
	for t := 0; t < nFrames; t++ {
		frame := spectrogram[t]

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

		var sumDb float64
		for _, mag := range bandMaxMag {
			sumDb += 20.0 * math.Log10(mag+eps)
		}
		avgDb := sumDb / float64(len(bandMaxMag))

		for bi, mag := range bandMaxMag {
			if mag <= 0 {
				continue
			}
			bin := bandMaxIdx[bi]
			magDb := 20.0 * math.Log10(mag+eps)

			if magDb < avgDb+minDbAboveAvg {
				continue
			}

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

	sort.Slice(peaks, func(i, j int) bool {
		if peaks[i].TimeIdx == peaks[j].TimeIdx {
			return peaks[i].FreqIdx < peaks[j].FreqIdx
		}
		return peaks[i].TimeIdx < peaks[j].TimeIdx
	})

	return peaks
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
