package session

import "testing"

func TestInterpolateIsDeterministic(t *testing.T) {
	mv := activeMove{node: "n", fromLat: 56.0, fromLon: -3.0, toLat: 57.0, toLon: -2.0,
		startMs: 1000, durationMs: 10000}
	lat1, lon1, done1 := interpolate(mv, 4000)
	lat2, lon2, done2 := interpolate(mv, 4000)
	if lat1 != lat2 || lon1 != lon2 || done1 != done2 {
		t.Fatalf("the same track at the same tick gave two different answers: (%v,%v,%v) vs (%v,%v,%v)",
			lat1, lon1, done1, lat2, lon2, done2)
	}
}

func TestInterpolateProgressesGradually(t *testing.T) {
	mv := activeMove{node: "n", fromLat: 0, fromLon: 0, toLat: 10, toLon: 0,
		startMs: 0, durationMs: 10000}
	// A node walking out of range should lose neighbours progressively, not
	// in one jump - checked here at the position level, since that is what
	// makes the neighbour count change gradually too.
	quarter, _, done := interpolate(mv, 2500)
	if done {
		t.Fatal("a quarter of the way through a track should not report done")
	}
	if quarter <= 0 || quarter >= 10 {
		t.Fatalf("a quarter of the way from 0 to 10 should be strictly between them, got %v", quarter)
	}
	half, _, _ := interpolate(mv, 5000)
	if half <= quarter {
		t.Fatalf("half-way (%v) should be further along than a quarter (%v)", half, quarter)
	}
}

func TestInterpolateReachesTheEndAndStops(t *testing.T) {
	mv := activeMove{node: "n", fromLat: 0, fromLon: 0, toLat: 10, toLon: 20, startMs: 0, durationMs: 1000}
	lat, lon, done := interpolate(mv, 1000)
	if !done {
		t.Fatal("a track at its own duration should report done")
	}
	if lat != 10 || lon != 20 {
		t.Fatalf("a finished track should land exactly on its destination, got %v,%v", lat, lon)
	}
	// Past the end, never an overshoot.
	lat, lon, done = interpolate(mv, 5000)
	if !done || lat != 10 || lon != 20 {
		t.Fatalf("well past the end: got %v,%v,%v, want the destination and done", lat, lon, done)
	}
}

func TestInterpolateZeroDurationIsAJump(t *testing.T) {
	mv := activeMove{node: "n", fromLat: 0, fromLon: 0, toLat: 5, toLon: 5, startMs: 100, durationMs: 0}
	lat, lon, done := interpolate(mv, 100)
	if !done || lat != 5 || lon != 5 {
		t.Fatalf("a zero-duration move should land immediately: got %v,%v,%v", lat, lon, done)
	}
}

func TestLongestGap(t *testing.T) {
	hist := []connSample{
		{atMs: 0, n: 6}, {atMs: 1000, n: 5}, {atMs: 2000, n: 3},
		{atMs: 3000, n: 0}, {atMs: 4000, n: 0}, {atMs: 5000, n: 0},
		{atMs: 6000, n: 0}, {atMs: 7000, n: 1}, {atMs: 8000, n: 3},
	}
	gapMs, atMs, minN := longestGap(hist)
	if gapMs != 4000 {
		t.Fatalf("longest gap = %d ms, want 4000 (3000..7000, the mockup's own four minutes-shaped example)", gapMs)
	}
	if atMs != 3000 {
		t.Fatalf("gap started at %d ms, want 3000", atMs)
	}
	if minN != 0 {
		t.Fatalf("min neighbours = %d, want 0", minN)
	}
}

// TestLongestGapReportsTwoSeparateGapsNotOne is the plan's own acceptance
// test: a node that leaves and returns shows two distinct gaps, not one
// long one averaged or merged across the return.
func TestLongestGapDistinguishesTwoSeparateGaps(t *testing.T) {
	hist := []connSample{
		{atMs: 0, n: 2},
		{atMs: 1000, n: 0}, {atMs: 2000, n: 0}, // gap 1: first zero at 1000, confirmed back at 3000 - 2000ms
		{atMs: 3000, n: 4},                                                             // back
		{atMs: 4000, n: 0}, {atMs: 5000, n: 0}, {atMs: 6000, n: 0}, {atMs: 7000, n: 0}, // gap 2: first zero at 4000
		{atMs: 8000, n: 2}, // confirmed back at 8000 - 4000ms
	}
	gapMs, atMs, _ := longestGap(hist)
	if gapMs != 4000 {
		t.Fatalf("longest gap = %d ms, want 4000 (the second, longer gap - not summed with the first)", gapMs)
	}
	if atMs != 4000 {
		t.Fatalf("longest gap started at %d, want 4000 (the second gap's own start)", atMs)
	}
}

func TestLongestGapWhenTheRunEndsMidGap(t *testing.T) {
	hist := []connSample{
		{atMs: 0, n: 3}, {atMs: 1000, n: 0}, {atMs: 2000, n: 0}, {atMs: 3000, n: 0},
	}
	gapMs, atMs, _ := longestGap(hist)
	if gapMs != 2000 {
		t.Fatalf("a gap still open when the run ends should count to the last sample: got %d, want 2000", gapMs)
	}
	if atMs != 1000 {
		t.Fatalf("gap start = %d, want 1000", atMs)
	}
}

func TestLongestGapWithNoGapAtAll(t *testing.T) {
	hist := []connSample{{atMs: 0, n: 4}, {atMs: 1000, n: 5}, {atMs: 2000, n: 3}}
	gapMs, _, minN := longestGap(hist)
	if gapMs != 0 {
		t.Fatalf("a node that always had neighbours should report no gap, got %d ms", gapMs)
	}
	if minN != 3 {
		t.Fatalf("min neighbours = %d, want 3", minN)
	}
}
