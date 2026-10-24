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
	row := func() state.FirmwareRow {
		t.Helper()
		if _, err := store.Do(ctx, "firmware.library", nil); err != nil {
			t.Fatal(err)
		}
		for _, r := range store.Snapshot().Library {
			if r.Version == "repeater-v1.17.1" {
				return r
			}
		}
		t.Fatal("the pinned build has no row")
		return state.FirmwareRow{}
	}

	if r := row(); !r.Checking || r.Unavailable || r.CatalogueDown {
		t.Errorf("while the catalogue is asked: %+v, want checking only", r)
	}

	// The catalogue could not be reached: not known, rather than not there.
	if _, err := store.Do(ctx, "firmware.published", publishedAnswer{failed: true}); err != nil {
		t.Fatal(err)
	}
	if r := row(); !r.CatalogueDown || r.Unavailable || r.Checking {
		t.Errorf("after a failed fetch: %+v, want catalogue down only", r)
	}

	// Reached, and it is not there: now it is unavailable.
	if _, err := store.Do(ctx, "firmware.published", []publishedBuild{}); err != nil {
		t.Fatal(err)
	}
	if r := row(); !r.Unavailable || r.Checking || r.CatalogueDown {
		t.Errorf("after the catalogue answered without it: %+v, want unavailable only", r)
	}
}
