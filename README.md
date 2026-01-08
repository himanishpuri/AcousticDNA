# 🎵 AcousticDNA

[![Go Version](https://img.shields.io/badge/Go-1.25.5-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()

**AcousticDNA** is a Shazam-like audio fingerprinting system built from scratch in Go. It can identify songs from short audio clips by generating unique acoustic fingerprints and matching them against a database using time-coherence voting algorithms.

---

## 🎯 What It Does

AcousticDNA analyzes audio files to create unique "fingerprints" that can identify songs even from short, noisy clips. The system:

1. **Ingests audio files** (any format supported by FFmpeg: MP3, WAV, FLAC, AAC, M4A, OGG, etc.)
2. **Converts to mono 16-bit PCM WAV** at 11,025 Hz sample rate for normalized processing
3. **Generates spectrograms** using Short-Time Fourier Transform (STFT) with Hamming windowing
4. **Extracts spectral peaks** (constellation points) as acoustic landmarks
5. **Creates combinatorial hashes** from peak pairs using anchor-target pairing
6. **Stores fingerprints** in SQLite database with precise time-offset information
7. **Matches query audio** using time-coherence voting to find the best match
8. **Returns ranked results** with confidence scores and time alignment

---

## 🚀 Quick Start

### Prerequisites

-  **Go 1.25+** ([Download](https://go.dev/dl/))
-  **FFmpeg & FFprobe** ([Download](https://ffmpeg.org/download.html))

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Windows (Chocolatey)
choco install ffmpeg
```

### Installation

```bash
# Clone the repository
git clone https://github.com/himanishpuri/AcousticDNA.git
cd AcousticDNA

# Install dependencies
go mod download

# Build the CLI
go build -o acousticDNA ./cmd/cli/
```

### Usage

```bash
# Add a song to the database
./acousticDNA add song.mp3 --title "Sandstorm" --artist "Darude" --youtube "y6120QOlsfU"

# Match an audio clip
./acousticDNA match recording.wav

# List all indexed songs
./acousticDNA list

# Delete a song by ID
./acousticDNA delete 1
```

---

## 🧬 Core Algorithm

### Audio Processing Flow

```
┌─────────────────┐
│  Input Audio    │  (MP3, WAV, FLAC, etc.)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ FFmpeg Convert  │  → Mono 16-bit PCM @ 11,025 Hz
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  STFT + Peaks   │  → Spectrogram → Constellation Points
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Fingerprints   │  → Combinatorial Hashes (32-bit)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ SQLite Storage  │  → hash → (songID, anchorTimeMs)
└─────────────────┘
```

### Matching Algorithm

```
Query Audio → Fingerprints → Database Lookup
                                   │
                                   ▼
                         Time-Offset Voting
                         ┌─────────────────┐
                         │ For each match: │
                         │ offset = db_time│
                         │        - query  │
                         │ votes[song][off]│
                         │        += 1     │
                         └────────┬────────┘
                                  │
                                  ▼
                         Rank by Max Votes
                                  │
                                  ▼
                           Top Matches 🎯
```

### Spectrogram Visualization

Example spectrogram of "Sandstorm" by Darude generated using the included [`make-spectrogram.go`](make-spectorgram.go) script (done linearly, instead of logarithmically for representation purposes):

![Sandstorm Spectrogram](test/spectrograms/Sandstorm-Darude.wav.png)

_Frequency vs. Time representation showing the audio's spectral characteristics. Brighter regions indicate higher energy at specific frequencies._

---

## ⚙️ Technical Specifications

### DSP Parameters

| Parameter                | Value          | Description                        |
| ------------------------ | -------------- | ---------------------------------- |
| **Sample Rate**          | 11,025 Hz      | Optimized for audio fingerprinting |
| **Bit Depth**            | 16-bit PCM     | Signed integer format              |
| **Channels**             | Mono           | Stereo converted by averaging L+R  |
| **Window Size**          | 1024 samples   | STFT frame length                  |
| **Hop Size**             | 256 samples    | 75% overlap between frames         |
| **Window Function**      | Hamming        | 0.54 - 0.46×cos(2πn/(N-1))         |
| **Frequency Resolution** | ~10.77 Hz/bin  | 11,025 Hz / 1024                   |
| **Time Resolution**      | ~23.2 ms/frame | 256 / 11,025                       |

### Fingerprinting Parameters

| Parameter           | Value       | Description                       |
| ------------------- | ----------- | --------------------------------- |
| **Fan-Out**         | 6           | Each anchor paired with 6 targets |
| **Min Delta Time**  | 10 ms       | Ignore same-frame peaks           |
| **Max Delta Time**  | 15,000 ms   | Maximum 15-second pairing window  |
| **Hash Size**       | 32-bit      | Combinatorial hash encoding       |
| **Frequency Bits**  | 9 bits      | Per frequency index               |
| **Time Delta Bits** | 14 bits     | Supports up to 16,383 ms          |
| **Peak Detection**  | Adaptive    | 3 dB above local average          |
| **Frequency Bands** | Logarithmic | 0-10, 10-20, 20-40 Hz, etc.       |

### Hash Structure

```
32-bit Hash Layout:
┌─────────┬─────────┬──────────┐
│ AnchorF │ TargetF │   ΔTime  │
│  9 bits │  9 bits │ 14 bits  │
└─────────┴─────────┴──────────┘
```

---

## 🏗️ Project Structure

```
AcousticDNA/
├── cmd/
│   └── cli/
│       └── main.go              # CLI entry point (add/match/list/delete)
├── internal/
│   ├── audio/
│   │   ├── processor.go         # FFmpeg audio conversion
│   │   ├── reader.go            # Custom WAV parser
│   │   ├── metadata.go          # FFprobe metadata extraction
│   │   └── reader_test.go       # Unit tests
│   ├── fingerprint/
│   │   ├── spectrogram.go       # STFT implementation
│   │   ├── peaks.go             # Peak extraction
│   │   ├── generator.go         # Fingerprint generation & matching
│   │   ├── hasher.go            # Hash encoding/decoding
│   │   └── *_test.go            # Comprehensive tests
│   ├── storage/
│   │   ├── sqlite.go            # Database client (GORM)
│   │   └── sqlite_test.go       # DB operation tests
│   ├── service/
│   │   ├── service.go           # High-level orchestration layer
│   │   └── service_test.go      # End-to-end integration tests
│   └── model/
│       └── models.go            # Data structures (Couple, Match, Peak)
├── pkg/
│   ├── logger/
│   │   └── logger.go            # Custom structured logger
│   └── utils/
│       ├── files.go             # File system utilities
│       └── crypto.go            # Cryptographic helpers
├── test/
│   ├── testdata/                # Original test audio files
│   └── convertedtestdata/       # Processed WAV files (11025 Hz mono)
├── go.mod                       # Go module dependencies
├── go.sum                       # Dependency checksums
├── acousticDNA                  # Compiled binary
└── README.md                    # This file
```

---

## 🛠️ Technology Stack

### Core Dependencies

**Audio Processing:**

-  [`github.com/go-audio/wav`](https://github.com/go-audio/wav) - WAV file decoding
-  [`github.com/go-audio/audio`](https://github.com/go-audio/audio) - Audio buffer handling
-  **FFmpeg** (external binary) - Universal format conversion

**Digital Signal Processing:**

-  [`github.com/mjibson/go-dsp/fft`](https://github.com/mjibson/go-dsp) - Fast Fourier Transform
-  Custom STFT implementation with Hamming windowing

**Database:**

-  [`gorm.io/gorm`](https://gorm.io/) - ORM for database operations
-  [`github.com/glebarez/sqlite`](https://github.com/glebarez/sqlite) - Pure Go SQLite driver
-  SQLite3 for local persistence

**Utilities:**

-  [`github.com/eligwz/spectrogram`](https://github.com/eligwz/spectrogram) - Visualization (testing)
-  [`github.com/lrstanley/go-ytdlp`](https://github.com/lrstanley/go-ytdlp) - YouTube integration (planned)
-  Custom logger with color-coded output

---

## 📊 Performance Metrics

**Typical Performance** (M1 MacBook Pro / AMD Ryzen 7):

| Operation         | Time        | Details             |
| ----------------- | ----------- | ------------------- |
| **Indexing**      | 2-4 seconds | Per 3-minute song   |
| **Matching**      | 1-2 seconds | Per 10-second query |
| **Database Size** | 5-10 MB     | Per 100 songs       |
| **Memory Usage**  | 50-100 MB   | During processing   |

**Scalability:**

-  ✅ Tested with 1000+ songs
-  ✅ Sub-second matching for 5-second clips
-  ✅ SQLite performs well up to 100K songs
-  📝 Consider PostgreSQL for larger deployments

**Example Results:**

```bash
./acousticDNA match cropped_song.wav

✅ Found 1 match!
1. "CityBGM" by kimurasukuru
   Score: 491 | Confidence: 206.3% | Offset: 40890ms
   YouTube: https://youtube.com/watch?v=R7dM0xQZpVh
```

_Note: Confidence > 100% indicates multiple hash matches per peak (fan-out effect)_

---

## 📖 Detailed Usage

### 1. Adding Songs

```bash
./acousticDNA add <audio_file> --title <title> --artist <artist> [--youtube <id>]
```

**Example:**

```bash
./acousticDNA add ~/Music/Sandstorm.mp3 \
  --title "Sandstorm" \
  --artist "Darude" \
  --youtube "y6120QOlsfU"
```

**Output:**

```
   _                      _   _      ____  _   _    _
  / \   ___ ___  _   _ ___| |_(_) ___|  _ \| \ | |  / \
 / _ \ / __/ _ \| | | / __| __| |/ __| | | |  \| | / _ \
/ ___ \ (_| (_) | |_| \__ \ |_| | (__| |_| | |\  |/ ___ \
\_/   \_/___\___/ \__,_|___/\__|_|\___|____/|_| \_/_/   \_/

           Audio Fingerprinting CLI Tool

🔧 Initializing service...
🎵 Processing audio file...
   This may take a few moments for large files
[INFO] Processing song: Sandstorm by Darude
[INFO] Extracted 10812 peaks
[INFO] Generated 20205 unique hashes
[INFO] Successfully added song ID=1

✅ Successfully added song to database!
   ID:      1
   Title:   Sandstorm
   Artist:  Darude
   YouTube: y6120QOlsfU
```

### 2. Matching Audio

```bash
./acousticDNA match <audio_file>
```

**Example:**

```bash
./acousticDNA match recording.wav
```

**Output:**

```
🔍 Analyzing audio file...
   Generating fingerprints and searching database
[INFO] Query has 10812 peaks
[INFO] Generated 20205 query hashes
[INFO] Found 1 candidate matches

✅ Found 1 match(es)!

🎵 Top Matches:

1. "Sandstorm" by Darude
   Score: 64850 | Confidence: 599.8% | Offset: 0ms
   YouTube: https://youtube.com/watch?v=y6120QOlsfU
```

### 3. Listing Songs

```bash
./acousticDNA list
```

**Output:**

```
📚 Found 4 song(s):

1. "Sandstorm" by Darude (ID: 1)
   YouTube: https://youtube.com/watch?v=y6120QOlsfU
   Duration: 7:26

2. "Amapiano" by AudioClubz (ID: 2)
   YouTube: https://youtube.com/watch?v=kA9QeP2XrLm
   Duration: 2:20

3. "CityBGM" by kimurasukuru (ID: 3)
   YouTube: https://youtube.com/watch?v=R7dM0xQZpVh
   Duration: 1:43

4. "Immensity" by atlanticlights (ID: 4)
   YouTube: https://youtube.com/watch?v=mF3A2Lw9ZKe
   Duration: 3:19
```

### 4. Deleting Songs

```bash
./acousticDNA delete <song_id>
```

**Example:**

```bash
./acousticDNA delete 4
```

**Output:**

```
✅ Successfully deleted song:
   ID:     4
   Title:  Immensity
   Artist: atlanticlights
[INFO] Deleted song ID=4 ('Immensity' by 'atlanticlights')
```

---

## 🧪 Testing

### Running Tests

```bash
# All tests
go test ./...

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/fingerprint -v

# Run service integration tests
go test ./internal/service -v
```

### Test Coverage

The project includes comprehensive tests:

-  ✅ **Unit tests** for all core modules
-  ✅ **Integration tests** for service layer
-  ✅ **End-to-end workflow tests**
-  ✅ 25+ test cases with >80% coverage

**Example test output:**

```bash
=== RUN   TestEndToEndFlow
    service_test.go:299: Step 1: Adding song to database...
    service_test.go:304: ✓ Song added with ID=1
    service_test.go:309: ✓ Stored 64850 fingerprints
    service_test.go:316: Step 2: Matching audio...
    service_test.go:325: ✓ Found 1 matches
    service_test.go:329: Top match: ID=1, Title='E2E Test Song', Score=64850, Confidence=599.80%
    service_test.go:341: ✓ End-to-end flow completed successfully
--- PASS: TestEndToEndFlow (9.20s)
```

---

## ⚙️ Configuration

### Environment Variables

| Variable               | Default                 | Description                        |
| ---------------------- | ----------------------- | ---------------------------------- |
| `ACOUSTIC_DB_PATH`     | `./acousticdna.sqlite3` | Database file location             |
| `ACOUSTIC_CONVERT_DIR` | `/tmp`                  | Temporary WAV conversion directory |

**Example:**

```bash
export ACOUSTIC_DB_PATH="$HOME/.acousticdna/database.sqlite3"
export ACOUSTIC_CONVERT_DIR="$HOME/.acousticdna/converted"
./acousticDNA add song.mp3 --title "Title" --artist "Artist"
```

---

## 🗺️ Roadmap

### Phase 1: Web Interface (In Progress)

-  [ ] REST API (Go + Chi/Gin framework)
   -  `POST /api/songs` - Add song
   -  `POST /api/match` - Match audio (multipart upload)
   -  `GET /api/songs` - List songs
   -  `DELETE /api/songs/:id` - Delete song
-  [ ] WebSocket support for real-time updates
-  [ ] React/Vue frontend
   -  Drag-and-drop audio upload
   -  Real-time matching progress
   -  Waveform visualization
   -  Match confidence charts

### Phase 2: WebAssembly (Planned)

-  [ ] Compile to WASM for browser-based fingerprinting
-  [ ] Client-side audio processing (no server upload)
-  [ ] Privacy-focused matching (only hashes sent to server)

### Phase 3: Enhanced Metadata (Planned)

-  [ ] **YouTube Integration** (`lrstanley/go-ytdlp`)
   -  Auto-fetch metadata (title, artist, thumbnail)
   -  Download audio from YouTube URL
   -  Batch indexing from playlists
-  [ ] **Spotify API Integration**
   -  Rich metadata (album art, genre, release date)
   -  Spotify track linking
   -  Playlist synchronization

### Phase 4: Advanced Features (Future)

-  [ ] Microphone input (real-time recording)
-  [ ] Partial match detection (identify from 3-5 seconds)
-  [ ] Noisy environment robustness (background filtering)
-  [ ] Multi-database support (PostgreSQL, MongoDB)
-  [ ] Distributed fingerprinting (horizontal scaling)
-  [ ] Mobile apps (iOS/Android via Go Mobile)

---

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

1. **Fork the repository**
2. **Create a feature branch** (`git checkout -b feature/amazing-feature`)
3. **Make your changes** and add tests
4. **Run tests** (`go test ./...`)
5. **Commit your changes** (`git commit -m 'Add amazing feature'`)
6. **Push to the branch** (`git push origin feature/amazing-feature`)
7. **Open a Pull Request**

### Development Guidelines

-  Follow Go conventions and use `gofmt`
-  Add tests for new features
-  Update documentation as needed
-  Keep commits atomic and descriptive

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

-  Inspired by the **Shazam algorithm** (Wang, 2003: _"An Industrial-Strength Audio Search Algorithm"_)
-  FFT implementation: [`mjibson/go-dsp`](https://github.com/mjibson/go-dsp)
-  SQLite driver: [`glebarez/sqlite`](https://github.com/glebarez/sqlite)
-  Audio libraries: [`go-audio/*`](https://github.com/go-audio)

---

## 👤 Author

**Himanish Puri**

-  GitHub: [@himanishpuri](https://github.com/himanishpuri)
-  Email: himanishpuri2203@gmail.com

---

## 📚 References

-  [Audio Fingerprinting Research Paper](https://hajim.rochester.edu/ece/sites/zduan/teaching/ece472/projects/2019/AudioFingerprinting.pdf)
-  [Acoustic Fingerprint - Wikipedia](https://en.wikipedia.org/wiki/Acoustic_fingerprint)
-  [STFT Tutorial - Stanford CCRMA](https://ccrma.stanford.edu/~jos/sasp/Short_Time_Fourier_Transform.html)
-  [Shazam's Original Patent](https://patents.google.com/patent/US6990453B2/)

---

<div align="center">

**⭐ Star this repo if you find it useful!**

Made with ❤️ by [Himanish Puri](https://github.com/himanishpuri)

</div>
