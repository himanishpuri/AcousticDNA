# AcousticDNA WASM - Browser-Based Audio Fingerprinting

WebAssembly implementation of AcousticDNA for privacy-preserving audio fingerprinting in the browser.

## 🎯 Overview

The WASM module allows users to generate audio fingerprints **entirely in the browser** without uploading raw audio files to the server. Only the computed fingerprint hashes are sent to the server for matching, preserving user privacy.

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      BROWSER (Client)                        │
├─────────────────────────────────────────────────────────────┤
│  1. User uploads audio file (via Web UI)                    │
│  2. Web Audio API decodes → Float64Array samples             │
│  3. WASM generates fingerprint HASHES                        │
│  4. Send ONLY hashes to server (privacy!)                   │
│  5. Receive match results from server                       │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   │ POST /api/match/hashes
                   │ { hashes: { 123456: 1000, ... } }
                   ▼
┌─────────────────────────────────────────────────────────────┐
│                    BACKEND SERVER (Go)                       │
├─────────────────────────────────────────────────────────────┤
│  1. Validate hashes (format & count)                         │
│  2. Batch query SQLite database (single SQL query)          │
│  3. Perform time-coherence voting                            │
│  4. Return ranked results                                    │
└─────────────────────────────────────────────────────────────┘
```

## 🚀 Quick Start

### Build WASM Module

```bash
# From project root
./scripts/build-wasm.sh
```

This creates:

- `web/public/fingerprint.wasm` - WASM binary (~2.7 MB, ~760 KB gzipped)
- `web/public/wasm_exec.js` - Go WASM runtime

### Serve Web Application

**Option 1: Using npx (recommended)**

```bash
cd web && npx serve public
# Opens at http://localhost:3000
```

**Option 2: Using Python**

```bash
cd web/public && python3 -m http.server 8000
# Opens at http://localhost:8000
```

**Option 3: Using Go**

```bash
# Install: go install github.com/shurcooL/goexec@latest
cd web/public && goexec 'http.ListenAndServe(`:8080`, http.FileServer(http.Dir(`.`)))'
```

### Test in Browser

1. Open http://localhost:8000 (or your port)
2. Select an audio file (MP3, WAV, OGG, M4A, FLAC)
3. Click "Generate Fingerprint"
4. Click "Match Song" to search the database

## 📁 File Structure

```
AcousticDNA/
├── cmd/wasm/
│   └── main.go                  # WASM entry point (180 lines)
├── pkg/acousticdna/fingerprint/
│   └── spectrogram.go           # Added ComputeSpectrogramFromSamples()
├── scripts/
│   └── build-wasm.sh            # Build automation
├── web/
│   ├── public/
│   │   ├── index.html           # Demo UI (500+ lines)
│   │   ├── fingerprint.wasm     # Compiled binary (generated)
│   │   └── wasm_exec.js         # Go runtime (generated)
│   └── src/api/
│       └── wasm.js              # JavaScript API wrapper (350+ lines)
```

## 🔧 How It Works

### 1. Audio Decoding (Browser)

```javascript
// User uploads file
const file = input.files[0];

// Web Audio API decodes to PCM samples
const audioContext = new AudioContext();
const arrayBuffer = await file.arrayBuffer();
const audioBuffer = await audioContext.decodeAudioData(arrayBuffer);

// Extract samples (mono or stereo)
const samples = Array.from(audioBuffer.getChannelData(0));
```

### 2. Fingerprint Generation (WASM)

```javascript
// Call WASM function
const result = window.generateFingerprint(
	samples, // Float64Array of audio samples
	audioBuffer.sampleRate, // 44100, 48000, etc.
	audioBuffer.numberOfChannels, // 1 (mono) or 2 (stereo)
);

// Result format:
// {
//   error: 0,  // 0 = success
//   data: [
//     { hash: 123456, anchorTime: 1000 },
//     { hash: 789012, anchorTime: 1500 },
//     ...
//   ]
// }
```

### 3. Server Matching (HTTP)

```javascript
// Convert to server format
const hashMap = {};
for (const { hash, anchorTime } of result.data) {
	hashMap[hash] = anchorTime;
}

// Send to server
const response = await fetch("http://localhost:8080/api/match/hashes", {
	method: "POST",
	headers: { "Content-Type": "application/json" },
	body: JSON.stringify({ hashes: hashMap }),
});

const matches = await response.json();
// { matches: [...], count: 3 }
```

## 🛠️ API Reference

### JavaScript API (`web/src/api/wasm.js`)

#### `wasmLoader.init()`

Initializes the WASM module. Must be called before any other operations.

```javascript
import { wasmLoader } from "./src/api/wasm.js";

await wasmLoader.init();
console.log("WASM ready:", wasmLoader.ready);
```

#### `wasmLoader.processAudioFile(file, progressCallback)`

Processes an audio file and returns fingerprint hashes.

```javascript
const file = document.getElementById("input").files[0];

const hashes = await wasmLoader.processAudioFile(file, (progress) => {
	console.log(`${progress.stage}: ${progress.progress}%`);
});

console.log(`Generated ${hashes.length} hashes`);
// [{ hash: 123456, anchorTime: 1000 }, ...]
```

**Progress Stages:**

- `reading` (0%) - Reading file
- `decoding` (25%) - Web Audio API decoding
- `extracting` (50%) - Extracting samples
- `fingerprinting` (75%) - Generating hashes
- `complete` (100%) - Done

#### `wasmLoader.matchHashes(hashes, serverUrl)`

Sends hashes to the server for matching.

```javascript
const results = await wasmLoader.matchHashes(hashes, "http://localhost:8080");

console.log(`Found ${results.count} matches:`);
results.matches.forEach((match) => {
	console.log(`  ${match.title} by ${match.artist} (${match.confidence}%)`);
});
```

#### `wasmLoader.processAndMatch(file, serverUrl, progressCallback)`

Complete workflow: process file + match against server.

```javascript
const results = await wasmLoader.processAndMatch(
	file,
	"http://localhost:8080",
	(progress) => console.log(progress.stage),
);
```

### WASM Function (`window.generateFingerprint`)

Direct access to the WASM fingerprinting function.

```javascript
const result = window.generateFingerprint(
	samples, // Float64Array or Array of numbers
	sampleRate, // Number (Hz) - e.g., 44100
	channels, // Number - 1 (mono) or 2 (stereo)
);

if (result.error === 0) {
	console.log("Success:", result.data);
} else {
	console.error("Error:", result.data);
}
```

**Error Codes:**

- `0` - Success
- `1` - Invalid arguments
- `2` - Processing error
- `3` - Spectrogram generation failed
- `4` - Peak extraction failed
- `5` - Hash generation failed

## 🎨 UI Demo Features

The included `index.html` demonstrates:

- ✅ **Drag-and-drop** file upload
- ✅ **Real-time progress** indicators
- ✅ **Fingerprint statistics** (hash count, processing time)
- ✅ **Match results** with confidence scores
- ✅ **Responsive design** (mobile-friendly)
- ✅ **Error handling** with user-friendly messages
- ✅ **YouTube links** for matched songs

## 📊 Performance

### Typical Results

| Audio Duration | Samples   | Hashes  | Processing Time | Transfer Size |
| -------------- | --------- | ------- | --------------- | ------------- |
| 10 seconds     | 441,000   | ~1,200  | 500-800ms       | ~10 KB        |
| 30 seconds     | 1,323,000 | ~3,600  | 1.5-2.5s        | ~30 KB        |
| 3 minutes      | 7,938,000 | ~21,600 | 8-12s           | ~170 KB       |

### WASM Binary Size

- **Uncompressed**: 2.7 MB
- **Gzipped**: ~760 KB (network transfer)
- **Brotli**: ~650 KB (if server supports)

### Browser Compatibility

✅ **Supported:**

- Chrome/Edge 57+
- Firefox 52+
- Safari 11+
- Mobile Safari (iOS 11+)
- Chrome Android

❌ **Not Supported:**

- Internet Explorer (no WASM support)

## 🔒 Privacy Benefits

### What Stays on Device

- ✅ Raw audio files
- ✅ Decoded audio samples
- ✅ Spectrograms
- ✅ Peak data

### What Gets Sent to Server

- ✅ Only fingerprint hashes (cryptographic integers)
- ✅ Anchor times (timestamps in milliseconds)

**Example:** A 3-minute song generates ~20,000 hashes → ~170 KB of data sent vs. ~30 MB for the original audio file.

**Privacy Impact:** Server cannot reconstruct the original audio from hashes.

## 🐛 Troubleshooting

### "WASM initialization failed"

**Cause:** WASM binary not found or Go runtime missing.

**Fix:**

1. Run `./scripts/build-wasm.sh` to generate WASM files
2. Ensure `fingerprint.wasm` and `wasm_exec.js` are in `web/public/`
3. Serve from the correct directory (`web/public/`)

### "Failed to decode audio"

**Cause:** Unsupported audio format or corrupt file.

**Fix:**

1. Try converting to MP3 or WAV
2. Check file integrity
3. Ensure file is actual audio (not video)

### "No peaks found in audio"

**Cause:** Audio is silent, too quiet, or very short.

**Fix:**

1. Check audio contains actual sound
2. Ensure audio is at least 3-5 seconds long
3. Normalize audio volume if very quiet

### CORS Errors

**Cause:** WASM loaded from different origin than API server.

**Fix:**

1. Serve WASM and API from same origin, OR
2. Configure CORS on server:
   ```bash
   ./bin/server -origins "http://localhost:8000"
   ```

### Large WASM Binary Slow to Load

**Cause:** WASM binary is 2.7 MB uncompressed.

**Fix:**

1. Enable gzip/brotli compression on server
2. Use CDN for caching
3. Implement lazy loading (load WASM only when needed)

## 🧪 Testing

### Manual Testing

```bash
# 1. Build WASM
./scripts/build-wasm.sh

# 2. Start server
cd web && npx serve public

# 3. Open browser console at http://localhost:3000

# 4. Test WASM function directly
const testSamples = new Array(44100).fill(0).map(() => Math.random() * 0.1);
const result = generateFingerprint(testSamples, 44100, 1);
console.log(result);
```

### Browser Console Commands

```javascript
// Check WASM status
console.log("WASM ready:", window.acousticDNA.ready);

// Process a file programmatically
const file = document.getElementById("audioFile").files[0];
const hashes = await window.acousticDNA.processAudioFile(file);
console.log(`Generated ${hashes.length} hashes`);

// Match against server
const results = await window.acousticDNA.matchHashes(hashes);
console.log(results);
```

### Expected Hash Counts

Audio files should generate approximately:

- **Short audio (10s)**: 800-1,500 hashes
- **Medium audio (30s)**: 2,500-4,500 hashes
- **Long audio (3min)**: 15,000-25,000 hashes

If hash count is far outside these ranges, check audio quality.

## 🔄 Rebuilding WASM

After modifying Go code:

```bash
# Rebuild WASM
./scripts/build-wasm.sh

# Hard refresh browser (Ctrl+Shift+R or Cmd+Shift+R)
# or clear cache
```

## 📝 Limitations

### Current Limitations

1. **No streaming**: Entire audio file must be loaded into memory
2. **Synchronous processing**: UI blocks during fingerprinting
3. **No Web Workers**: Processing happens on main thread
4. **Memory usage**: ~10-20 MB for 3-minute audio

### Future Enhancements

- [ ] Implement Web Workers for background processing
- [ ] Add streaming support for large files
- [ ] Reduce WASM binary size with TinyGo
- [ ] Add progress bars for long operations
- [ ] Implement audio visualization (waveform/spectrogram)
- [ ] Add batch processing for multiple files

## 🤝 Integration

### Embedding in Your Web App

```html
<!DOCTYPE html>
<html>
	<head>
		<script type="module">
			import { wasmLoader } from "./src/api/wasm.js";

			window.addEventListener("load", async () => {
				await wasmLoader.init();
				console.log("AcousticDNA ready!");
			});
		</script>
	</head>
	<body>
		<input
			type="file"
			id="audioInput"
			accept="audio/*"
		/>
		<button onclick="processAudio()">Process</button>

		<script type="module">
			import { wasmLoader } from "./src/api/wasm.js";

			window.processAudio = async () => {
				const file = document.getElementById("audioInput").files[0];
				const results = await wasmLoader.processAndMatch(file);
				alert(`Match: ${results.matches[0]?.title || "None"}`);
			};
		</script>
	</body>
</html>
```

### Framework Integration (React Example)

```javascript
import { useEffect, useState } from "react";
import { wasmLoader } from "./api/wasm";

function AudioFingerprint() {
	const [ready, setReady] = useState(false);
	const [results, setResults] = useState(null);

	useEffect(() => {
		wasmLoader.init().then(() => setReady(true));
	}, []);

	const handleFile = async (e) => {
		const file = e.target.files[0];
		const matches = await wasmLoader.processAndMatch(file);
		setResults(matches);
	};

	return ready ? (
		<input
			type="file"
			onChange={handleFile}
			accept="audio/*"
		/>
	) : (
		<div>Loading WASM...</div>
	);
}
```

## 📚 Additional Resources

- [Go WebAssembly Wiki](https://github.com/golang/go/wiki/WebAssembly)
- [Web Audio API](https://developer.mozilla.org/en-US/docs/Web/API/Web_Audio_API)
- [AcousticDNA Server API](../cmd/server/README.md)
- [AcousticDNA Algorithm](../docs/ALGORITHM.md)

## 🎓 Learning More

### Understanding the Fingerprinting Process

1. **Spectrogram Generation**: Convert audio to time-frequency representation
2. **Peak Extraction**: Identify prominent spectral landmarks
3. **Hash Generation**: Create unique identifiers from peak pairs
4. **Time Coherence**: Match using temporal alignment

See [docs/ALGORITHM.md](../docs/ALGORITHM.md) for details.

## 📄 License

Same as main project - see root LICENSE file.
