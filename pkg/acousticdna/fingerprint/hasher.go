package fingerprint

import (
	"math"
)

const (
	MaxFreqBits  = 9
	MaxDeltaBits = 14
	FanOut       = 6
	MinDeltaMs   = 10
	MaxDeltaMs   = 15000
	UseFreqIdx   = true
)

func createAddress(anchor Peak, target Peak) (uint32, bool) {
	var anchorFreqVal uint32
	var targetFreqVal uint32
	if UseFreqIdx {
		anchorFreqVal = uint32(anchor.FreqIdx)
		targetFreqVal = uint32(target.FreqIdx)
	} else {
		anchorFreqVal = uint32(math.Floor(anchor.Freq/10.0 + 0.5))
		targetFreqVal = uint32(math.Floor(target.Freq/10.0 + 0.5))
	}

	deltaMs := uint32(math.Round((target.Time - anchor.Time) * 1000.0))

	if deltaMs < MinDeltaMs || deltaMs > MaxDeltaMs {
		return 0, false
	}

	maxFreqMask := uint32((1 << MaxFreqBits) - 1)
	maxDeltaMask := uint32((1 << MaxDeltaBits) - 1)

	if anchorFreqVal > maxFreqMask || targetFreqVal > maxFreqMask {
		return 0, false
	}
	if deltaMs > maxDeltaMask {
		return 0, false
	}

	shiftTarget := MaxDeltaBits
	shiftAnchor := MaxDeltaBits + MaxFreqBits

	address := (anchorFreqVal << shiftAnchor) | (targetFreqVal << shiftTarget) | (deltaMs & maxDeltaMask)
	return address, true
}
