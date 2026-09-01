// Whether this machine's bandwidth may be spent on terrain.
//
// A plain launch opens a national network and measures every link in it, and
// every link wants ground underneath it. On a fresh install that was 513 MB
// over six thousand tiles, downloaded before the operator had decided they
// wanted the software: nothing asked, nothing said what it would cost, and
// nothing offered to decline. Terrain is the only large thing the application
// fetches on its own initiative, so it is the only thing that has to ask.
//
// Three states, not two. Allowed and refused are both answers; never asked is
// the state a fresh install is in, and it is the one that holds a warm rather
// than guessing on the operator's behalf.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// terrainAllowed reports whether terrain may be downloaded.
//
// True where there is nowhere to keep an answer: a session with no settings
// file is a test or an embedding, and a question whose answer cannot be
// remembered is one that would be asked again on every launch.
func (s *Sim) terrainAllowed() bool {
	if !s.persist {
		return true
	}
	return s.prefs.TerrainDownloads != nil && *s.prefs.TerrainDownloads
}

// terrainAsked reports whether the operator has answered either way.
func (s *Sim) terrainAsked() bool {
	return !s.persist || s.prefs.TerrainDownloads != nil
}

// setTerrainDownloads records the answer and applies it to the live store.
//
// Both, because the store is built once and kept: setting only the preference
// left the running session downloading exactly as before and the promise the
// switch made unkept until the next launch.
func (s *Sim) setTerrainDownloads(w *state.World, on bool) {
	s.prefs.TerrainDownloads = &on
	if ts, ok := s.terr.(*terrain.TileStore); ok && ts != nil {
		ts.Offline = !on
	}
	w.TerrainDownloads = on
}

// heldForTerrain reports whether this warm must stop and ask first, and says so
// when it does.
//
// Held rather than measured without the ground: the engine's fallback for a
// profile it cannot walk is free space, which is the most optimistic answer
// there is. A first launch that quietly reported every link as reachable would
// be worse than one that reported nothing, because only one of the two is
// obviously incomplete.
func (s *Sim) heldForTerrain(ctx context.Context, st *state.Store, nodes []scenario.Node) bool {
	if s.terrainAsked() {
		return false
	}
	ts, ok := s.terrain().(*terrain.TileStore)
	if !ok || ts == nil || len(nodes) == 0 {
		return false
	}
	// The tiles this warm would actually walk, priced. Nothing missing is no
	// question to ask: the ground is already here and the warm costs nothing.
	est := ts.EstimateTiles(profileTiles(nodes, ts.Zoom))
	if est.ToFetch == 0 {
		return false
	}
	done, release := finishing(ctx)
	defer release()
	_, _ = st.Do(done, "job.progress", state.Job{
		ID: "links", What: "waiting for permission to download terrain",
		Done: 1, Total: 1, Finished: true, Failed: true})
	_, _ = st.Do(done, "ui.said", fmt.Sprintf(
		"measuring these links needs ground this machine has not got: about %d MB "+
			"over %d terrain tiles. Nothing has been downloaded. Allow it in "+
			"Configuration > System, or run terrain.allow, or start with -fixture \"\" "+
			"to open nothing at all",
		est.BytesRough>>20, est.ToFetch))
	// Nothing is being measured, so nothing should be waited on: leaving the
	// warm flagged as running blocks every run behind a measurement that will
	// never finish on its own.
	s.warmMu.Lock()
	s.warmed = true
	s.warmMu.Unlock()
	return true
}

// registerTerrainConsent is the answer, and the switch that gives it.
func registerTerrainConsent(st *state.Store, s *Sim) {
	st.Handle("terrain.allow", func(w *state.World, p any) (any, error) {
		// Allow by default: this verb exists to grant permission, and the
		// caller who wrote no argument at all wrote the common case.
		on := true
		if v, ok := boolField(p, "on"); ok {
			on = v
		}
		asked := s.terrainAsked()
		s.setTerrainDownloads(w, on)
		if err := s.savePrefs(w); err != nil {
			w.Say("terrain downloads are " + onOff(on) + " for this session")
		} else if on {
			w.Say("terrain downloads are on, here and on the next launch")
		} else {
			w.Say("terrain downloads are off: links are measured on the ground " +
				"already cached, which over an unfetched area is free space and " +
				"therefore the most optimistic answer there is")
		}
		// The warm that was held has to be started again by hand, because the
		// thing that held it has returned: nothing is watching for permission
		// to arrive.
		warming := false
		if on && !asked && len(s.nodes) > 0 {
			s.warmMu.Lock()
			s.warmed = false
			s.warmMu.Unlock()
			s.warm(st, len(s.nodes))
			warming = true
		}
		return map[string]any{"on": on, "asked": asked, "warming": warming}, nil
	})
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
