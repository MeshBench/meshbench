package session

import (
	"sync"
	"sync/atomic"
	"testing"
)

// The state seam must behave like the typed field it replaces: one instance per
// domain per Sim, created once, shared by every caller.
func TestDomainStateReturnsOneInstancePerKey(t *testing.T) {
	var s Sim
	type box struct{ n int }
	made := 0
	mk := func() any { made++; return &box{} }

	first := DomainState(&s, "d", mk).(*box)
	second := DomainState(&s, "d", mk).(*box)
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
	a := DomainState(&s, "a", func() any { return new(int) })
	b := DomainState(&s, "b", func() any { return new(int) })
	if a == b {
		t.Fatal("two keys shared one instance")
	}
}

// Two Sims never share state, even under the same key.
func TestDomainStateIsPerSim(t *testing.T) {
	var s1, s2 Sim
	a := DomainState(&s1, "d", func() any { return new(int) })
	b := DomainState(&s2, "d", func() any { return new(int) })
	if a == b {
		t.Fatal("two Sims shared one key's state")
	}
}

// A first-touch race still yields one instance, and the maker still runs once:
// this is why the lock is here rather than left to each domain.
func TestDomainStateFirstTouchIsRaceSafe(t *testing.T) {
	var s Sim
	var made int64
	mk := func() any { atomic.AddInt64(&made, 1); return new(int) }

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
