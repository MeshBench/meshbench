package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// rfStore is the verb set with the store's goroutine running behind it.
func rfStore(t *testing.T) (*state.Store, *Sim) {
	t.Helper()
	st, s := register(t)
	go st.Run(t.Context())
	return st, s
}

// Asked with nothing, rf.mode reports rather than refuses.
//
// The mode is stamped into every result, so a script that has to say which
// physics produced a number needs to be able to ask. There was no way to: the
// only reader was the snapshot, which a socket client does not have, and the
// verb answered a bare call with `rf.mode is calculated or waveform, not ""`.
// The only route to the answer was to set a mode, which is the one thing a
// caller asking the question does not want to do.
func TestRFModeReportsTheModeInForce(t *testing.T) {
	st, _ := rfStore(t)
	ctx := t.Context()

	// The default, before anything has set one. A fresh session is calculated
	// and has to say so rather than answering with an empty string.
	got, err := st.Do(ctx, "rf.mode", nil)
	if err != nil {
		t.Fatalf("rf.mode with no argument was refused: %v", err)
	}
	if m := mapOf(t, got)["mode"]; m != "calculated" {
		t.Fatalf("a fresh session reports %v, want calculated", m)
	}

	if _, err := st.Do(ctx, "rf.mode", "waveform"); err != nil {
		t.Fatal(err)
	}
	got, err = st.Do(ctx, "rf.mode", nil)
	if err != nil {
		t.Fatalf("rf.mode with no argument was refused: %v", err)
	}
	if m := mapOf(t, got)["mode"]; m != "waveform" {
		t.Fatalf("after setting waveform the mode reads %v", m)
	}

	// The empty object is the same question, because a client that builds its
	// parameters as a map sends {} rather than null.
	got, err = st.Do(ctx, "rf.mode", map[string]any{})
	if err != nil {
		t.Fatalf("rf.mode with an empty object was refused: %v", err)
	}
	if m := mapOf(t, got)["mode"]; m != "waveform" {
		t.Fatalf("an empty object reports %v, want waveform", m)
	}
}

// Reporting must not be a set in disguise.
//
// The shape this guards against is the one the whole issue is about: a verb
// that answers success while quietly doing something else. A getter that fell
// through to setRFMode with an empty mode would either refuse - the old
// behaviour - or, worse, put the session back on calculated and report that as
// the mode in force.
func TestRFModeAskedForNothingChangesNothing(t *testing.T) {
	st, s := rfStore(t)
	ctx := t.Context()
	if _, err := st.Do(ctx, "rf.mode", "waveform"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := st.Do(ctx, "rf.mode", nil); err != nil {
			t.Fatal(err)
		}
	}
	if s.rfMode != "waveform" {
		t.Fatalf("asking the mode three times left the session on %q", s.rfMode)
	}
	if got := st.Snapshot().RFMode; got != "waveform" {
		t.Fatalf("the snapshot says %q after three reads", got)
	}
}

// A mode that was supplied and cannot be read is refused, not reported.
//
// This is the line between "not asked for" and "asked for and unusable". A
// caller who sent a number and was handed back the mode already in force would
// read it as the mode they had just set.
func TestRFModeRefusesAModeItCannotRead(t *testing.T) {
	st, _ := rfStore(t)
	for _, params := range []any{
		map[string]any{"mode": 5},
		map[string]any{"mode": true},
		[]any{"waveform"},
	} {
		msg := refuses(t, st, "rf.mode", params)
		mentions(t, msg, "rf.mode")
	}
	// And a mode that is a string but not one of the two.
	msg := refuses(t, st, "rf.mode", "wavefrom")
	mentions(t, msg, "rf.mode", "calculated", "waveform")
}

// The same shape one verb along: rf.environment set the buildings layer and
// refused to say what it was set to, so a script reporting a result could not
// say whether buildings had been priced into the path without changing the
// answer to find out.
func TestRFEnvironmentReportsWhatIsLoaded(t *testing.T) {
	st, s := rfStore(t)
	ctx := t.Context()

	got, err := st.Do(ctx, "rf.environment", nil)
	if err != nil {
		t.Fatalf("rf.environment with no argument was refused: %v", err)
	}
	if e := mapOf(t, got)["environment"]; e != "" {
		t.Fatalf("bare earth reports %v, want the empty string", e)
	}

	s.envDir = "/tiles/glasgow"
	got, err = st.Do(ctx, "rf.environment", map[string]any{})
	if err != nil {
		t.Fatalf("rf.environment with an empty object was refused: %v", err)
	}
	if e := mapOf(t, got)["environment"]; e != "/tiles/glasgow" {
		t.Fatalf("it reports %v, not the tiles that are loaded", e)
	}
	// And asking did not unload them, which is what a getter that fell through
	// to the setter's "on: false" branch would have done.
	if s.envDir != "/tiles/glasgow" {
		t.Fatalf("asking unloaded the environment; it is now %q", s.envDir)
	}

	// A dir that was named and is empty is still refused: supplied and
	// unusable is not the same as not supplied.
	msg := refuses(t, st, "rf.environment", map[string]any{"dir": ""})
	mentions(t, msg, "rf.environment")
}

func mapOf(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("answer is a %T, not an object", v)
	}
	return m
}
