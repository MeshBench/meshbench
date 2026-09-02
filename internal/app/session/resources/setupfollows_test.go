package resources_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/resources"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// asking builds a session that has a settings file to keep an answer in, and so
// has a terrain question worth asking.
//
// A Sim with nowhere to persist reads as allowed rather than undecided, on the
// reasoning that a question whose answer cannot be kept would be asked on every
// launch. That makes the bare Sim the other tests here use useless for this one:
// its terrain row is ready before anything has been granted.
func asking(t *testing.T) (*state.Store, context.Context) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	sim := &session.Sim{}
	if err := sim.LoadPrefs(); err != nil {
		t.Fatalf("LoadPrefs on an empty directory: %v", err)
	}
	store := state.New(10)
	session.Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go store.Run(ctx)
	return store, ctx
}

// terrainState reads the terrain row's state off the published snapshot, which
// is the same thing the Setup page draws and nothing else.
func terrainState(t *testing.T, store *state.Store) string {
	t.Helper()
	snap := store.Snapshot()
	if snap == nil {
		t.Fatal("the store has published no snapshot")
	}
	for _, g := range snap.Setup {
		for _, r := range g.Rows {
			if r.Verb == "terrain.allow" {
				return r.State
			}
		}
	}
	t.Fatalf("no terrain row in the setup page: %v", snap.Setup)
	return ""
}

// The page follows the answer, whichever route granted it.
//
// The row's own button re-runs setup.check itself and has always worked, so it
// is not what this guards. The two routes that did not are the switch in
// Configuration, which fires terrain.allow with the value it just moved to, and
// a script calling the same verb with no argument at all. Both used to leave the
// page reading "waiting on you" over a download that was already running.
func TestTheSetupPageFollowsConsentGrantedAnywhere(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params any
	}{
		// What internal/ui/workbench's Configuration switch sends.
		{"the Configuration switch", map[string]any{"on": true}},
		// What a script sends: the verb exists to grant, so bare means on.
		{"a scripted terrain.allow", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, ctx := asking(t)
			if _, err := store.Do(ctx, "setup.check", nil); err != nil {
				t.Fatalf("setup.check: %v", err)
			}
			if got := terrainState(t, store); got != string(state.SetupUndecided) {
				t.Fatalf("terrain reads %q before anything was granted, want %q",
					got, state.SetupUndecided)
			}

			if _, err := store.Do(ctx, "terrain.allow", tc.params); err != nil {
				t.Fatalf("terrain.allow: %v", err)
			}
			// Deliberately no setup.check here. Re-running it is the bug: the
			// page is supposed to already be right.
			if got := terrainState(t, store); got == string(state.SetupUndecided) {
				t.Fatalf("terrain still reads %q after consent was granted by %s",
					got, tc.name)
			}
		})
	}
}

// A page nobody has opened is not built by granting consent.
//
// The rebuild walks the resource cache on disk, and doing that on a verb that
// did not ask for it would put a disk walk behind every grant on a session that
// has no Setup page open. The panel asks on its first draw, which is where a
// page that has never been built comes from.
func TestGrantingConsentDoesNotBuildAPageNobodyAskedFor(t *testing.T) {
	store, ctx := asking(t)
	if _, err := store.Do(ctx, "terrain.allow", map[string]any{"on": true}); err != nil {
		t.Fatalf("terrain.allow: %v", err)
	}
	if snap := store.Snapshot(); snap != nil && len(snap.Setup) != 0 {
		t.Fatalf("granting consent built a setup page nobody asked for: %v", snap.Setup)
	}
	// And asking for it afterwards still answers, with the grant in it.
	if _, err := store.Do(ctx, "setup.check", nil); err != nil {
		t.Fatalf("setup.check: %v", err)
	}
	if got := terrainState(t, store); got == string(state.SetupUndecided) {
		t.Fatalf("terrain reads %q on a page built after the grant", got)
	}
}
