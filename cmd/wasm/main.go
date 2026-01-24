//go:build js && wasm
// +build js,wasm

package main

import (
	"fmt"
	"syscall/js"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/fingerprint"
)

// Error codes returned to JavaScript
const (
	ErrorNone              = iota // Success
	ErrorInvalidArgs              // Invalid function arguments
	ErrorProcessing               // Error during audio processing
	ErrorSpectrogramFailed        // Spectrogram generation failed
	ErrorPeakExtraction           // Peak extraction failed
	ErrorHashGeneration           // Hash generation failed
)

// generateFingerprint processes audio samples and returns fingerprint hashes.
//
// JavaScript signature:
//
//	generateFingerprint(audioArray, sampleRate, channels)
//
// Parameters:
//   - audioArray: Float64Array or Array containing audio samples
//   - sampleRate: Number - sample rate in Hz (e.g., 44100, 11025)
//   - channels: Number - number of channels (1 = mono, 2 = stereo)
//
// Returns: JavaScript object { error: number, data: array | string }
//   - error: 0 = success, >0 = error code (see constants above)
//   - data: On success, array of {hash: number, anchorTime: number}
//     On error, string with error message
func generateFingerprint(this js.Value, args []js.Value) interface{} {
	// Validate argument count
	if len(args) < 3 {
		return makeErrorResponse(ErrorInvalidArgs, "Expected 3 arguments: audioArray, sampleRate, channels")
	}

	// Extract and validate arguments
	audioDataJS := args[0]
	sampleRateJS := args[1]
	channelsJS := args[2]

	// Validate types
	if audioDataJS.Type() != js.TypeObject {
		return makeErrorResponse(ErrorInvalidArgs, "audioArray must be an Array or Float64Array")
	}
	if sampleRateJS.Type() != js.TypeNumber {
		return makeErrorResponse(ErrorInvalidArgs, "sampleRate must be a number")
	}
	if channelsJS.Type() != js.TypeNumber {
		return makeErrorResponse(ErrorInvalidArgs, "channels must be a number")
	}

	sampleRate := sampleRateJS.Int()
	channels := channelsJS.Int()

	// Validate values
	if sampleRate <= 0 {
		return makeErrorResponse(ErrorInvalidArgs, fmt.Sprintf("Invalid sample rate: %d", sampleRate))
	}
	if channels < 1 || channels > 2 {
		return makeErrorResponse(ErrorInvalidArgs, fmt.Sprintf("Channels must be 1 (mono) or 2 (stereo), got: %d", channels))
	}

	// Extract audio samples from JavaScript array
	length := audioDataJS.Length()
	if length == 0 {
		return makeErrorResponse(ErrorInvalidArgs, "audioArray is empty")
	}

	samples := make([]float64, length)
	for i := 0; i < length; i++ {
		val := audioDataJS.Index(i)
		if val.Type() != js.TypeNumber {
			return makeErrorResponse(ErrorInvalidArgs, fmt.Sprintf("audioArray element %d is not a number", i))
		}
		samples[i] = val.Float()
	}

	// Convert stereo to mono if needed
	if channels == 2 {
		samples = stereoToMono(samples)
	}

	// Calculate audio duration for peak extraction
	duration := float64(len(samples)) / float64(sampleRate)

	// Generate spectrogram using in-memory function
	spec, err := fingerprint.ComputeSpectrogramFromSamples(samples, sampleRate, 0, 0)
	if err != nil {
		return makeErrorResponse(ErrorSpectrogramFailed, fmt.Sprintf("Failed to generate spectrogram: %v", err))
	}

	// Extract peaks from spectrogram
	peaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	if len(peaks) == 0 {
		return makeErrorResponse(ErrorPeakExtraction, "No peaks found in audio (audio may be silent or too short)")
	}

	// Generate fingerprint hashes
	// Use songID=0 for query fingerprints (not storing in database)
	fingerprintMap := fingerprint.Fingerprint(peaks, 0)
	if len(fingerprintMap) == 0 {
		return makeErrorResponse(ErrorHashGeneration, "No fingerprint hashes generated")
	}

	// Convert Go map to JavaScript-friendly array
	hashArray := js.Global().Get("Array").New()
	idx := 0
	for hash, couples := range fingerprintMap {
		// For query fingerprints, we only care about the anchor times
		// Each hash may have multiple couples (from the same query with different anchor times)
		for _, couple := range couples {
			hashObj := js.Global().Get("Object").New()
			hashObj.Set("hash", hash)
			hashObj.Set("anchorTime", couple.AnchorTimeMs)
			hashArray.SetIndex(idx, hashObj)
			idx++
		}
	}

	// Return success response
	result := js.Global().Get("Object").New()
	result.Set("error", ErrorNone)
	result.Set("data", hashArray)
	return result
}

// stereoToMono converts stereo samples (interleaved L/R) to mono by averaging channels
func stereoToMono(stereo []float64) []float64 {
	if len(stereo)%2 != 0 {
		// Odd length, truncate the last sample
		stereo = stereo[:len(stereo)-1]
	}

	monoLength := len(stereo) / 2
	mono := make([]float64, monoLength)

	for i := 0; i < monoLength; i++ {
		left := stereo[i*2]
		right := stereo[i*2+1]
		mono[i] = (left + right) / 2.0
	}

	return mono
}

// makeErrorResponse creates a JavaScript error response object
func makeErrorResponse(errorCode int, message string) js.Value {
	result := js.Global().Get("Object").New()
	result.Set("error", errorCode)
	result.Set("data", message)
	return result
}

// main is the entry point for the WASM module
func main() {
	// Log to console early for debugging
	console := js.Global().Get("console")
	if !console.IsUndefined() {
		console.Call("log", "🔧 AcousticDNA WASM module initializing...")
	}

	// Create a channel to prevent the program from exiting
	done := make(chan struct{})

	// Register the generateFingerprint function globally
	js.Global().Set("generateFingerprint", js.FuncOf(generateFingerprint))

	if !console.IsUndefined() {
		console.Call("log", "📝 generateFingerprint function registered")
	}

	// Notify JavaScript that WASM is ready
	// Dispatch a custom event that the JavaScript loader can listen for
	window := js.Global().Get("window")
	if !window.IsUndefined() {
		if !console.IsUndefined() {
			console.Call("log", "📤 Dispatching wasmReady event...")
		}
		eventInit := js.Global().Get("Object").New()
		event := js.Global().Get("CustomEvent").New("wasmReady", eventInit)
		window.Call("dispatchEvent", event)
		if !console.IsUndefined() {
			console.Call("log", "✅ wasmReady event dispatched")
		}
	} else {
		if !console.IsUndefined() {
			console.Call("error", "❌ window object is undefined!")
		}
	}

	if !console.IsUndefined() {
		console.Call("log", "✅ AcousticDNA WASM module loaded and ready")
	}

	// Keep the Go runtime alive
	// Without this, the program would exit and WASM functions would become unavailable
	<-done
}
