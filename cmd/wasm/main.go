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
	ErrorNone = iota
	ErrorInvalidArgs
	ErrorProcessing
	ErrorSpectrogramFailed
	ErrorPeakExtraction
	ErrorHashGeneration
)

// Processes audio samples and returns fingerprint hashes.
// Returns: {error: number, data: array | string}
func generateFingerprint(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return makeErrorResponse(ErrorInvalidArgs, "Expected 3 arguments: audioArray, sampleRate, channels")
	}

	audioDataJS := args[0]
	sampleRateJS := args[1]
	channelsJS := args[2]

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

	if sampleRate <= 0 {
		return makeErrorResponse(ErrorInvalidArgs, fmt.Sprintf("Invalid sample rate: %d", sampleRate))
	}
	if channels < 1 || channels > 2 {
		return makeErrorResponse(ErrorInvalidArgs, fmt.Sprintf("Channels must be 1 (mono) or 2 (stereo), got: %d", channels))
	}

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

	if channels == 2 {
		samples = stereoToMono(samples)
	}

	duration := float64(len(samples)) / float64(sampleRate)

	spec, err := fingerprint.ComputeSpectrogramFromSamples(samples, sampleRate, 0, 0)
	if err != nil {
		return makeErrorResponse(ErrorSpectrogramFailed, fmt.Sprintf("Failed to generate spectrogram: %v", err))
	}

	peaks := fingerprint.ExtractPeaks(spec, duration, sampleRate)
	if len(peaks) == 0 {
		return makeErrorResponse(ErrorPeakExtraction, "No peaks found in audio (audio may be silent or too short)")
	}

	fingerprintMap := fingerprint.Fingerprint(peaks, "")
	if len(fingerprintMap) == 0 {
		return makeErrorResponse(ErrorHashGeneration, "No fingerprint hashes generated")
	}

	hashArray := js.Global().Get("Array").New()
	idx := 0
	for hash, couples := range fingerprintMap {
		for _, couple := range couples {
			hashObj := js.Global().Get("Object").New()
			hashObj.Set("hash", hash)
			hashObj.Set("anchorTime", couple.AnchorTimeMs)
			hashArray.SetIndex(idx, hashObj)
			idx++
		}
	}

	result := js.Global().Get("Object").New()
	result.Set("error", ErrorNone)
	result.Set("data", hashArray)
	return result
}

func stereoToMono(stereo []float64) []float64 {
	if len(stereo)%2 != 0 {
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

func makeErrorResponse(errorCode int, message string) js.Value {
	result := js.Global().Get("Object").New()
	result.Set("error", errorCode)
	result.Set("data", message)
	return result
}

func main() {
	console := js.Global().Get("console")
	if !console.IsUndefined() {
		console.Call("log", "🔧 AcousticDNA WASM module initializing...")
	}

	done := make(chan struct{})

	js.Global().Set("generateFingerprint", js.FuncOf(generateFingerprint))

	if !console.IsUndefined() {
		console.Call("log", "📝 generateFingerprint function registered")
	}

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

	<-done
}
