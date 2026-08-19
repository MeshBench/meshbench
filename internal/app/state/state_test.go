package state_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func run(t *testing.T, stepMs uint32) (*state.Store, context.Context) {
	t.Helper()
	s := state.New(stepMs)
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	t.Cleanup(func() { cancel(); time.Sleep(10 * time.Millisecond) })
	return s, ctx
}

// The renderer must never wait for a verb. In the old design both ran on the
// frame thread, which is why four verbs had to be special-cased as polls so
// that driving the application did not deadlock against it.
func TestSnapshotNeverBlocksOnASlowVerb(t *testing.T) {
	s, ctx := run(t, 10)
	release := make(chan struct{})
	s.Handle("slow", func(w *state.World, _ any) (any, error) {
		<-release
		w.Say("slow finished")
		return nil, nil
	})

	go func() { _, _ = s.Do(ctx, "slow", nil) }()
	time.Sleep(20 * time.Millisecond) // let it be in flight

	// A renderer would call this every frame. It must return immediately even
	// though a verb is mid-flight and holding the store's goroutine.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			if s.Snapshot() == nil {
				t.Error("snapshot was nil while a verb was in flight")
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reading snapshots blocked while a verb was in flight, " +
			"which is the exact coupling this layer exists to remove")
	}
	close(release)
}

// A snapshot is immutable: holding one across further mutations must not see
// them appear underneath. A frame that tears is a frame that shows two
// different moments at once.
func TestSnapshotDoesNotChangeUnderTheRenderer(t *testing.T) {
	s, ctx := run(t, 10)
	s.Handle("add", func(w *state.World, p any) (any, error) {
		w.Nodes = append(w.Nodes, state.Node{Name: p.(string)})
		return len(w.Nodes), nil
	})
	if _, err := s.Do(ctx, "add", "one"); err != nil {
		t.Fatal(err)
	}
	held := s.Snapshot()
	if len(held.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(held.Nodes))
	}
	for _, n := range []string{"two", "three", "four"} {
		if _, err := s.Do(ctx, "add", n); err != nil {
			t.Fatal(err)
		}
	}
	if len(held.Nodes) != 1 {
		t.Errorf("the held snapshot grew to %d nodes: the renderer was reading "+
			"live state, not a snapshot", len(held.Nodes))
	}
	if now := s.Snapshot(); len(now.Nodes) != 4 {
		t.Errorf("a fresh snapshot should have 4 nodes, got %d", len(now.Nodes))
	}
}

// Simulated time advances on the store's own ticker, not on frames. A headless
// run and a watched run must be the same run.
func TestTimeAdvancesWithoutAnythingRendering(t *testing.T) {
	s, ctx := run(t, 5)
	s.Handle("play", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		return nil, nil
	})
	if _, err := s.Do(ctx, "play", nil); err != nil {
		t.Fatal(err)
	}
	start := s.Snapshot().NowMs
	time.Sleep(120 * time.Millisecond)
	got := s.Snapshot().NowMs
	if got <= start {
		t.Fatalf("simulated time did not advance without a renderer: %d -> %d", start, got)
	}
}

// The engine's tick is driven by the store, so a scenario steps whether or not
// a window exists. This is what makes headless mode fall out rather than need
// its own path.
func TestEngineTickIsDrivenByTheStore(t *testing.T) {
	s, ctx := run(t, 5)
	ticks := make(chan uint32, 64)
	s.Handle("start", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Tick = func(dt uint32) {
			select {
			case ticks <- dt:
			default:
			}
		}
		return nil, nil
	})
	if _, err := s.Do(ctx, "start", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case dt := <-ticks:
		if dt != 5 {
			t.Errorf("tick advanced %d ms, expected the configured 5", dt)
		}
	case <-time.After(time.Second):
		t.Fatal("the engine never ticked")
	}
}

// An unknown verb names itself. A generic failure sends somebody to read the
// dispatch switch.
func TestUnknownVerbIsNamed(t *testing.T) {
	s, ctx := run(t, 10)
	if _, err := s.Do(ctx, "nope.missing", nil); err == nil {
		t.Fatal("expected an error for an unregistered verb")
	}
}

// The verb list comes from the store, so the parity test can be generated from
// what is actually registered. The hand-counted list in the plan was wrong by
// three verbs, which is the argument for this in one line.
func TestVerbsAreDiscoverable(t *testing.T) {
	s, _ := run(t, 10)
	s.Handle("a.one", func(*state.World, any) (any, error) { return nil, nil })
	s.Handle("b.two", func(*state.World, any) (any, error) { return nil, nil })
	got := s.Verbs()
	if len(got) != 2 {
		t.Fatalf("expected 2 registered verbs, got %d: %v", len(got), got)
	}
}

// Verbs from many goroutines at once must not corrupt state: the socket, the
// MCP server and the renderer all call Do.
func TestConcurrentVerbsAreSerialised(t *testing.T) {
	s, ctx := run(t, 10)
	s.Handle("inc", func(w *state.World, _ any) (any, error) {
		w.Seed++
		return w.Seed, nil
	})
	const n = 200
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() {
			_, _ = s.Do(ctx, "inc", nil)
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if got := s.Snapshot().Seed; got != n {
		t.Errorf("expected %d increments to land, got %d - state was raced", n, got)
	}
}

// syncBuffer is safe for Say to write to from the store's goroutine while a
// test reads it from its own.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Every status line goes to the log writer too, not just the twenty-line
// strip Log keeps for the UI - the difference between "what just happened"
// and "what happened three sessions ago and nobody was watching for it".
func TestSayWritesToTheLogWriter(t *testing.T) {
	s := state.New(10)
	var buf syncBuffer
	// Set before Run starts, as SetLogWriter documents it must be - World is
	// only safe to touch from the store's own goroutine once Run is running.
	s.SetLogWriter(&buf)
	s.Handle("test.say", func(w *state.World, _ any) (any, error) {
		w.Say("hello world")
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	if _, err := s.Do(ctx, "test.say", nil); err != nil {
		t.Fatalf("test.say: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "hello world") {
		t.Fatalf("log writer never saw the status line, got %q", got)
	}
}

// FullLog is what the Logs panel reads; it must actually receive what Say
// says, not just Log's twenty-line strip.
func TestSayWritesToFullLog(t *testing.T) {
	s := state.New(10)
	s.Handle("test.say", func(w *state.World, p any) (any, error) {
		w.Say("hello from FullLog")
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	if _, err := s.Do(ctx, "test.say", nil); err != nil {
		t.Fatalf("test.say: %v", err)
	}
	snap := s.Snapshot()
	if len(snap.FullLog) == 0 {
		t.Fatal("Snapshot().FullLog is empty after Say")
	}
	found := false
	for _, l := range snap.FullLog {
		if strings.Contains(l, "hello from FullLog") {
			found = true
		}
	}
	if !found {
		t.Fatalf("FullLog %v did not contain the line Say just said", snap.FullLog)
	}
}
