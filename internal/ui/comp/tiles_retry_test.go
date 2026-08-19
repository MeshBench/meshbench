package comp

import (
	"testing"
	"time"
)

// A tile whose download fails must be asked for again.
//
// fetched is set before the request and doubles as the "already asked" mark,
// so leaving it set after a failure - which is what discarding the error did -
// meant one transient failure blanked that tile for the whole session. A zoom
// into uncached country fires a hundred requests at once, so a handful failing
// is the normal case, not the rare one.
func TestAFailedTileIsRetriedAfterItsDelay(t *testing.T) {
	tl := newTestTiles()
	const k = "dark/12/2048/1362"
	now := time.Now()

	if !tl.mayFetch(k, now) {
		t.Fatal("a tile nobody has asked for was refused")
	}
	// In flight, or already fetched: not asked again.
	tl.fetched[k] = true
	if tl.mayFetch(k, now) {
		t.Error("asked for a tile that is already in flight")
	}

	// The failure path: the mark comes off and a wait is recorded.
	delete(tl.fetched, k)
	tl.retryAfter[k] = now.Add(tileRetryDelay)
	if tl.mayFetch(k, now) {
		t.Error("retried a failed tile immediately, which is a retry storm")
	}
	if tl.mayFetch(k, now.Add(tileRetryDelay-time.Millisecond)) {
		t.Error("retried before the delay was up")
	}
	if !tl.mayFetch(k, now.Add(tileRetryDelay+time.Millisecond)) {
		t.Error("a failed tile was never retried - it stays blank for the session")
	}
}

// The wait applies to the tile that failed, not to its neighbours.
func TestOneFailedTileDoesNotHoldBackTheRest(t *testing.T) {
	tl := newTestTiles()
	now := time.Now()
	tl.retryAfter["dark/12/2048/1362"] = now.Add(tileRetryDelay)
	if !tl.mayFetch("dark/12/2049/1362", now) {
		t.Error("a neighbouring tile was held back by another tile's failure")
	}
}

func newTestTiles() *Tiles {
	return &Tiles{fetched: map[string]bool{}, retryAfter: map[string]time.Time{}}
}
