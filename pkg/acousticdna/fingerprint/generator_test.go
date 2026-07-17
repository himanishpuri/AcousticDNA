package fingerprint

import (
	"testing"

	"github.com/himanishpuri/AcousticDNA/pkg/models"
)

// mustAddr builds the hash address for an anchor->target peak pair, failing the
// test if the pair is rejected by createAddress.
func mustAddr(t *testing.T, anchor, target Peak) uint32 {
	t.Helper()
	addr, ok := createAddress(anchor, target)
	if !ok {
		t.Fatalf("createAddress(%+v, %+v) returned ok=false", anchor, target)
	}
	return addr
}

// TestQueryFingerprints locks the offset-alignment + best-offset-per-song
// reduction and the descending-Count ordering.
//
// Four query peaks at 0/100/200/300ms with distinct freq indices generate six
// unique pair addresses. The db map is hand-built so that:
//   - song A has 4 coherent votes at offset 5000 plus 1 decoy vote at offset 100
//     (proves best-offset-per-song picks the max bucket, not the sum),
//   - song C has 3 coherent votes at offset 7000,
//   - song B has 2 coherent votes at offset 3000.
//
// Expected result is sorted by Count descending: A(4), C(3), B(2).
func TestQueryFingerprints(t *testing.T) {
	qp := []Peak{
		{FreqIdx: 1, Time: 0.0},
		{FreqIdx: 2, Time: 0.1},
		{FreqIdx: 3, Time: 0.2},
		{FreqIdx: 4, Time: 0.3},
	}

	addr01 := mustAddr(t, qp[0], qp[1]) // query anchor 0ms
	addr02 := mustAddr(t, qp[0], qp[2]) // query anchor 0ms
	addr03 := mustAddr(t, qp[0], qp[3]) // query anchor 0ms
	addr12 := mustAddr(t, qp[1], qp[2]) // query anchor 100ms
	addr13 := mustAddr(t, qp[1], qp[3]) // query anchor 100ms
	addr23 := mustAddr(t, qp[2], qp[3]) // query anchor 200ms

	// Sanity: all six addresses are distinct, so votes never collide.
	seen := map[uint32]bool{}
	for _, a := range []uint32{addr01, addr02, addr03, addr12, addr13, addr23} {
		if seen[a] {
			t.Fatalf("expected six distinct addresses, got a collision on %d", a)
		}
		seen[a] = true
	}

	db := map[uint32][]models.Couple{
		addr01: {{SongID: "A", AnchorTimeMs: 5000}, {SongID: "B", AnchorTimeMs: 3000}},
		addr02: {{SongID: "A", AnchorTimeMs: 5000}, {SongID: "B", AnchorTimeMs: 3000}},
		addr03: {{SongID: "A", AnchorTimeMs: 5000}, {SongID: "C", AnchorTimeMs: 7000}},
		addr12: {{SongID: "A", AnchorTimeMs: 5100}, {SongID: "A", AnchorTimeMs: 200}}, // 5100->offset 5000, 200->offset 100 (decoy)
		addr13: {{SongID: "C", AnchorTimeMs: 7100}},                                   // ->offset 7000
		addr23: {{SongID: "C", AnchorTimeMs: 7200}},                                   // ->offset 7000
	}

	matches := QueryFingerprints(qp, db)

	// A's decoy vote at offset 100 is the second-tallest bin -> SecondBestCount 1.
	// C and B each have a single coherent offset -> SecondBestCount 0.
	want := []models.Match{
		{SongID: "A", OffsetMs: 5000, Count: 4, SecondBestCount: 1},
		{SongID: "C", OffsetMs: 7000, Count: 3, SecondBestCount: 0},
		{SongID: "B", OffsetMs: 3000, Count: 2, SecondBestCount: 0},
	}

	if len(matches) != len(want) {
		t.Fatalf("got %d matches, want %d: %+v", len(matches), len(want), matches)
	}
	for i, w := range want {
		if matches[i] != w {
			t.Fatalf("match[%d] = %+v, want %+v (full: %+v)", i, matches[i], w, matches)
		}
	}
}

// TestQueryFingerprintsNoDBMatch confirms addresses absent from the db produce
// no matches (empty, non-nil slice).
func TestQueryFingerprintsNoDBMatch(t *testing.T) {
	qp := []Peak{
		{FreqIdx: 1, Time: 0.0},
		{FreqIdx: 2, Time: 0.1},
	}
	matches := QueryFingerprints(qp, map[uint32][]models.Couple{})
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}

// TestQueryFingerprintsDescendingSort guards the descending-Count ordering
// independently of map iteration order by using unambiguous, strictly
// decreasing counts spread across many query pairs.
func TestQueryFingerprintsDescendingSort(t *testing.T) {
	matches := QueryFingerprints([]Peak{
		{FreqIdx: 1, Time: 0.0},
		{FreqIdx: 2, Time: 0.1},
		{FreqIdx: 3, Time: 0.2},
	}, map[uint32][]models.Couple{
		mustAddr(t, Peak{FreqIdx: 1, Time: 0.0}, Peak{FreqIdx: 2, Time: 0.1}): {
			{SongID: "hi", AnchorTimeMs: 1000},
			{SongID: "lo", AnchorTimeMs: 2000},
		},
		mustAddr(t, Peak{FreqIdx: 1, Time: 0.0}, Peak{FreqIdx: 3, Time: 0.2}): {
			{SongID: "hi", AnchorTimeMs: 1000},
		},
		mustAddr(t, Peak{FreqIdx: 2, Time: 0.1}, Peak{FreqIdx: 3, Time: 0.2}): {
			{SongID: "hi", AnchorTimeMs: 1100},
		},
	})

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %+v", matches)
	}
	if matches[0].SongID != "hi" || matches[1].SongID != "lo" {
		t.Fatalf("expected descending order [hi, lo], got %+v", matches)
	}
	if matches[0].Count < matches[1].Count {
		t.Fatalf("matches not sorted by descending Count: %+v", matches)
	}
}
