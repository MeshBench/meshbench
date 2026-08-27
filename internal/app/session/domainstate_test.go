package session

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The state seam must behave like the typed field it replaces: one instance per
// domain per Sim, created once, shared by every caller.
func TestDomainStateReturnsOneInstancePerKey(t *testing.T) {
	var s Sim
	type box struct{ n int }
	made := 0
	mk := func() *box { made++; return &box{} }

	first := DomainState(&s, "d", mk)
	second := DomainState(&s, "d", mk)
	if first != second {
		t.Fatal("two calls for one key returned different instances")
	}
	if made != 1 {
		t.Fatalf("maker ran %d times, want once", made)
	}
	// Mutating through one handle is visible through the other, because it is
	// the same object - the whole point of holding state rather than copying.
	first.n = 7
	if second.n != 7 {
		t.Fatalf("second handle saw %d, not the write through the first", second.n)
	}
}

// Distinct keys are distinct state - one domain cannot read another's.
func TestDomainStateSeparatesKeys(t *testing.T) {
	var s Sim
	a := DomainState(&s, "a", func() *int { return new(int) })
	b := DomainState(&s, "b", func() *int { return new(int) })
	if a == b {
		t.Fatal("two keys shared one instance")
	}
}

// Two Sims never share state, even under the same key.
func TestDomainStateIsPerSim(t *testing.T) {
	var s1, s2 Sim
	a := DomainState(&s1, "d", func() *int { return new(int) })
	b := DomainState(&s2, "d", func() *int { return new(int) })
	if a == b {
		t.Fatal("two Sims shared one key's state")
	}
}

// A first-touch race still yields one instance, and the maker still runs once:
// this is why the lock is here rather than left to each domain.
func TestDomainStateFirstTouchIsRaceSafe(t *testing.T) {
	var s Sim
	var made int64
	mk := func() *int { atomic.AddInt64(&made, 1); return new(int) }

	const n = 32
	var wg sync.WaitGroup
	got := make([]any, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) { defer wg.Done(); got[i] = DomainState(&s, "d", mk) }(i)
	}
	wg.Wait()
	if made != 1 {
		t.Fatalf("maker ran %d times under contention, want once", made)
	}
	for i := 1; i < n; i++ {
		if got[i] != got[0] {
			t.Fatalf("goroutine %d got a different instance", i)
		}
	}
}

// The teardown and tick hooks run what was registered, in order, against the
// Sim and (for ticks) the world - the seams a state-holding domain uses in
// place of the inline core code it replaced.
func TestTeardownAndTickHooksRun(t *testing.T) {
	// Save and restore the process-wide registries so this test does not leak
	// hooks into others in the package.
	savedT, savedK := teardowns, ticks
	t.Cleanup(func() { teardowns, ticks = savedT, savedK })
	teardowns, ticks = nil, nil

	var s Sim
	torn := 0
	RegisterTeardown(func(*Sim) { torn++ })
	runTeardowns(&s)
	runTeardowns(&s)
	if torn != 2 {
		t.Fatalf("teardown ran %d times over two calls, want 2", torn)
	}

	ticked := 0
	RegisterTick(func(_ *Sim, _ *state.World) { ticked++ })
	var w state.World
	runTicks(&s, &w)
	if ticked != 1 {
		t.Fatalf("tick ran %d times, want 1", ticked)
	}
}
