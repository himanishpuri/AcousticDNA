# AcousticDNA, Explained Simply

This doc is the plain-English version. `README.md` has setup/usage, `docs/INTERVIEW.md`
has deep technical Q&A. This one is for understanding **what the project actually does**
and **how the pieces fit together**, without needing DSP background.

---

## 1. What is this, in one sentence?

You give it a short, noisy clip of a song. It tells you which song it is — like Shazam,
built from scratch in Go.

## 2. The core idea

You can't compare two audio recordings directly — a phone-mic recording of a song sounds
completely different, byte-for-byte, from the studio track. So instead of comparing audio,
the system:

1. Turns every song into a **fingerprint**: a sparse set of numbers derived only from its
   loudest, most distinctive frequencies.
2. Stores those fingerprints in a database, tagged with which song and what timestamp.
3. Does the same to your short clip, then checks: *do these fingerprints show up in the
   database, all at one consistent point in time?*

If yes → match. If the hits are scattered randomly across different timestamps → coincidence,
not a match.

## 3. The pipeline, in plain steps

```
Your audio file
      │
      ▼
1. Convert to a standard format (mono, fixed sample rate) — via FFmpeg
      │
      ▼
2. Slice into tiny overlapping time chunks, measure loudness at each frequency
   (this "picture" of sound-over-time is a spectrogram)
      │
      ▼
3. Pick out the loudest, sharpest points in that picture — the "peaks"
   (quiet stuff is thrown away; only strong, distinctive points survive)
      │
      ▼
4. Pair up nearby peaks and turn each pair into one 32-bit number (a "hash")
      │
      ▼
5a. ADDING a song → store {hash → song, timestamp} rows in SQLite
5b. MATCHING a clip → look up its hashes in the database, see which song's
    hits all line up at the same time offset → that's your match
```

### Why peaks, not raw audio?

Background noise, cheap microphones, and MP3 compression destroy quiet detail but leave
the loud, sharp points mostly intact. By keeping only those points, the fingerprint
survives conditions that would make a raw audio comparison useless.

### Why pairs of peaks, not single peaks?

One peak (say, "440 Hz was loud here") is shared by thousands of songs — not useful on its
own. A **pair** of peaks with a specific time gap between them is much rarer, so a handful of
matching pairs is enough to pin down one specific song.

### Why "time offset voting" instead of just counting matching hashes?

Random coincidental hash matches happen constantly, but they land at *random* points in time.
Real matches — because your clip really is a slice of that song — all land at the *same*
time offset (how far into the song your clip started). So the system counts, for each song,
which time-offset gets the most hits. A big spike at one offset = real match. A flat spread
across many offsets = noise.

## 4. How confident is a match?

Early on, "confidence" was a naive percentage that always came out near 0%, even for
correct matches — not useful. The current version instead looks at two things:

- **Peakiness**: how dominant the best time-offset is compared to the second-best. A real match has one sharp spike; noise is flat.
- **Margin**: how far ahead the top song is compared to the runner-up song. A confident hit
  clearly beats the next candidate; ambiguous noise ties with it.

These two are blended and passed through a curve to get a 0–100% score. There's also a
safety floor: if only a handful of hashes matched at all, the confidence gets capped low no
matter how "peaky" it looks — a coincidence of 2-3 hashes can't score high just because they
happened to line up.

## 5. The three ways to use it

All three call the exact same underlying logic (`pkg/acousticdna/service.go`) — nothing is
duplicated between them except the WASM voting code (see §7).

| Interface          | What it's for                                                     | Entry point               |
| ------------------ | ----------------------------------------------------------------- | ------------------------- |
| **CLI**            | Run locally from a terminal: add songs, match a clip, list/delete | `cmd/cli`                 |
| **REST API**       | A server other apps (or the web UI) talk to over HTTP             | `cmd/server`              |
| **Browser + WASM** | Same fingerprinting code, compiled to run *inside the browser*    | `cmd/wasm` + `web/public` |

### CLI (`cmd/cli`)

```bash
./acousticDNA add song.mp3 --title "Sandstorm" --artist "Darude"
./acousticDNA add --youtube-url "https://youtube.com/watch?v=..."
./acousticDNA match recording.wav
./acousticDNA list
./acousticDNA delete <song-id>
```

Two flags apply to every command: `-db <path>` (which database file to use) and
`-temp <dir>` (scratch space for converted audio). Both can also be set via
`ACOUSTIC_DB_PATH` / `ACOUSTIC_TEMP_DIR` environment variables. These aren't shown in the
README's quick examples but they're how you'd point the CLI at a different database.

### REST API (`cmd/server`)

| Route                     | Does                                                      |
| ------------------------- | --------------------------------------------------------- |
| `GET /health`             | Is the server alive                                       |
| `GET /api/health/metrics` | Song count, fingerprint row count, DB path, sample rate   |
| `GET /api/songs`          | List all songs                                            |
| `POST /api/songs`         | Add a song from an uploaded audio file                    |
| `POST /api/songs/youtube` | Add a song from a YouTube URL                             |
| `GET /api/songs/{id}`     | Get one song                                              |
| `DELETE /api/songs/{id}`  | Delete a song                                             |
| `POST /api/match`         | Match an uploaded audio clip                              |
| `POST /api/match/hashes`  | Match pre-computed hashes (used by the browser/WASM path) |

Server shuts down gracefully: on Ctrl-C it stops accepting new requests but gives
in-flight ones 30 seconds to finish before exiting.

### Browser + WASM (`cmd/wasm`, `web/public/wasm.js`)

This is the privacy-preserving option. Instead of uploading your raw audio file to the
server, your browser does the fingerprinting itself:

1. Page loads a compiled WebAssembly module (the same Go fingerprinting code, just
   compiled with `GOOS=js GOARCH=wasm` instead of for your OS).
2. You pick a file → the browser decodes and resamples it to the standard rate locally.
3. The WASM module runs the full spectrogram → peaks → hashing pipeline *in your browser*.
4. Only the resulting hashes (small integers, no actual sound) get sent to the server via
   `POST /api/match/hashes`.
5. Server looks up those hashes and votes, same as normal, and sends back the match.

Net effect: your actual recording never leaves your device — only ~60KB of opaque numbers
does, versus a multi-megabyte audio file. That's roughly a 98% bandwidth cut, measured on a
real test clip (see README's Performance section for the exact numbers).

## 6. Where things are stored

SQLite, two tables:

- **Song** — id, title, artist, YouTube ID, duration.
- **Fingerprint** — hash, which song it belongs to, and the timestamp inside that song.

The `hash` column is indexed, because every single match does one lookup of the form
"give me every row whose hash is in this list of thousands of hashes" — that index is what
makes matching fast instead of a full table scan.

Adding a song inserts tens of thousands of fingerprint rows (a 3-minute song generates
that many peak-pairs), so inserts happen in batches rather than one row at a time — batching
turns a slow operation into a fast one.

If adding a song fails partway through (audio saved but fingerprints failed to write), the
system deletes the half-added song rather than leaving a broken entry behind. Re-adding a
song that's already in the database (same title + artist) just returns the existing entry
instead of creating a duplicate.

## 7. Honest limitations (worth knowing going in)

- **No cover-song or tempo-shift matching.** This approach only recognizes an exact,
  same-speed recording — a live version, a cover, or a sped-up upload won't match. Shazam has
  this same limitation; it's inherent to the exact-hash approach, not a bug.
- **All the tuning numbers are hand-picked constants** (window size, hop size, thresholds),
  not learned from data.
- **SQLite has a ceiling.** Fine for a demo-sized catalog (the sample DB here is ~91MB for
  22 songs); a catalog with tens of thousands of songs would want a purpose-built index
  instead (e.g. Redis-backed inverted index) rather than `WHERE hash IN (...)` on SQLite.
- **Confidence is a heuristic**, not a statistically calibrated probability — it's good at
  separating real matches from noise, but "73% confidence" isn't a formal probability.
- **There are two copies of the voting logic** — one for normal file uploads
  (`generator.go`), one for the WASM/hash-only path (`service.go`). They're kept manually in
  sync and tested to match, but a cleaner version would share one implementation. Flagged as
  known tech debt, not a hidden bug.

## 8. Map: "I want to understand X, go read Y"

| I want to understand...             | Read this file                                       |
| ----------------------------------- | ---------------------------------------------------- |
| How sound becomes a spectrogram     | `pkg/acousticdna/fingerprint/spectrogram.go`         |
| How the "loudest points" get picked | `pkg/acousticdna/fingerprint/peaks.go`               |
| How two peaks become one number     | `pkg/acousticdna/fingerprint/hasher.go`              |
| How matching/voting works           | `pkg/acousticdna/fingerprint/generator.go`           |
| The main add/match business logic   | `pkg/acousticdna/service.go`                         |
| Confidence score math               | `pkg/acousticdna/service.go` (`calculateConfidence`) |
| Database schema & queries           | `pkg/acousticdna/storage/sqlite.go`                  |
| CLI commands                        | `cmd/cli/main.go`                                    |
| REST routes & handlers              | `cmd/server/routes.go`, `cmd/server/handlers.go`     |
| What runs inside the browser        | `cmd/wasm/main.go`, `web/public/wasm.js`             |

For the deeper "why" behind every design choice (window sizes, sample rate, hash bit
layout, scaling limits), see `docs/INTERVIEW.md` — this doc is the map, that one is the
terrain.

---

## 9. Appendix: DSP concepts Q&A (the "why" behind the numbers)

This section is a from-scratch walkthrough of the DSP concepts this project leans on —
useful for explaining the project (e.g. on a resume/interview) without hand-waving.

### 9.1 Resume line, decoded phrase by phrase

> "Engineered a full DSP pipeline in Go, STFT with Hamming windowing, spectral peak
> extraction, and 32-bit combinatorial hashing, identifying songs from noisy 5–15 second
> clips."

- **"full DSP pipeline"** — decode→spectrogram→peaks→hash was hand-built in Go
  (`pkg/acousticdna/fingerprint/*`, `pkg/acousticdna/audio/*`), not called from an existing
  fingerprinting library.
- **"STFT with Hamming windowing"** — `spectrogram.go`. 1024-sample windows, 256-sample hop
  (75% overlap), each window multiplied by a Hamming curve before the FFT.
- **"spectral peak extraction"** — `peaks.go`. Log-frequency bands, per-frame adaptive dB
  threshold, local-max check — this is what makes it noise-robust.
- **"32-bit combinatorial hashing"** — `hasher.go`. Pairs of peaks packed into one `uint32`:
  `anchorFreq[9 bits] | targetFreq[9 bits] | Δtime[14 bits]`.
- **"noisy 5-15s clips"** — the payoff, delivered by peak robustness + time-offset voting
  (§9.5 below).

> "Deployed WebAssembly processing sending only fingerprint hashes (~98% bandwidth
> reduction), with a single batched indexed lookup and CLI, REST, and WASM interfaces."

- **"Deployed WebAssembly processing"** — the same Go fingerprint code compiles to a normal
  binary (server) and to `GOOS=js GOARCH=wasm` (browser, `cmd/wasm/main.go`) — one codebase,
  two runtimes.
- **"sending only fingerprint hashes (~98% bandwidth reduction)"** — browser does the full
  pipeline locally, POSTs only the hash map to `/api/match/hashes`. Measured on a real 15s
  clip: ~3.74MB WAV → ~61KB of hashes (3,795 hashes). Say "measured on this clip, scales with
  clip length/format" if pressed for rigor — don't claim it as a universal constant.
- **"single batched indexed lookup"** — `storage/sqlite.go`: one `WHERE hash IN (...)`
  against an indexed `Hash` column instead of one query per hash.
- **"CLI, REST, and WASM interfaces"** — three thin entry points, all calling the same
  `acousticdna.Service` — no duplicated business logic.
- **Known gotcha to volunteer if asked**: the WASM path's vote/scoring logic is a second,
  hand-written copy in `service.go`, manually kept in sync with `generator.go`. Flag it
  proactively as known tech debt — reads as maturity, not a weakness.

### 9.2 STFT — Short-Time Fourier Transform

A plain FFT tells you what frequencies exist in a signal, but throws away *when* — one
spectrum for the whole clip, no time axis. Useless for music, where different frequencies
happen at different moments.

STFT fixes this by never transforming the whole signal at once:

1. Slice the audio into small overlapping windows — here, 1024 samples each, sliding
   forward 256 samples at a time (75% overlap).
2. Run a regular FFT on each window separately.
3. Stack the results into a 2D grid: **time (which window) × frequency (which bin) →
   magnitude**. That grid is the spectrogram.

Why this project needs it: every stored hash carries an anchor *time*. A plain FFT gives
frequencies with no timestamps — you can't build `(freq, freq, Δtime)` hashes without
knowing when each peak occurred. STFT is what makes "time-frequency peaks" a coherent idea
at all.

One slice is 1024 samples at 11,025 Hz = 1024/11025 ≈ **0.093s (~93ms)**.

**The resolution trade-off — why bigger window = better frequency resolution but worse
time resolution:**

Frequency resolution is `sample_rate / N` Hz per bin, where N = window size.

- N=1024 at 11,025Hz → 11025/1024 ≈ **10.77 Hz/bin**.
- N=2048 → ~5.4 Hz/bin — twice as fine. More samples in the FFT = more frequency buckets =
  finer resolution.

But that same window is also a chunk of time — N=2048 ≈ 186ms instead of 93ms. The FFT
gives **one** spectrum for the *entire* window; it can't say "the frequency changed halfway
through." A bigger window blurs together events happening close together in time — worse
time resolution. It's a direct trade from a fixed sample budget, not two independent knobs —
you can't maximize both at once. That's why 1024/256 is called a "sweet spot," not "the
correct value."

### 9.3 Hamming window — what and why

Each 1024-sample slice gets multiplied, sample-by-sample, by:

```
w(n) = 0.54 − 0.46·cos(2πn/(N−1))
```

Shaped like a smooth hump: ~0 at both edges of the window, ~1 in the middle.

**The problem it fixes:** FFT math assumes the windowed segment repeats infinitely
(periodic). A 1024-sample slice cut out of a continuous song almost never starts and ends
at the same value/slope — there's a discontinuity at the wrap-around point. That
discontinuity behaves, spectrally, like a sharp edge — and a sharp edge in time smears
energy across many frequencies that shouldn't have any (Fourier duality: sharp in one
domain = spread out in the other). This is **spectral leakage** — and to be precise about
where it happens: it's energy leaking across *frequency bins within one slice's spectrum*,
not leaking into other time slices. Each window is transformed independently either way.

**How Hamming fixes it:** tapering the slice's edges toward zero means sample 0 and sample
1023 are both near-silent, so there's no artificial jump at the wrap-around point, so far
less cross-bin smearing, so sharper, cleaner peaks per bin.

**Why it matters here specifically:** `peaks.go` decides what's a peak based on "is this
bin notably louder than the frame average, and a true local max." Leakage would smear real
peaks into fuzzy blobs and could invent fake peaks — polluting the exact spectral
fingerprint the whole matching algorithm depends on.

(If asked "why Hamming and not Hann/Blackman" — it's a standard, cheap, well-understood
default for this kind of audio analysis, not a hand-tuned research result. Fine to say
exactly that.)

### 9.4 FanOut — what it is and why 6

For every anchor peak, you don't hash it once — you pair it with the next `FanOut` peaks
that occur *after* it in time (here, 6). Each pairing produces one 32-bit hash. So one
anchor → up to 6 hashes, not 1. Database rows scale directly:
`total fingerprint rows ≈ peaks × FanOut`.

**Why more than 1 target at all:** if each anchor paired with exactly one fixed target and
that specific target peak got buried by noise, clipped by the query boundary, or altered by
compression, the hash never forms — you've lost your only shot at that anchor. Pairing with
several targets gives several independent chances per anchor; even if some target peaks are
corrupted in a noisy query clip, others usually survive, so you still generate *some*
matching hash for that region of the song.

**Why not push it much higher:** two costs rise with FanOut —

- **Database size**, linearly — doubling FanOut roughly doubles fingerprint rows per song.
- **Diminishing signal quality** — a target far in time from its anchor is a weaker,
  less distinctive relationship (this is also why `MaxDeltaMs = 15000` hard-rejects pairs
  over 15s apart). Later targets in the fan-out list cost the same storage as earlier ones
  but add progressively less discriminating power.

**Why 6 specifically:** no formula derives it — it's a hand-tuned constant, same category as
window=1024/hop=256, sitting in the range Shazam-style reimplementations commonly use
(small single digits: enough redundancy to survive noise without exploding the database).
Honest answer if pushed: it's an empirically-picked trade-off point, not a value with a
provable optimum — sweeping it against a real noisy-query benchmark would be legitimate
future work, not something this project has rigorously done.

### 9.5 Time-coherence voting, in depth

**Setup:** every query hash carries a `queryAnchorTime` (when that anchor peak occurred in
the clip). Look each hash up in the DB and get back every `Couple{songID, dbAnchorTimeMs}`
that ever produced that same hash, across the whole catalog.

**Step 1 — one offset per collision.** For every (query hash, matching DB row):

```
offset = dbAnchorTimeMs − queryAnchorTimeMs
```

This means: "if this clip really is a slice of this song, it started at position `offset`
within the song."

**Step 2 — bucket votes by (song, offset).** `votes[songID][offset] += 1`. Per candidate
song you build a histogram: x-axis = possible offsets, y-axis = how many hashes voted for
that offset.

**Step 3 — worked example.** Say your clip is really 47 seconds into "Sandstorm." Every
true hash match computes `offset ≈ 47000ms`, because that arithmetic is the same regardless
of *which* pair of peaks you're looking at — the whole clip is shifted by the same 47
seconds relative to the full song. True collisions all pile into **one** bucket:
`votes["Sandstorm"][47000]` climbs high. Meanwhile your clip also incidentally shares some
hashes with unrelated songs (expected — two unrelated peak-pairs can hash to the same
32-bit value by chance). Those coincidental matches have no reason to agree with each other
on a time offset — one lands at offset=12000, another at 88000, another at 3000 — scattered
randomly. No single bucket dominates.

**Step 4 — score by the tallest bucket.** For each song, `bestCount` = the highest bucket
(`bestOffset`, `bestCount`), `secondBestCount` = the runner-up bucket. Rank all candidates by
`bestCount` descending.

**Why this is the actual trick:** counting *total* matching hashes per song (skipping
offset-bucketing) would let coincidental hashes inflate wrong songs too — "20 hashes all
agreeing on one moment" and "20 hashes scattered everywhere by chance" would look identical
as a raw count. Bucketing by offset turns "a pile of matching hashes" into "concentrated
agreement on one specific alignment," which is the actual evidence of a genuine match. This
is exactly the `bestCount`/`secondBestCount` pair the confidence formula's "peakiness" term
reads from directly (§4 above).

### 9.6 Mono, sample rate, and frequency bins — three separate axes

Easy to conflate these three — worth splitting apart explicitly.

**Mono is about channel count, not frequency.** A stereo recording is 2 separate sample
streams (left ear, right ear); mono averages them into 1. Nothing to do with pitch — we do
it because fingerprinting only cares about *what frequencies are present*, not stereo
imaging, so keeping 2 channels would double the data for no benefit.

**Sample rate is about time, not frequency content of the music.** It means "how many
amplitude measurements we take per second" — a property of how the audio is stored, not a
property of the song. CD-quality is 44,100 samples/sec; this project deliberately stores
11,025/sec. Why downsample at all, if it's not "the frequency of the sound"? Because sample
rate puts a hard ceiling on which frequencies you're able to represent — the Nyquist limit:
`max representable frequency = sample_rate / 2`. At 11,025Hz that ceiling is ~5,512Hz. Music's
identifying content (fundamentals + important overtones) lives below ~5kHz; content above
that is mostly detail that noise/cheap mics/MP3 compression destroy anyway. So dropping to
11,025Hz discards range you don't need while keeping 4× fewer samples/sec than CD quality to
push through the FFT every window — a deliberate "spend samples where the signal actually
is" trade. (FFmpeg's resampling also low-pass filters before downsampling so high frequencies
don't fold — alias — into the wrong low bins; standard resampling behavior.)

**Frequency bins depend on window size, not directly on sample rate.** An FFT of an
N-sample window always produces N output values (512 kept, since the other half mirrors for
real-valued audio). Sample rate tells you which Hz each bin corresponds to:
`Hz per bin = sample_rate / window_size = 11025 / 1024 ≈ 10.77 Hz`. Bin 0 ≈ 0Hz, bin 1 ≈
10.77Hz, ... bin 512 ≈ 5,512Hz (Nyquist ceiling). Notice: at 44,100Hz with the same
1024-window you'd get `44100/1024 ≈ 43Hz/bin` — *coarser* resolution, spread across
0–22,050Hz instead of 0–5,512Hz. So downsampling to 11,025Hz doesn't just discard unneeded
high frequencies — it also gives *finer* resolution exactly where music's identifying
structure lives, for the same window size. Two benefits from one change.

| Axis                   | Controls                                                | Value here                                   |
| ---------------------- | ------------------------------------------------------- | -------------------------------------------- |
| Channels (mono/stereo) | how many parallel audio streams                         | 1 (mono)                                     |
| Sample rate            | samples/sec, caps max representable frequency (Nyquist) | 11,025 Hz → ceiling ~5,512Hz                 |
| Window size (FFT size) | how many frequency bins, Hz-per-bin resolution          | 1024 samples → 512 usable bins, ~10.77Hz/bin |

### 9.7 What a frequency bin actually stores (and what it doesn't)

A bin does **not** count peaks, and does **not** store a max picked from a range. It holds a
single **magnitude number** — computed straight from the FFT's complex output
(`sqrt(real² + imag²)`) — answering "how strongly is this exact frequency present, right
now, in this window." The full spectrogram is a dense grid: every frame × every bin always
has *some* magnitude value (a quiet bin just has a low number; nothing is ever "empty").

Also worth being precise about the input→output relationship: it's a **transform**, not a
relabeling. You feed in 1024 raw time-domain samples (just "how loud was the air pressure at
this instant," repeated 1024 times). Every one of the 1024 output bins is computed from
*all* 1024 input samples together — it is not that sample #17 becomes bin #17. The FFT asks,
for each of 1024 candidate frequencies, "how much of a pure sine wave at exactly this
frequency would explain the waveform I just saw across the whole window?" That's a
genuinely different operation than sorting or relabeling the same numbers.

"Hz range" language (the ~10.77Hz resolution) describes something subtler than "the bin
scans a range and keeps the max": a bin index maps to one exact center frequency
(`bin_freq = bin_index × sample_rate/1024`), but a real-world tone sitting *between* two bin
centers can't be represented exactly — its energy gets smeared across the nearest bin(s).
Resolution describes how finely two nearby real frequencies can be told apart, not a range
being scanned for a maximum.

**Peaks are a separate, later structure — not something bins hold.** `ExtractPeaks` runs
*after* the spectrogram exists: it groups the 512 bins into log-widening bands (0–10, 10–20,
20–40, …), and within each band picks the single highest-magnitude bin as that band's
candidate (**this** is where a "max" operation actually happens — over already-computed bin
magnitudes, once per frame, not inside the FFT). A candidate becomes a peak only if it also
clears the frame's 3dB-above-average threshold and is a true local max in its ±3 freq × ±1
time neighborhood. Most bins, in most frames, produce zero peaks — they're quiet background
that gets discarded. A peak is one specific `{TimeIdx, FreqIdx, Time, Freq, MagDB}` point,
not a tally.

| Structure             | What it is                                                                               | Density                         |
| --------------------- | ---------------------------------------------------------------------------------------- | ------------------------------- |
| Spectrogram           | a magnitude value for every bin, every frame                                             | dense — nothing skipped         |
| Peaks (constellation) | a short list of specific (time, freq) points that won their band + cleared the threshold | sparse — most bins produce none |

The spectrogram is raw material; peaks are a small, selective distillation of it. Hashing
(anchor/target pairing) only ever operates on the sparse peak list — the dense spectrogram
is thrown away once peaks are extracted, and is not what gets stored in the database.
