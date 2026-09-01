package state_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A verb name is a public contract across packages that cannot see each
// other's registrations, so nothing catches two of them claiming the same
// name except the store itself, at whichever one runs second. Handle used to
// let the second one win silently - which is how an internal verb could be
// made public by accident, by nothing more than an ordinary-looking Handle
// call for a name already taken.
func expectClaimPanic(t *testing.T, verb string, first, second func(*state.Store)) {
	t.Helper()
	s := state.New(10)
	first(s)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("registering %q twice did not panic", verb)
		}
		if !strings.Contains(r.(string), verb) {
			t.Fatalf("panic %q does not name the verb %q", r, verb)
		}
	}()
	second(s)
}

func handleNoop(verb string) func(*state.Store) {
	return func(s *state.Store) {
		s.Handle(verb, func(*state.World, any) (any, error) { return nil, nil })
	}
}

func TestHandleTwiceForTheSameVerbPanics(t *testing.T) {
	expectClaimPanic(t, "dup.handle", handleNoop("dup.handle"), handleNoop("dup.handle"))
}

func TestHandleSpecOverAnExistingVerbPanics(t *testing.T) {
	expectClaimPanic(t, "dup.spec", handleNoop("dup.spec"), func(s *state.Store) {
		s.HandleSpec("dup.spec", state.Spec{What: "second"},
			func(*state.World, any) (any, error) { return nil, nil })
	})
}

// The case the old "replace" behaviour made possible: a plain Handle call
// reusing an internal verb's name would have quietly made it public, since
// replacing a handler used to clear the private mark along with it.
func TestHandleOverAnInternalVerbPanicsRatherThanMakingItPublic(t *testing.T) {
	s := state.New(10)
	s.HandleInternal("dup.internal", func(*state.World, any) (any, error) { return nil, nil })
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("registering over an internal verb with Handle did not panic")
		}
		if !strings.Contains(r.(string), "dup.internal") {
			t.Fatalf("panic %q does not name the verb", r)
		}
		if !s.IsInternal("dup.internal") {
			t.Fatal("the verb stopped being internal despite the panic")
		}
	}()
	s.Handle("dup.internal", func(*state.World, any) (any, error) { return nil, nil })
}
