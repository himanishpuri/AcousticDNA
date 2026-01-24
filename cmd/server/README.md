# AcousticDNA Server

A RESTful API server for audio fingerprinting and matching, optimized for privacy-preserving WASM clients.

## Architecture

The server is structured into modular components:

```
cmd/server/
├── main.go       # Application entry point and initialization
├── handlers.go   # HTTP request handlers
├── routes.go     # Route registration and CORS middleware
└── types.go      # Request/Response DTOs and validation
```

## Features

- **Hash-Based Matching**: Privacy-preserving endpoint for WASM clients to send only fingerprint hashes
- **Batch Hash Retrieval**: Single SQL query for matching thousands of hashes (eliminates N+1 query problem)
- **Dual Song Addition**: Support both local file uploads and YouTube URL downloads
- **CORS Support**: Configurable cross-origin resource sharing for browser clients
- **Smart Rate Limits**: Tiered hash limits with soft/hard thresholds
- **Hash Validation**: Lightweight validation to prevent malformed or malicious hashes

## Quick Start

### Build

```bash
go build -o bin/server ./cmd/server/
```

### Run

```bash
# Basic usage
./bin/server

# With custom configuration
./bin/server -port 8080 -db acousticdna.sqlite3 -rate 11025 -origins "*"
```

### Command-Line Flags

| Flag       | Default               | Description                              |
| ---------- | --------------------- | ---------------------------------------- |
| `-port`    | `8080`                | HTTP server port                         |
| `-db`      | `acousticdna.sqlite3` | Path to SQLite database                  |
| `-temp`    | `/tmp`                | Temporary directory for audio processing |
| `-rate`    | `11025`               | Audio sample rate (Hz)                   |
| `-origins` | `*`                   | Allowed CORS origins (comma-separated)   |

### Environment Variables

- `ACOUSTIC_DB_PATH`: Override default database path
- `ACOUSTIC_TEMP_DIR`: Override default temp directory

## API Endpoints

### Health & Metrics

#### GET /health

Health check endpoint

**Response:**

```json
{
	"status": "healthy",
	"time": "2026-01-24T12:00:00Z"
}
```

#### GET /api/health/metrics

Server and database metrics

**Response:**

```json
{
	"status": "healthy",
	"database_path": "acousticdna.sqlite3",
	"song_count": 42,
	"fingerprint_count": 125000,
	"sample_rate": 11025
}
```

### Song Management

#### GET /api/songs

List all songs in the database

**Response:**

```json
{
	"songs": [
		{
			"id": 1,
			"title": "Song Title",
			"artist": "Artist Name",
			"youtube_id": "dQw4w9WgXcQ",
			"duration_ms": 213000
		}
	],
	"count": 1
}
```

#### GET /api/songs/{id}

Get a specific song by ID

**Response:**

```json
{
	"id": 1,
	"title": "Song Title",
	"artist": "Artist Name",
	"youtube_id": "dQw4w9WgXcQ",
	"duration_ms": 213000
}
```

#### POST /api/songs

Add a song from local file upload

**Request:** `multipart/form-data`

- `audio`: Audio file (required)
- `title`: Song title (required)
- `artist`: Artist name (required)
- `youtube_id`: YouTube video ID (optional)

**Response:**

```json
{
	"message": "Song added successfully",
	"id": 1,
	"title": "Song Title",
	"artist": "Artist Name",
	"youtube_id": "dQw4w9WgXcQ"
}
```

**cURL Example:**

```bash
curl -X POST http://localhost:8080/api/songs \
  -F "audio=@song.mp3" \
  -F "title=Song Title" \
  -F "artist=Artist Name"
```

#### POST /api/songs/youtube

Add a song from YouTube URL

**Request:**

```json
{
	"youtube_url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
	"title": "Optional Title",
	"artist": "Optional Artist"
}
```

**Response:**

```json
{
	"message": "Song added successfully from YouTube",
	"id": 1,
	"title": "Song Title",
	"artist": "Artist Name",
	"youtube_id": "dQw4w9WgXcQ"
}
```

**Notes:**

- If `title` and `artist` are not provided, they will be extracted from YouTube metadata
- Downloads are synchronous with 5-minute timeout
- Most music videos (2-5 minutes) complete in 30-60 seconds

**cURL Example:**

```bash
curl -X POST http://localhost:8080/api/songs/youtube \
  -H "Content-Type: application/json" \
  -d '{
    "youtube_url": "https://youtube.com/watch?v=dQw4w9WgXcQ"
  }'
```

#### DELETE /api/songs/{id}

Delete a song and all its fingerprints

**Response:**

```json
{
	"message": "Song deleted successfully",
	"id": 1
}
```

### Audio Matching

#### POST /api/match

Match audio file (traditional approach)

**Request:** `multipart/form-data`

- `audio`: Audio file (required)

**Response:**

```json
{
	"matches": [
		{
			"song_id": 1,
			"title": "Song Title",
			"artist": "Artist Name",
			"youtube_id": "dQw4w9WgXcQ",
			"score": 425,
			"offset_ms": 1200,
			"confidence": 87.5
		}
	],
	"count": 1
}
```

**cURL Example:**

```bash
curl -X POST http://localhost:8080/api/match \
  -F "audio=@query.mp3"
```

#### POST /api/match/hashes

Match pre-computed hashes (WASM/privacy-preserving)

**Request:**

```json
{
	"hashes": {
		"123456": 1000,
		"789012": 1500,
		"345678": 2000
	}
}
```

Where keys are hash values (uint32) and values are anchor times in milliseconds.

**Response:**

```json
{
	"matches": [
		{
			"song_id": 1,
			"title": "Song Title",
			"artist": "Artist Name",
			"youtube_id": "dQw4w9WgXcQ",
			"score": 425,
			"offset_ms": 1200,
			"confidence": 87.5
		}
	],
	"count": 1
}
```

**Hash Limits:**

- **Soft Limit**: 10,000 hashes (~20-30 seconds of audio)
- **Hard Limit**: 50,000 hashes (~2 minutes of audio)
- **Warning Threshold**: 5,000 hashes (triggers logging)

**cURL Example:**

```bash
curl -X POST http://localhost:8080/api/match/hashes \
  -H "Content-Type: application/json" \
  -d '{
    "hashes": {
      "123456": 1000,
      "789012": 1500
    }
  }'
```

## Client Flow (WASM)

The hash-based matching endpoint is designed for privacy-preserving WASM clients:

```
┌─────────────────────────────────────────────────────────────┐
│                      BROWSER (Client)                        │
├─────────────────────────────────────────────────────────────┤
│  1. User uploads audio file (via Web UI)                    │
│  2. WASM processes audio → generates fingerprint HASHES     │
│  3. Send ONLY hashes to server (privacy!)                   │
│  4. Receive match results from server                       │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   │ POST /api/match/hashes
                   │ {"hashes": {123456: 1000, ...}}
                   ▼
┌─────────────────────────────────────────────────────────────┐
│                    BACKEND SERVER (Go)                       │
├─────────────────────────────────────────────────────────────┤
│  1. Validate hashes (format & count)                         │
│  2. Batch query SQLite (single SQL IN clause)               │
│  3. Perform time-coherence voting                            │
│  4. Return ranked results                                    │
└─────────────────────────────────────────────────────────────┘
```

**Privacy Benefits:**

- Audio never leaves the client
- Only cryptographic hashes are transmitted
- Server cannot reconstruct original audio from hashes
- Minimal data transfer (10,000 hashes ≈ 80KB)

## Hash Format

Hashes are 32-bit unsigned integers with packed components:

```
Bit Layout: [anchorFreq (9 bits) | targetFreq (9 bits) | deltaTime (14 bits)]
```

**Validation Rules:**

- `deltaTime`: 1-16,383 ms (must be > 0)
- `anchorFreq`, `targetFreq`: 0-511 (frequency bins)
- `anchorFreq` ≠ `targetFreq` (must be different)

**Example:**

```javascript
// JavaScript (WASM client)
const hash = (anchorFreq << 23) | (targetFreq << 14) | (deltaTime & 0x3fff);
```

## Performance

### Batch Hash Retrieval

The server uses a **single SQL query** to retrieve all hash matches:

```sql
SELECT hash, song_id, anchor_time_ms
FROM fingerprints
WHERE hash IN (?, ?, ?, ...)
```

**Performance Comparison:**

- **Old (N queries)**: 10,000 hashes × 2ms = 20 seconds
- **New (1 query)**: 10,000 hashes = 50-200ms

**10-100x performance improvement** for typical queries!

### Timeouts

- Hash matching: 30 seconds
- File matching: 2 minutes
- Song addition (file): 5 minutes
- Song addition (YouTube): 5 minutes

## Error Handling

All errors return standard JSON format:

```json
{
	"error": "Bad Request",
	"message": "too many hashes: 60000 (maximum: 50000)",
	"code": 400
}
```

**Common HTTP Status Codes:**

- `200 OK`: Successful operation
- `201 Created`: Resource created successfully
- `400 Bad Request`: Invalid input or validation error
- `404 Not Found`: Resource not found
- `405 Method Not Allowed`: HTTP method not supported
- `500 Internal Server Error`: Server-side error

## CORS Configuration

The server supports CORS for browser-based WASM clients:

```bash
# Allow all origins (development)
./bin/server -origins "*"

# Allow specific origins (production)
./bin/server -origins "https://app.example.com,https://admin.example.com"
```

**CORS Headers:**

- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods`: GET, POST, PUT, DELETE, OPTIONS
- `Access-Control-Allow-Headers`: Content-Type, Authorization, X-Requested-With
- `Access-Control-Max-Age`: 3600

## Database Schema

### Songs Table

```sql
CREATE TABLE songs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title       TEXT NOT NULL,
  artist      TEXT NOT NULL,
  youtube_id  TEXT,
  spotify_id  TEXT,
  duration_ms INTEGER,
  created_at  TIMESTAMP,
  UNIQUE(title, artist)
);

CREATE INDEX idx_song_meta ON songs(title, artist);
CREATE INDEX idx_youtube_id ON songs(youtube_id);
```

### Fingerprints Table

```sql
CREATE TABLE fingerprints (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  hash           INTEGER NOT NULL,
  song_id        INTEGER NOT NULL,
  anchor_time_ms INTEGER NOT NULL,
  FOREIGN KEY(song_id) REFERENCES songs(id)
);

CREATE INDEX idx_hash ON fingerprints(hash);
CREATE INDEX idx_song ON fingerprints(song_id);
```

## Examples

### Add Song from YouTube

```bash
# Auto-detect metadata
curl -X POST http://localhost:8080/api/songs/youtube \
  -H "Content-Type: application/json" \
  -d '{"youtube_url": "https://youtube.com/watch?v=dQw4w9WgXcQ"}'

# Custom metadata
curl -X POST http://localhost:8080/api/songs/youtube \
  -H "Content-Type: application/json" \
  -d '{
    "youtube_url": "https://youtube.com/watch?v=dQw4w9WgXcQ",
    "title": "Never Gonna Give You Up",
    "artist": "Rick Astley"
  }'
```

### Match Audio (Hash-Based)

```bash
# Typical WASM client request
curl -X POST http://localhost:8080/api/match/hashes \
  -H "Content-Type: application/json" \
  -d @hashes.json
```

Where `hashes.json`:

```json
{
	"hashes": {
		"8388608": 0,
		"8388864": 256,
		"8389120": 512,
		"8389376": 768
	}
}
```

### List All Songs

```bash
curl http://localhost:8080/api/songs
```

### Delete Song

```bash
curl -X DELETE http://localhost:8080/api/songs/1
```

## Development

### Testing

```bash
# Run server with debug logging
./bin/server -db test.sqlite3

# Test health endpoint
curl http://localhost:8080/health

# Test metrics
curl http://localhost:8080/api/health/metrics
```

### Building for Production

```bash
# Build with optimizations
go build -ldflags="-s -w" -o bin/server ./cmd/server/

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o bin/server-linux ./cmd/server/
```

## Troubleshooting

### "Too many hashes" Error

If you receive a 400 error about exceeding hash limits:

- **Soft limit (10,000)**: Typical for 20-30 seconds of audio
- **Hard limit (50,000)**: Absolute maximum for ~2 minutes
- Consider splitting long audio into chunks

### YouTube Download Fails

- Ensure `yt-dlp` is installed and in PATH
- Check YouTube URL format
- Verify network connectivity
- Check server logs for detailed error messages

### CORS Errors in Browser

- Set `-origins` flag to include your frontend URL
- Check browser console for specific CORS error
- Ensure preflight OPTIONS requests are handled

### Database Locked

- Only one writer at a time (SQLite limitation)
- Increase connection pool if needed
- Consider PostgreSQL for high-concurrency scenarios

## License

See root LICENSE file.
