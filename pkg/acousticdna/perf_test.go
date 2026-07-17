//go:build !js && !wasm
// +build !js,!wasm

package acousticdna

// Perf verification for the two quantified resume claims, measured against the
// real demo DB (test/../acousticdna.sqlite3) and a real query clip. Skips when
// those artifacts are absent so it never breaks CI on a bare checkout.
//
//	go test ./pkg/acousticdna -run TestPerfClaims -v
//
// It asserts nothing tight — it PRINTS the measured numbers so we can compare
// against the resume ("99.5% bandwidth reduction", "60x SQL speedup").

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/audio"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/fingerprint"
	"github.com/himanishpuri/AcousticDNA/pkg/acousticdna/storage"
)

const perfSampleRate = 11025 // matches cmd/cli defaultSampleRate

func findRepoRoot(t *testing.T) string {
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("repo root (go.mod) not found")
	return ""
}

// queryHashes fingerprints a real clip exactly as the match path does.
func queryHashes(t *testing.T, root string) []uint32 {
	clip := filepath.Join(root, "test", "testCroppedAudio", "bruatiful_test.wav")
	if _, err := os.Stat(clip); err != nil {
		t.Skipf("query clip missing: %v", err)
	}
	wav, err := audio.ConvertToMonoWAV(context.Background(), clip, t.TempDir(),
		audio.ConvertWAVConfig{SampleRate: perfSampleRate})
	if err != nil {
		t.Skipf("ffmpeg unavailable: %v", err)
	}
	samples, sr, err := audio.ReadWavAsFloat64(wav)
	if err != nil {
		t.Fatalf("read wav: %v", err)
	}
	spec, err := fingerprint.ComputeSpectrogramFromSamples(samples, sr, 0, 0)
	if err != nil {
		t.Fatalf("spectrogram: %v", err)
	}
	peaks := fingerprint.ExtractPeaks(spec, sr)
	fps := fingerprint.Fingerprint(peaks, "")
	hashes := make([]uint32, 0, len(fps))
	for h := range fps {
		hashes = append(hashes, h)
	}
	return hashes
}

func TestPerfClaims(t *testing.T) {
	root := findRepoRoot(t)
	dbPath := filepath.Join(root, "acousticdna.sqlite3")
	fi, err := os.Stat(dbPath)
	if err != nil {
		t.Skipf("real DB missing: %v", err)
	}

	db, err := storage.NewDBClientWithPath(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	hashes := queryHashes(t, root)
	if len(hashes) == 0 {
		t.Fatal("no query hashes produced")
	}

	// --- Claim 1: bandwidth. Raw WAV bytes vs hash-map wire payload. ---
	// WASM path POSTs {hashes: {hashStr: anchorMs}} JSON. Approximate the wire
	// size: ~16 bytes/entry (uint32 hash as decimal string + uint32 anchor).
	clipInfo, _ := os.Stat(filepath.Join(root, "test", "testCroppedAudio", "bruatiful_test.wav"))
	rawBytes := clipInfo.Size()
	const bytesPerEntry = 16
	hashPayload := int64(len(hashes) * bytesPerEntry)
	reduction := 100.0 * (1.0 - float64(hashPayload)/float64(rawBytes))
	t.Logf("BANDWIDTH: raw WAV=%d B, hash payload≈%d B (%d hashes) → %.2f%% reduction",
		rawBytes, hashPayload, len(hashes), reduction)

	// --- Claim 2: batched IN(...) vs per-hash SELECT loop. ---
	start := time.Now()
	batched, err := db.GetCouplesByHashes(hashes)
	if err != nil {
		t.Fatalf("batched lookup: %v", err)
	}
	batchedDur := time.Since(start)

	start = time.Now()
	perHashRows := 0
	for _, h := range hashes {
		m, err := db.GetCouplesByHashes([]uint32{h}) // one hash per query = N round trips
		if err != nil {
			t.Fatalf("per-hash lookup: %v", err)
		}
		perHashRows += len(m)
	}
	perHashDur := time.Since(start)

	speedup := float64(perHashDur) / float64(batchedDur)
	t.Logf("SQL: batched IN(%d)=%v (%d hashes hit) | per-hash loop=%v → %.1fx speedup",
		len(hashes), batchedDur, len(batched), perHashDur, speedup)
	t.Logf("DB size on disk: %d MB", fi.Size()/(1024*1024))
}
