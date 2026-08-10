# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

AcousticDNA — Shazam-style audio fingerprinting in Go. STFT spectrogram → constellation peaks → combinatorial 32-bit hashes → SQLite storage → time-offset voting for matches. Three entry points share the same fingerprinting core: CLI (`cmd/cli`), REST server (`cmd/server`), and a WASM build (`cmd/wasm`) that runs fingerprint generation client-side in the browser so raw audio never leaves it.

## Build tags — the thing that will bite you

`pkg/acousticdna/service.go`, `storage_adapter.go`, `storage/sqlite.go`, `cmd/cli`, `cmd/server` are all `//go:build !js && !wasm`. `cmd/wasm/main.go` is `//go:build js && wasm`. The two trees never compile together. If you touch `pkg/acousticdna/fingerprint/*` (no build tag — shared by both), verify it still builds for both targets:

```bash
go build ./...                              # native (server/cli)
GOOS=js GOARCH=wasm go build ./cmd/wasm/     # browser
```

`refrence_scripts/*.go` are `//go:build ignore` — standalone examples, not part of any build.

## Commands

```bash
# Build
go build -o acousticDNA ./cmd/cli/
go build -o server ./cmd/server/
GOOS=js GOARCH=wasm go build -o web/public/fingerprint.wasm ./cmd/wasm/   # or: scripts/build-wasm.sh

# Test
go test ./...
go test ./pkg/acousticdna/fingerprint/... -run TestName -v   # single test
go test ./pkg/acousticdna -run TestPerfClaims -v             # perf numbers against demo DB (test/testCroppedAudio); skips if fixtures absent

# Run
./server -port 8080 -db acousticdna.sqlite3 -temp /tmp -rate 11025 -origins "*"
./acousticDNA add song.mp3 --title "..." --artist "..."
./acousticDNA match recording.wav
```

Requires FFmpeg/ffprobe and yt-dlp on PATH for anything touching `pkg/acousticdna/audio` (native path only — WASM build has no audio I/O, it takes raw sample arrays from JS).

## Architecture

**Service layer** (`pkg/acousticdna/service.go`): `acousticService` implements the `Service` interface (`interfaces.go`) — `AddSong`, `MatchSong` (from audio file), `MatchHashes` (from pre-computed hashes, used by the WASM path), plus song CRUD. Constructed via `NewService(opts ...Option)` (functional options in `config.go`); defaults to SQLite storage at `DBPath` unless `WithStorage` injects one (tests use this to swap in a fake).

**Storage** (`pkg/acousticdna/storage/sqlite.go` + `storage_adapter.go`): `storage.DBClient` wraps gorm over `glebarez/sqlite` (pure-Go, CGO-free — this is a load-bearing invariant, see Dockerfile comments; don't swap in `mattn/go-sqlite3`). `storageAdapter` bridges `DBClient`'s gorm-flavored methods to the package's `Storage` interface, translating between `storage.Song`/`storage.Fingerprint` (gorm models) and `models.Song`/`models.Couple` (domain types). Two tables: `Song` and `Fingerprint` (hash → songID, anchorTimeMs, indexed on `hash`). Fingerprint lookups always do one batched `hash IN (...)` query, never per-hash round trips.

**Fingerprinting pipeline** (`pkg/acousticdna/fingerprint/`), shared unmodified by native and WASM builds:
1. `spectrogram.go` — STFT (1024-sample window, 256-sample hop, Hamming) over mono 11,025 Hz PCM
2. `peaks.go` — extract constellation points (local maxima above threshold)
3. `hasher.go` — pair anchor+target peaks into 32-bit hashes: `[anchorFreq(9) | targetFreq(9) | deltaTime(14)]`
4. `generator.go` — orchestrates the above (`Fingerprint()`) and does the time-offset voting for matches (`QueryFingerprints()`)

**Matching**: for each query hash, look up DB couples with the same hash, compute `offset = db_anchor_time - query_anchor_time`, vote `votes[songID][offset]++`. Best-count offset per song wins. `service.go`'s `MatchHashes` reimplements this voting loop directly (rather than calling `fingerprint.QueryFingerprints`, which expects peaks) since the WASM client sends `hash -> anchorTime` pairs, not peaks — keep the two voting implementations in sync if you change the algorithm. Confidence scoring (`calculateConfidence`) blends peakiness (winning offset bin vs. runner-up) and margin (this song's score vs. the best competing song), with a quadratic floor damping low-vote spurious hits — see the doc comment on `calculateConfidence` for the reasoning if you touch it.

**Two client paths hit the same server API**: browser WASM computes hashes locally and POSTs to `/api/match/hashes` (bandwidth: hashes only, ~98% smaller than raw audio); direct upload POSTs the audio file to `/api/match` and the server fingerprints it. CLI talks to the DB directly, bypassing HTTP entirely.

**HTTP layer** (`cmd/server/`): stdlib `net/http` + `ServeMux`, no framework. `routes.go` wires CORS + logging middleware around handlers in `handlers.go`. Static frontend (`web/public/`) is served at `/`.

## Conventions

- Errors are wrapped with `fmt.Errorf("...: %w", err)` up through every layer — preserve this when adding calls.
- Logging goes through the `Logger` interface (`pkg/logger`), injected via `WithLogger`, not raw `log`/`fmt.Println` (server's middleware is the one exception, using `logger.GetLogger()` directly).
- `acousticdna.sqlite3` is tracked via Git LFS (`.gitattributes`); the compose setup seeds fresh volumes from it on first boot (`docker-entrypoint.sh`).
