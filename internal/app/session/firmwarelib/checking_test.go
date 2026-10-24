package firmwarelib

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A build nobody has yet, while the catalogue has not answered, is "checking",
// not "not published".
//
// On a fresh install Setup sends a new user to the library to download a build
// for each role, and the library opened by calling every one of them
// unavailable: it had asked the catalogue and not yet heard back.
func TestTheLibrarySaysCheckingUntilTheCatalogueAnswers(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)

	nodes := []scenario.Node{
		{Name: "hill", Kind: scenario.SimpleRepeater, Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.1"}},
		{Name: "deck", Kind: scenario.SimpleRepeater, Firmware: scenario.FirmwareRef{
			Version: "v1.17.1", Board: "LilyGo_TDeck"}},
	}
	store := state.New(10)
	sim := &session.Sim{}
	sim.BuildSeeded(nodes, 869.618, 1)
	register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)

	// A fetch in flight, held there so the test does not reach the network.
	catalogueOf(sim).fetching = true
	row := func(version string) state.FirmwareRow {
		t.Helper()
		if _, err := store.Do(ctx, "firmware.library", nil); err != nil {
			t.Fatal(err)
		}
		for _, r := range store.Snapshot().Library {
			if r.Version == version {
				return r
			}
		}
		t.Fatalf("the pinned build %s has no row", version)
		return state.FirmwareRow{}
	}
	native, board := func() state.FirmwareRow { return row("repeater-v1.17.1") },
		func() state.FirmwareRow { return row("v1.17.1") }

	for name, r := range map[string]state.FirmwareRow{"native": native(), "board": board()} {
		if !r.Checking || r.Unavailable || r.CatalogueDown {
			t.Errorf("%s, while the catalogue is asked: %+v, want checking only", name, r)
		}
	}

	// The native half could not be reached: not known, rather than not
	// there. The board half answered, and the image is not in it.
	if _, err := store.Do(ctx, "firmware.published", publishedAnswer{nativeDown: true}); err != nil {
		t.Fatal(err)
	}
	if r := native(); !r.CatalogueDown || r.Unavailable || r.Checking {
		t.Errorf("native, after its catalogue failed: %+v, want catalogue down only", r)
	}
	if r := board(); !r.Unavailable || r.CatalogueDown || r.Checking {
		t.Errorf("board, after its catalogue answered: %+v, want unavailable only", r)
	}

	// And the other way about.
	if _, err := store.Do(ctx, "firmware.published", publishedAnswer{boardsDown: true}); err != nil {
		t.Fatal(err)
	}
	if r := native(); !r.Unavailable || r.CatalogueDown || r.Checking {
		t.Errorf("native, after its catalogue answered: %+v, want unavailable only", r)
	}
	if r := board(); !r.CatalogueDown || r.Unavailable || r.Checking {
		t.Errorf("board, after its catalogue failed: %+v, want catalogue down only", r)
	}

	// Both reached, neither has it: now both are unavailable.
	if _, err := store.Do(ctx, "firmware.published", []publishedBuild{}); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]state.FirmwareRow{"native": native(), "board": board()} {
		if !r.Unavailable || r.Checking || r.CatalogueDown {
			t.Errorf("%s, after the catalogue answered without it: %+v, want unavailable only", name, r)
		}
	}
}
