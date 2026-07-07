# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Shazam-style audio fingerprinting engine in Go. Same fingerprinting core runs three ways: a CLI, a REST server, and a browser WASM module. See `README.md` for algorithm theory and DSP parameters.

## Build & run

```bash
go build -o acousticDNA ./cmd/cli/      # CLI
go build -o server ./cmd/server/        # REST server
./scripts/build-wasm.sh                 # WASM -> web/public/fingerprint.wasm (+ copies wasm_exec.js)

go vet ./...                            # no linter configured; vet + gofmt is the bar
```

There are **no tests** in this repo (`go test ./...` finds nothing). If you add logic worth guarding, add a `_test.go` beside it.

Runtime binaries invoked via `os/exec` — must be on PATH: **ffmpeg**, **ffprobe**, **yt-dlp**. Missing binaries fail only at request time, not build time.

## Build-tag split (important)

The `fingerprint` and `models` packages have **no build tags** — they compile for both native and `js/wasm`. Everything else is gated:

- `//go:build !js && !wasm` → `cmd/cli`, `cmd/server`, `service.go`, `storage_adapter.go`, `storage/`. Native only (uses `os/exec`, gorm, sqlite).
- `//go:build js && wasm` → `cmd/wasm/main.go`. Browser only (uses `syscall/js`).

Consequence: **`fingerprint` must never import anything native.** Adding an `os/exec`, gorm, or filesystem dependency to `fingerprint` (or `models`) breaks the WASM build silently — you won't notice until `build-wasm.sh`. `spectrogram.go` importing `audio.ReadWavAsFloat64` is why `ComputeSpectrogramFromSamples` (samples-in, no file) exists as the WASM-safe entry point.

## The core invariant: hash layout is duplicated in 3 places

A fingerprint hash is a packed `uint32`: `[anchorFreq(9) | targetFreq(9) | deltaTime(14)]`. The client (WASM) generates hashes and the server matches them, so **both sides must agree on the bit layout.** If you change it, change all three together:

1. `pkg/acousticdna/fingerprint/hasher.go` — `createAddress` (the writer) + the `Max*Bits`/`*DeltaMs`/`FanOut` constants.
2. `pkg/models/api.go` — `IsValidHash` (server-side sanity check on incoming WASM hashes; hardcodes the same masks and the 10–15000ms delta range).
3. The matcher must use the same `FanOut` and delta window or query/db hashes won't line up.

Same goes for **sample rate** (default 11025 Hz): a song added at one rate won't match a query fingerprinted at another. WASM uses the browser's `AudioContext.sampleRate` (often 44100/48000), which is a known mismatch source vs. server-added songs.

## Request → match flow

Two entry points, **both do time-offset voting** (`offset = dbAnchorTime − queryAnchorTime`, then rank songs by the most-voted offset):

- **File path** (`MatchSong`, CLI + `/api/match`): decode → ffmpeg to mono 16-bit PCM → STFT → peaks → fingerprint → batch DB lookup → vote. Voting lives in `fingerprint.QueryFingerprints`.
- **Hash path** (`MatchHashes`, `/api/match/hashes`): client already sent hashes → batch DB lookup → vote. Voting is **reimplemented inline** in `service.go:MatchHashes` (not shared with `QueryFingerprints`). Keep the two voting implementations behaviourally identical when touching either.

`calculateConfidence` (service.go) is a hand-tuned sigmoid over match-count / min-fingerprint-count — magic constants (`steepness`, `midpoint`, the `>0.30` boost, the `<5` penalty) are intentional, not arbitrary.

## Layering & the adapter seam

`Service` and `Storage` are interfaces in `interfaces.go`. The concrete SQLite layer (`storage/sqlite.go`, `storage.DBClient`) is bridged to the `Storage` interface by `storage_adapter.go`. This means there are **two parallel type families** and the adapter converts between them field-by-field:

- `storage.Song` / `storage.Fingerprint` — gorm structs, DB tags, the actual tables.
- `models.Song` / `models.Couple` / `models.Match` — domain types crossing the interface.
- `models.*DTO` (api.go) — JSON wire shapes for HTTP.

When you add a field to a song, expect to touch all three families plus the two conversion sites. `types.go` is an empty stub kept for import compatibility — don't add types there; use `pkg/models`.

DB facts: song identity is the **(title, artist)** unique index; `RegisterSong` is idempotent on that pair (returns existing ID, backfills YouTubeID). `DeleteSongByID` cascades fingerprints in a transaction. `StoreFingerprints` bulk-inserts in batches of ~500/1000 — a fingerprint failure after `RegisterSong` triggers a compensating `DeleteSongByID` in the service.

## Gotchas

- **The 91 MB `acousticdna.sqlite3` is committed** and `.gitignore` only ignores `.env`. Don't regenerate/commit it casually; don't add large artifacts expecting them to be ignored.
- `audio.DownloadYouTubeAudio` hardcodes `--cookies-from-browser firefox`, `--js-runtimes deno`, and `--remote-components ejs:github`. This is tuned to one machine — it will fail without Firefox/Deno. Change here if YouTube adds break.
- Both `AddSong` and `MatchSong` read the WAV **twice** — once via `ReadWavAsFloat64` (for duration) and again inside `ComputeSpectrogram(wavPath,...)`. Be aware if optimizing the file path.
- `refrence_scripts/` (sic) are standalone `package main` examples, not wired into the build.
