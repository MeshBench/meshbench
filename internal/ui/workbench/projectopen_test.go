package workbench

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The whole of step one of the first-simulation page, at the seam it failed
// at: File, Open a saved network, choose the shipped network. On a machine
// that has never run MeshBench the reply carries no saved projects at all, and
// the chooser used to read only those - so it opened empty, with no error,
// and there was nothing to choose.
func TestOpenAskOffersTheShippedNetworkWhenNothingIsSaved(t *testing.T) {
	h := newShellHarness(t)
	st := state.New(10)
	opened := make(chan string, 1)
	st.Handle("project.list", func(_ *state.World, _ any) (any, error) {
		return map[string]any{
			"projects": []string{},
			"dir":      t.TempDir(),
			"fixtures": []string{"fixture-fife-strict", "fixture-scotland-strict"},
		}, nil
	})
	st.Handle("project.open", func(_ *state.World, p any) (any, error) {
		name, _ := p.(string)
		opened <- name
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	menuDeps{sh: h.sh, st: st, ctx: ctx}.onMenu("project.open")
	deadline := time.Now().Add(3 * time.Second)
	for !h.sh.Ask.Showing() {
		if time.Now().After(deadline) {
			t.Fatal("Open a saved network never asked anything")
		}
		h.frame()
		time.Sleep(5 * time.Millisecond)
	}
	// Narrowed the way somebody following the page narrows it, by typing what
	// they were told to look for.
	h.r.Queue(key.EditEvent{Text: "fife"})
	h.frame()
	h.r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	h.frame()

	select {
	case name := <-opened:
		if name != "fixture-fife-strict" {
			t.Fatalf("choosing the shipped network opened %q", name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("choosing the shipped network opened nothing")
	}
}

// The chooser a new user meets. Nothing saved, because nothing has been run
// here before: the rows have to be the shipped networks, and choosing one has
// to open by name, since on a fresh install the only copy is inside the
// binary and there is no path to join.
func TestOpenChoicesOffersShippedNetworksWhenNothingIsSaved(t *testing.T) {
	names, opens := openChoices(map[string]any{
		"projects": []string{},
		"dir":      "/home/nobody/.config/meshbench/projects",
		"fixtures": []string{"fixture-fife-strict", "fixture-scotland-strict"},
	})
	if len(names) != 2 {
		t.Fatalf("a fresh install was offered %v", names)
	}
	if !slices.Contains(names, "fixture-fife-strict"+builtInMark) {
		t.Fatalf("the shipped network is not on the list: %v", names)
	}
	if got := opens["fixture-fife-strict"+builtInMark]; got != "fixture-fife-strict" {
		t.Errorf("a shipped network opens %q, not by the name the fixture search knows", got)
	}
}

// A saved network still opens by path, and is not dressed up as a shipped one.
func TestOpenChoicesOpensSavedNetworksByPath(t *testing.T) {
	dir := "/home/nobody/.config/meshbench/projects"
	names, opens := openChoices(map[string]any{
		"projects": []string{"my-glen"},
		"dir":      dir,
		"fixtures": []string{"fixture-fife-strict"},
	})
	if names[0] != "my-glen" {
		t.Errorf("the user's own network is labelled %q", names[0])
	}
	if want := filepath.Join(dir, "my-glen.json"); opens["my-glen"] != want {
		t.Errorf("a saved network opens %q, not %q", opens["my-glen"], want)
	}
}

// The same reply arriving over the socket, where a list of names has been
// through JSON and comes back as []any. A picker that reads only []string
// shows an empty list and no error, which is the failure this whole path is
// about.
func TestOpenChoicesReadsNamesThatCameThroughJSON(t *testing.T) {
	names, _ := openChoices(map[string]any{
		"projects": []any{"my-glen"},
		"dir":      "/tmp/projects",
		"fixtures": []any{"fixture-fife-strict"},
	})
	if len(names) != 2 {
		t.Fatalf("names round-tripped through JSON were read as %v", names)
	}
}
