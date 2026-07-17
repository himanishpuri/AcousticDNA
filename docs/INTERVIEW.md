# AcousticDNA — Deep Dive (Interview Prep)

A from-scratch, Shazam-style audio fingerprinting system in Go. This doc explains
**how it works end to end**, the **algorithm**, the **design decisions**, and the
**nuances / trade-offs** most likely to come up under questioning. Every number and
constant below is pulled from the actual code (paths cited inline).

---

## 1. One-line pitch

> Given a noisy 5–15 second audio clip, identify the song by turning both the
> database tracks and the query clip into **constellations of time–frequency peaks**,
> hashing **pairs of peaks** into compact 32-bit integers, and finding the song whose
> hashes align at a **single consistent time offset**.

It's the Wang/Shazam approach (2003 paper), reimplemented: STFT → spectral peaks →
combinatorial pair hashing → time-coherent offset voting.

---

## 2. Why fingerprinting at all (the core intuition)

Raw audio can't be compared directly — the query is a short, noisy, possibly
volume-shifted, phone-mic recording of the original. So we throw away almost
everything and keep only what survives noise and compression: **the loudest
frequency peaks and their positions in time**.

Two properties make this work:

1. **Peaks are robust.** The strongest spectral points survive background noise,
   MP3 compression, and cheap microphones. Everything quiet gets discarded.
2. **Pairs of peaks encode structure invariant to _when_ the clip starts.** A hash
   built from `(freq_anchor, freq_target, Δtime)` is the same whether the clip is
   taken from 0:03 or 1:47 of the song — only the _absolute_ time differs, and that
   difference is exactly the signal we exploit in voting.

---

## 3. The pipeline, stage by stage

```
audio file ──ffmpeg──▶ mono WAV @ target rate
   │
   ▼
STFT (Hamming window)  ──▶  magnitude spectrogram  [frames × freq-bins]
   │
   ▼
peak extraction (per-band local maxima)  ──▶  []Peak  (a "constellation")
   │
   ▼
combinatorial pairing (anchor × fan-out targets)  ──▶  32-bit hash + anchor time
   │
   ├── ADD:   store {hash → (songID, anchorTimeMs)} rows in SQLite
   └── MATCH: look up query hashes, vote on time offsets, score
```

### 3.1 Audio → mono WAV (`pkg/acousticdna/audio`)

FFmpeg decodes any container (MP3/FLAC/AAC/M4A/OGG/WAV) and downmixes to **mono** at a
fixed sample rate. Mono because we only care about spectral content, not stereo
image; fixed rate so frequency-bin math is identical for DB and query.

### 3.2 STFT — Short-Time Fourier Transform (`fingerprint/spectrogram.go`)

- **Window size 1024, hop 256** (`WindowSize`, `HopSize`). 75% overlap between frames.
- Each frame is multiplied by a **Hamming window** (`0.54 − 0.46·cos(2πn/(N−1))`)
  before the FFT — reduces spectral leakage from the hard frame boundary.
- FFT via `mjibson/go-dsp`; we keep the **magnitude** of the first `N/2` bins
  (`MagnitudeSpectrum` — the spectrum is symmetric for real input, so the top half is
  redundant).
- Output: `[][]float64` = spectrogram, indexed `[frame][freqBin]`.

**Nuance — resolution trade-off:** window 1024 gives frequency resolution
`sampleRate/1024` Hz per bin and time resolution `256/sampleRate` s per frame. Bigger
window = sharper frequency, blurrier time (Heisenberg-style trade-off). 1024/256 is the
classic Shazam-ish sweet spot for music.

### 3.3 Peak extraction (`fingerprint/peaks.go`)

This is where most of the "cleverness" lives. Naive "pick the N loudest bins" biases
toward bass and loud sections. Instead:

- **Logarithmic frequency bands.** Bins split into bands `[0,10), [10,20), [20,40),
[40,80), …` (each band doubles). This mirrors human hearing (log-frequency) and
  guarantees high frequencies get represented even though they carry less energy.
- **One candidate per band per frame** — the strongest bin in each band.
- **Adaptive threshold.** A candidate is kept only if its magnitude (in dB) is at least
  `minDbAboveAvg = 3.0` dB above the average of that frame's per-band maxima. So a peak
  must stand out _relative to the current frame_, not clear a global cutoff — this makes
  it robust to quiet passages and overall volume.
- **Local-maximum check.** The candidate must be the max within a `±3 freq × ±1 time`
  neighborhood. Kills clusters where one real peak would otherwise spawn several
  near-duplicates.
- Result: a sparse set of `Peak{TimeIdx, FreqIdx, Time, Freq, MagDB}`, sorted by time.
  This is the **constellation map**.

### 3.4 Combinatorial hashing (`fingerprint/hasher.go`, `generator.go`)

A single peak is not distinctive (too many songs share a 440 Hz peak). A **pair** is.
For each anchor peak, pair it with the next `FanOut = 6` peaks in time:

```
address (32-bit) = anchorFreqIdx[9 bits] | targetFreqIdx[9 bits] | Δtime_ms[14 bits]
                   << 23                    << 14                    << 0
```

Constants (`hasher.go`):

- `MaxFreqBits = 9` → freq index 0–511 fits (spectrogram half-width ≤ 512 bins for 1024 window).
- `MaxDeltaBits = 14` → Δtime up to 16383 ms fits.
- `MinDeltaMs = 10`, `MaxDeltaMs = 15000` → pairs closer than 10 ms or farther than 15 s
  are rejected (too-close = redundant/noisy; too-far = likely spans unrelated sections).
- Pairs whose freq index or Δ exceeds the mask are rejected (`return 0, false`).

Each stored hash carries the **anchor's absolute time** (`AnchorTimeMs`). The `models.Couple`
= `{SongID, AnchorTimeMs}` is the value stored under each hash key.

**Why pairs instead of single peaks?** Specificity. `FanOut = 6` means each anchor
generates up to 6 hashes → the database is ~6× the peak count, but each hash is far more
discriminating, so a handful of matching hashes already pins down the song. It's the
speed/specificity knob.

### 3.5 Storage (`storage/sqlite.go`)

- Two tables: `Song{ID(uuid), Title, Artist, YouTubeID, DurationMs}` and
  `Fingerprint{Hash, SongID, AnchorTimeMs}` with an **index on `Hash`** (`idx_hash`) —
  the entire match hot path is `WHERE hash IN (...)`, so that index is load-bearing.
- Pure-Go SQLite driver (`glebarez/sqlite`) → **no CGO** → the binary is fully static
  (this is what lets it ship in a tiny scratch Docker image and cross-compile).
- Inserts are **batched** (`CreateInBatches`, 500/batch) — a 3-minute song produces
  tens of thousands of fingerprint rows; row-by-row inserts would be pathologically slow.
- Unique index `(Title, Artist)` + idempotent `RegisterSong` — re-adding the same song
  returns the existing ID instead of duplicating.

### 3.6 Matching — the time-coherence vote (`service.go`, `generator.go`)

This is the heart of the algorithm and the most-asked part.

1. Fingerprint the **query** the same way → set of query hashes, each with its own
   `queryAnchorTime`.
2. **One batched SQL query**: `GetCouplesByHashes(hashList)` → `WHERE hash IN (...)`
   pulls every DB row matching any query hash in a single round trip, vs. one SELECT
   per hash. Measured ~3× faster locally on the 822K-row demo DB; the gap widens with
   real per-query network latency and larger catalogs. (Do **not** claim a fixed
   multiplier in an interview — it depends entirely on the baseline.)
3. For every (queryHash, dbRow) collision, compute
   **`offset = dbAnchorTimeMs − queryAnchorTimeMs`** and tally votes per song:
   `votes[songID][offset]++`.
4. For each song, find the **offset bin with the most votes** (`bestCount`) and its
   time offset. Also track the **second-tallest bin** (`SecondBestCount`).
5. Rank songs by `bestCount` (the "score").

**The key insight:** for the _correct_ song, hashes match because the query really is a
slice of that track, so _all_ matching hashes share the **same** `dbTime − queryTime`
offset (the position of the clip within the song). They pile into **one offset bin** →
sharp spike. For a _wrong_ song, coincidental hash collisions scatter across random
offsets → flat histogram, no spike. **A dominant offset bin is the match; a flat
distribution is noise.** This is why we vote on offsets rather than just counting shared
hashes — a bag-of-hashes count is fooled by common hashes; time-coherence is not.

**Invariant to be aware of:** there are two independent implementations of this vote —
`QueryFingerprints` (native, `generator.go`) and the inline loop in `MatchHashes`
(`service.go`, used by the WASM path). They must stay behaviorally identical; both track
the second-best bin the same way. Tests lock this.

---

## 4. Confidence scoring (`service.go: calculateConfidence`)

This is a good "tell me about a bug you fixed" story.

**The bug:** confidence was `matchCount / totalQueryHashes` fed through a logistic.
But aligned votes are _always_ a tiny fraction of total hashes (a 15 s clip has
thousands of hashes; even a perfect match aligns ~20). So _every_ match — true or
false — collapsed to the sigmoid floor (~5%). A correct Benson Boone match and pure
noise both read ~5%. The confidence number was useless.

**The fix:** score from the two signals that _actually_ separate true from false, both
length-independent:

- **Peakiness** = `1 − secondBestOffsetCount / bestCount`. A real match spikes at one
  offset (bestCount ≫ second) → ~1. Noise scatters → ~0.
- **Cross-song margin** = `(bestCount − competingSongScore) / bestCount`. A confident hit
  beats the runner-up song; ambiguous noise ties with it.

Blend `0.5·peakiness + 0.5·margin`, spread through a logistic (steepness 9, midpoint
0.35), then apply an **absolute-reliability floor**: if `bestCount < 8`, multiply by
`(bestCount/8)²` — a 2–3-vote fluke can't read high no matter how "peaked" it looks.

Result (verified live): genuine clip ~69%, noise ~2%. Ranking is unchanged (still by
`bestCount`) — only the _displayed_ confidence changed, so it's a low-risk fix.

---

## 5. WebAssembly path — the "privacy" + "bandwidth" story

The differentiator on the resume. Two ways to match from the browser:

- **Server-side (default):** browser uploads the audio file; server does the whole
  pipeline.
- **WASM (privacy mode):** the _same Go fingerprinting code_ compiles to
  `GOOS=js GOARCH=wasm` and runs **in the browser**. The browser does STFT + peaks +
  hashing locally and POSTs only the **hash map** to `/api/match/hashes`
  (`web/public/wasm.js`). Server-side `MatchHashes` runs the vote and returns matches.

Why this is a real win, not a gimmick:

- **Bandwidth (~98% reduction, measured):** a real 15 s WAV is ~3.7 MB; the hash map is
  ~61 KB of integers (measured on the test clip, 3,795 hashes). You ship fingerprints,
  not audio. Scales higher for longer/lossless sources.
- **Privacy:** the raw recording never leaves the device — only opaque hashes do.
- **Code reuse:** the DSP is compiled once, run in two environments. This forces a
  **build-tag discipline** — `pkg/acousticdna/fingerprint` and `pkg/models` must compile
  for _both_ native and `js/wasm`, so they contain **no native-only imports** (no SQLite,
  no ffmpeg, no os). Storage/audio are `//go:build !js && !wasm` gated. This is why the
  package split looks the way it does — it's not arbitrary, it's the WASM boundary.

---

## 6. Interfaces (`cmd/`)

- **CLI** (`cmd/cli`): `add` (file or `--youtube-url`, auto-download via yt-dlp + metadata),
  `match`, `list`, `delete`.
- **REST** (`cmd/server`): `POST /api/songs`, `POST /api/match` (file upload),
  `POST /api/match/hashes` (WASM path), `GET /api/songs`, `GET /api/health/metrics`.
- **WASM** (`cmd/wasm`): the fingerprinting compiled to `fingerprint.wasm`, driven by
  `web/public/wasm.js`.

All three call the **same** `acousticdna.Service` — the interfaces are thin; the logic
lives in one place.

---

## 7. Likely interview questions — crisp answers

**Q: How is it robust to noise / volume / a bad mic?**
Peaks are the loudest points and survive noise; the dB threshold is _relative to the
frame's own average_, so overall volume doesn't matter; only spectral _structure_ is
kept. Time-coherence voting means a few surviving peaks still align.

**Q: Why hash _pairs_ of peaks, not single peaks?**
Specificity. A single peak is shared by countless songs. A `(f1, f2, Δt)` pair is rare
enough that a small number of aligned pairs uniquely identifies a track.

**Q: Why does the _time offset_ voting work — why not just count matching hashes?**
A pure count is fooled by common hashes appearing in many songs. The correct song has
all its matches at _one_ consistent offset (the clip's position in the track), producing
a spike; false matches scatter across offsets. The spike is the discriminator.

**Q: Time complexity of a match?**
Query fingerprinting is O(peaks · fanOut). Retrieval is one indexed `hash IN (...)`
query. Voting is O(total collisions). No linear scan over songs — the hash index does
the pruning.

**Q: Why 1024/256 window/hop? Why FanOut 6? Why 32-bit hash?**
Window/hop = frequency-vs-time resolution trade-off tuned for music. FanOut trades DB
size for match specificity. 32 bits packs 9+9+14 (two freq indices + Δtime) into one
`uint32` — compact, index-friendly, and enough entropy to be discriminating.

**Q: Biggest limitation / what would you improve?**

- No pitch/tempo invariance — a cover or a sped-up version won't match (this is inherent
  to the exact-hash approach; Shazam has the same limitation).
- `hash IN (...)` with thousands of query hashes can get large; a hash-partitioned or
  in-memory inverted index would scale better than SQLite for a big catalog.
- Peak params are hand-tuned constants — a real system would calibrate them per corpus.

**Q: Why Go? Why no CGO?**
Pure-Go SQLite → static binary → trivial containerization and the _same_ code cross-
compiles to WASM. That dual-target reuse is the architectural payoff.

---

## 8. File map (where to point during a walkthrough)

| Concern                                        | File                                         |
| ---------------------------------------------- | -------------------------------------------- |
| STFT + Hamming window                          | `pkg/acousticdna/fingerprint/spectrogram.go` |
| Peak extraction (bands, thresholds, local-max) | `pkg/acousticdna/fingerprint/peaks.go`       |
| 32-bit pair hash + bit layout                  | `pkg/acousticdna/fingerprint/hasher.go`      |
| Pairing + offset voting                        | `pkg/acousticdna/fingerprint/generator.go`   |
| Orchestration, WASM vote, confidence           | `pkg/acousticdna/service.go`                 |
| Schema, batched insert, batched lookup         | `pkg/acousticdna/storage/sqlite.go`          |
| CLI / REST / WASM entry points                 | `cmd/cli`, `cmd/server`, `cmd/wasm`          |
| Browser-side WASM glue                         | `web/public/wasm.js`                         |

---

## 9. Deeper probes (the follow-up questions)

These are the second-layer questions a strong interviewer asks to test _depth_. Every
answer below is grounded in the actual code.

### 9.1 Complexity — what's the Big-O of a match?

- **Fingerprint the query:** `O(P·F)` — P peaks × FanOut 6 pair attempts.
- **Retrieval:** one indexed `hash IN (…)` → roughly `O(H·log N)` on the B-tree
  (`idx_hash`), H = distinct query hashes (~3.8K for a 15 s clip).
- **Voting:** `O(C)` — C = total (queryHash, dbRow) collisions retrieved.
- **Ranking:** `O(M log M)`, M = candidate songs (tiny).
- **No `O(#songs)` scan** — the hash index prunes; the DB never iterates songs. That's
  the whole point of the inverted-hash design.

### 9.2 Why 11,025 Hz specifically? (`cmd/cli` `defaultSampleRate = 11025`)

Nyquist: the max representable frequency is `sampleRate/2 ≈ 5.5 kHz`. Nearly all musical
_fundamentals and lower harmonics_ — the part that identifies a song — live below ~5 kHz.
So 11,025 Hz keeps the discriminative content while being **4× smaller than CD (44.1 kHz)**:
less compute in the FFT, fewer samples, smaller DB. Higher rates waste work on
high-frequency detail that noise/compression destroys anyway; lower rates start eating
into musical structure. It's a deliberate accuracy/cost trade. (Both add and query use the
same rate — non-negotiable, or the frequency-bin math wouldn't line up.)

### 9.3 Hash collisions & false positives — aren't 32 bits too few?

Collisions are **expected and harmless by design**. Two unrelated peak-pairs hashing to the
same 32-bit value just append two `Couple{SongID, AnchorTimeMs}` entries to that bucket
(`sqlite.go StoreFingerprints`). At match time _all_ couples for a hash vote. A spurious
collision lands at a **random** time offset; the true song's collisions all land at the
**same** offset. So false hits scatter across the offset histogram and the real match spikes
in one bin — **time-coherence voting is the collision filter**. This is why we never rely on
raw hash-match _count_; we rely on offset _concentration_ (peakiness) — see §4.

### 9.4 How do peaks survive MP3 / a phone mic?

Lossy codecs (MP3/AAC) use _perceptual_ coding: they discard what humans can't hear —
masked and quiet components — and preserve the perceptually dominant spectral energy. The
**loudest bin per band** (exactly what `ExtractPeaks` keeps) is precisely what survives.
Noise and cheap mics corrupt _weak_ components; the peaks stay. And the threshold is
**relative to each frame's own average** (`minDbAboveAvg = 3.0` dB), so overall volume /
gain shifts don't move which peaks get picked.

### 9.5 Scale & memory — how big can this go?

Rough per-song cost: `frames × peaks/frame × FanOut` rows. A 3-min track ≈ tens of
thousands of `Fingerprint` rows (the live demo DB: **22 songs → ~822K rows → 91 MB**, so
~37K rows/song average). That extrapolates to ~**GBs at 10K+ songs** — SQLite handles it but
slows, and `hash IN (…)` has a practical size ceiling. **Where I'd take it next:** an
in-memory inverted index `hash → []SongID` (Redis) or a sharded store, to drop the per-query
DB round trip. Query-side memory is trivial (~tens of KB of hashes).

### 9.6 Why SQLite (not Redis / a real inverted index)?

Deliberate for this scope: **zero-config, embeddable, single-file, pure-Go driver
(`glebarez/sqlite`) → no CGO → static binary + WASM code reuse.** For a demo/small catalog
it's plenty and it keeps deployment trivial (drop the `.sqlite3` file in the container). The
honest limitation: it's a _row store queried by an index_, not a purpose-built inverted
index — hence §9.5. Knowing _when_ the current choice breaks is the point.

### 9.7 The two-implementation invariant (the subtle risk)

The offset vote exists **twice**: `QueryFingerprints` (native, `generator.go`) and the inline
loop in `MatchHashes` (`service.go`, the WASM entry point). They must produce identical
`Match{Count, SecondBestCount, OffsetMs}`. Today they're kept in lockstep by hand + tests;
**the clean fix is to extract one shared vote function** — call that out proactively as known
tech debt, it reads as maturity.

### 9.8 Concurrency

Per-request work (STFT → peaks → hash → vote) is sequential and CPU-bound. Concurrency is at
the **server** layer: the SQLite pool caps at `SetMaxOpenConns(25)` (`sqlite.go`), so ~25
requests run in parallel; graceful shutdown uses a background goroutine (`routes.go`).
Obvious parallelism _not yet exploited_ (and a good "what would you optimize" answer): STFT
frames are independent → parallelizable; the query-hash set could be chunked across workers.

### 9.9 FFmpeg / yt-dlp dependency & failure modes

Both are shelled out via `exec.CommandContext` (`audio/processor.go`) — so they inherit the
request's context/timeout. Failure modes: binary missing from PATH, unsupported codec, or a
geo-blocked/unavailable video → non-zero exit, surfaced as a wrapped error (`AddSong`/
`MatchSong` fail loudly, no silent fallback). yt-dlp path: `-J` for metadata JSON first, then
download; title/artist fall back to YouTube metadata when not supplied.

### 9.10 AddSong atomicity

If fingerprint storage fails after the song row is created, `AddSong` **rolls back** by
deleting the song (`service.go` — `DeleteSongByID` on store error), so a half-added song
can't linger. `RegisterSong` is **idempotent** on `(title, artist)` — re-adding returns the
existing ID instead of duplicating.

### 9.11 Testing strategy

Real tests in the repo: `fingerprint/generator_test.go` (hashing + voting), `service_test.go`
(the confidence model — locks exact values, monotonicity, thresholds), `audio/processor_test.go`
(yt-dlp arg building), `cmd/server/routes_test.go` (endpoints), and `perf_test.go` (the
measured bandwidth + batched-lookup benchmark, skips without the real DB/ffmpeg). **What I'd
add:** adversarial robustness (noise injection, near-duplicate songs) and a native-vs-WASM
equivalence test to enforce §9.7.

### 9.12 Honest limitations (say these before you're asked)

- **No pitch/tempo invariance** — a cover, a live version, or a sped-up upload won't match.
  Inherent to exact-hash matching (Shazam has the same limit). Fixing it needs a fundamentally
  different feature (e.g. chroma/CQT).
- **Params are hand-tuned constants**, not learned per corpus.
- **SQLite ceiling** at large catalogs (§9.5–9.6).
- **Confidence is a heuristic** (peakiness + margin + a quadratic low-count floor), not a
  calibrated probability. It separates true from noise well; it isn't a real posterior.
