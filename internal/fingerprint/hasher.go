package fingerprint

import (
	"math"
)

// ------------------------ TUNABLES (change for experiments) ------------------------
const (
	// Number of bits allocated to frequency indices (must fit number of FFT bins)
	MaxFreqBits = 9

	// Number of bits allocated to delta time (milliseconds)
	// With 14 bits you can represent up to 16383 ms (~16.3s)
	MaxDeltaBits = 14

	// Fan-out: how many target peaks to pair with each anchor (per-anchor)
	FanOut = 6

	// Minimum and maximum delta time (ms) allowed for a pair
	MinDeltaMs = 10    // ignore extremely short deltas (likely same frame)
	MaxDeltaMs = 15000 // discard very long pairings

	// Whether to use Peak.FreqIdx (true) or Peak.Freq (Hz) quantized (/10) (false)
	// Using FreqIdx is preferred because it is deterministic from the FFT.
	UseFreqIdx = true
)


// ------------------------ Hash packing utilities ------------------------

// createAddress packs anchor/target frequency and delta time into a 32-bit key.
// Returns (address, ok). ok==false if the pair is out of representable bounds
// (e.g., delta too large or freq indices don't fit in allocated bits).
func createAddress(anchor Peak, target Peak) (uint32, bool) {
	// Choose frequency representation
	var anchorFreqVal uint32
	var targetFreqVal uint32
	if UseFreqIdx {
		anchorFreqVal = uint32(anchor.FreqIdx)
		targetFreqVal = uint32(target.FreqIdx)
	} else {
		// quantize by ~10Hz (as some references do); using Hz is less deterministic
		anchorFreqVal = uint32(math.Floor(anchor.Freq/10.0 + 0.5))
		targetFreqVal = uint32(math.Floor(target.Freq/10.0 + 0.5))
	}

	// delta in milliseconds (round)
	deltaMs := uint32(math.Round((target.Time - anchor.Time) * 1000.0))

	if deltaMs < MinDeltaMs || deltaMs > MaxDeltaMs {
		return 0, false
	}

	// Verify we can fit frequencies and delta in allotted bits
	maxFreqMask := uint32((1 << MaxFreqBits) - 1)
	maxDeltaMask := uint32((1 << MaxDeltaBits) - 1)

	if anchorFreqVal > maxFreqMask || targetFreqVal > maxFreqMask {
		// Frequencies don't fit — drop this pair
		return 0, false
	}
	if deltaMs > maxDeltaMask {
		return 0, false
	}

	// bit layout: [ anchorFreq (MaxFreqBits) | targetFreq (MaxFreqBits) | delta (MaxDeltaBits) ]
	shiftTarget := MaxDeltaBits
	shiftAnchor := MaxDeltaBits + MaxFreqBits

	address := (anchorFreqVal << shiftAnchor) | (targetFreqVal << shiftTarget) | (deltaMs & maxDeltaMask)
	return address, true
}
