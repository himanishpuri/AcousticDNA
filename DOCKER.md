# 🐳 Docker Deployment Guide

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Build and start the container
docker-compose up -d

# View logs
docker-compose logs -f

# Stop the container
docker-compose down
```

### Using the Build Script

```bash
# Build the Docker image
./docker-build.sh

# Run with docker-compose
docker-compose up -d
```

### Manual Docker Build

```bash
# Build the image
docker build -t acousticdna:latest .

# Run the container
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/temp:/app/temp \
  --name acousticdna \
  acousticdna:latest
```

## Configuration

### Environment Variables

- `ACOUSTIC_DB_PATH` - Database file path (default: `/app/data/acousticdna.sqlite3`)
- `ACOUSTIC_TEMP_DIR` - Temporary files directory (default: `/app/temp`)
- `PORT` - Server port (default: `8080`)

### Volumes

- `/app/data` - Persistent database storage
- `/app/temp` - Temporary audio processing files

## Usage

### Access the Web Interface

Once running, open your browser to: **`http://localhost:8080`**

The web interface provides:

- Audio file upload and fingerprinting
- Real-time matching
- WASM-based client-side processing

### Access the API

API endpoints available at: `http://localhost:8080/api/`

```bash
# Check health
curl http://localhost:8080/health

# List songs
curl http://localhost:8080/api/songs

# Add song via YouTube
curl -X POST http://localhost:8080/api/songs/youtube \
  -H "Content-Type: application/json" \
  -d '{"url": "https://youtube.com/watch?v=VIDEO_ID", "title": "Song", "artist": "Artist"}'
```

### Using the CLI

Use the included wrapper script:

```bash
# Make sure container is running first
docker compose up -d

# List songs
./acousticdna list

# Add a song from local file
./acousticdna add /path/to/song.mp3 --title "Song Title" --artist "Artist Name"

# Match audio
./acousticdna match /path/to/clip.wav

# Add from YouTube
./acousticdna youtube "https://youtube.com/watch?v=VIDEO_ID" --title "Song" --artist "Artist"
```

**Note:** When using file paths with the CLI wrapper, files must be accessible inside the container. Use volume mounts or copy files first:

```bash
# Option 1: Copy file to container
docker cp song.mp3 acousticdna-server:/tmp/song.mp3
./acousticdna add /tmp/song.mp3 --title "My Song" --artist "My Artist"

# Option 2: Use mounted volumes (if configured)
# Add volume mount to docker-compose.yml:
#   - ./music:/app/music
# Then:
./acousticdna add /app/music/song.mp3 --title "My Song" --artist "My Artist"
```

## Development Mode

For development with live code changes:

```bash
# Use the dev compose file
docker-compose -f docker-compose.dev.yml up -d

# This mounts the web directory for live updates
```

## Troubleshooting

### Check Container Logs

```bash
docker-compose logs -f acousticdna
```

### Check Container Status

```bash
docker-compose ps
```

### Rebuild After Code Changes

```bash
docker-compose down
docker-compose build --no-cache
docker-compose up -d
```

### Reset Database

```bash
# Stop container
docker-compose down

# Remove database
rm -rf data/acousticdna.sqlite3

# Restart
docker-compose up -d
```

## Production Deployment

For production, consider:

1. Using a reverse proxy (nginx/traefik)
2. Setting up proper CORS origins:

   ```bash
   docker run -d \
     -p 8080:8080 \
     -e ACOUSTIC_DB_PATH=/app/data/acousticdna.sqlite3 \
     -v $(pwd)/data:/app/data \
     acousticdna:latest \
     ./acousticdna-server -origins "https://yourdomain.com"
   ```

3. Using Docker secrets for sensitive configuration
4. Setting up health checks and monitoring
5. Configuring log rotation

## Image Details

**Base Image:** Alpine Linux  
**Size:** ~150MB (optimized multi-stage build)  
**Included:**

- FFmpeg (audio processing)
- yt-dlp (YouTube downloads)
- AcousticDNA server & CLI

**Exposed Ports:** 8080  
**Volumes:** `/app/data`, `/app/temp`
