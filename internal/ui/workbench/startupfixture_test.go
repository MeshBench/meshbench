package workbench

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// openedVerbs records which project verbs the startup path reached, in order.
// Guarded because the handlers run on the store's goroutine.
type openedVerbs struct {
	mu   sync.Mutex
	seen []string
}

func (o *openedVerbs) add(v string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.seen = append(o.seen, v)
}

func (o *openedVerbs) all() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.seen...)
}

func (o *openedVerbs) saw(verb string) bool {
	for _, v := range o.all() {
		if strings.HasPrefix(v, verb) {
			return true
		}
	}
	return false
}

// startupHarness is a store where the project verbs are recorded rather than
// performed. openFails makes project.open refuse, which is the case where a
// name resolved to a file that turned out not to be a network.
func startupHarness(t *testing.T, openFails bool) (*state.Store, *openedVerbs) {
	t.Helper()
	st := state.New(10)
	seen := &openedVerbs{}
	st.Handle("project.open", func(_ *state.World, p any) (any, error) {
		name, _ := p.(string)
		seen.add("project.open " + name)
		if openFails {
			return nil, errors.New("it has no nodes")
		}
		return map[string]any{"opened": name}, nil
	})
	st.Handle("project.new", func(_ *state.World, _ any) (any, error) {
		seen.add("project.new")
		return map[string]any{"nodes": 0}, nil
	})
	st.Handle("ui.said", func(w *state.World, p any) (any, error) {
		line, _ := p.(string)
		w.Say(line)
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go st.Run(ctx)
	t.Cleanup(cancel)
	return st, seen
}

// An empty -fixture starts a blank network, not nothing at all.
//
// It is a deliberate idiom: a plain launch warms a 311-node default, and
// anything being measured wants a workbench with nothing in it. What it got
// instead was a session where project.open had been handed an empty string,
// refused, and been logged to stderr behind the splash - so nothing was ever
// installed. No engine, no tick, no network. The workbench came up, answered
// the control socket, and refused everything a caller went on to ask, several
// steps after the cause.
func TestAnEmptyFixtureStartsABlankNetwork(t *testing.T) {
	st, seen := startupHarness(t, false)
	openStartupNetwork(context.Background(), st, "")

	if !seen.saw("project.new") {
		t.Errorf("nothing installed a network: the startup path did %v", seen.all())
	}
	// And it did not try to open a network called nothing on the way past: that
	// call could only ever fail, and its failure is what used to be the whole
	// of the answer.
	if seen.saw("project.open") {
		t.Errorf("an empty -fixture still called project.open: %v", seen.all())
	}
	// Said where a person is looking, not only on stderr.
	saidSomethingAbout(t, st, "no network")
}

// A name that resolves to a file which then will not load leaves a usable
// session and says why, rather than a session that refuses everything.
func TestAFixtureThatWillNotLoadStillLeavesAWorkingSession(t *testing.T) {
	st, seen := startupHarness(t, true)
	openStartupNetwork(context.Background(), st, "somewhere.json")

	if !seen.saw("project.open") {
		t.Fatalf("it never tried to open the fixture: %v", seen.all())
	}
	if !seen.saw("project.new") {
		t.Errorf("a fixture that would not load left nothing installed: %v", seen.all())
	}
	saidSomethingAbout(t, st, "no nodes")
}

// A fixture that opens is opened, and nothing blanks it afterwards.
func TestAFixtureThatOpensIsNotThenBlanked(t *testing.T) {
	st, seen := startupHarness(t, false)
	openStartupNetwork(context.Background(), st, "fife-strict")

	if got := seen.all(); len(got) != 1 || got[0] != "project.open fife-strict" {
		t.Fatalf("opening a good fixture did %v", got)
	}
}

// A name this copy cannot resolve is refused at launch.
//
// Before, it was one line on stderr and then a workbench that could not run
// anything: the flag said which network to open, no network was opened, and
// the process carried on as though it had been. Resolving the name is a
// handful of stats and a look inside the binary, which is cheap enough to do
// before anything is constructed.
func TestAnUnresolvableFixtureIsRefusedAtLaunch(t *testing.T) {
	err := unknownFixture("no-network-is-called-this")
	if err == nil {
		t.Fatal("a name nothing answers to was accepted")
	}
	for _, want := range []string{"-fixture", "no-network-is-called-this"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not mention %q", err, want)
		}
	}
	// And the message says what could have been typed instead, which is the
	// difference between a refusal somebody can act on and one they have to go
	// and investigate.
	if !strings.Contains(err.Error(), "fife-strict") {
		t.Errorf("%q does not say what this copy does have", err)
	}
}

// The two names that must not be refused: nothing at all, which is the blank
// idiom, and a network this copy ships.
func TestResolvableFixturesAreNotRefused(t *testing.T) {
	for _, name := range []string{"", "fife-strict", "fixture-fife-strict"} {
		if err := unknownFixture(name); err != nil {
			t.Errorf("-fixture %q was refused: %v", name, err)
		}
	}
}

// saidSomethingAbout fails unless the status log carries the phrase.
func saidSomethingAbout(t *testing.T, st *state.Store, want string) {
	t.Helper()
	for _, line := range st.Snapshot().FullLog {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Errorf("nothing in the status log mentions %q; it says %v",
		want, st.Snapshot().FullLog)
}
