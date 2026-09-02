// Whether this machine's bandwidth may be spent on terrain.
//
// On unless somebody has turned it off. Terrain is not optional input to this
// simulator: it is what separates it from a link-budget rule, and an answer
// computed over bare earth where a DEM was available is not a cheaper answer
// but a wrong one, wrong in the optimistic direction.
//
// This used to be three states - allowed, refused, and never asked - with the
// third holding a warm until somebody answered. The problem it was written for
// was real: a fresh install spent 513 MB over six thousand tiles before the
// operator had decided they wanted the software, and nothing said what it
// would cost. The mistake was fixing that by blocking on a question rather
// than by making the spend visible, which left a flat earth as the resting
// state of every unanswered install. The prefetch announces its tiles and its
// megabytes and can be stopped, which is what that problem actually wanted.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
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
	return s.prefs.TerrainDownloads == nil || *s.prefs.TerrainDownloads
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

// registerTerrainConsent is the switch and the question a script asks before
// it believes a study.
func registerTerrainConsent(st *state.Store, s *Sim) {
	st.Handle("terrain.ground", func(w *state.World, _ any) (any, error) {
		w.Ground = s.GroundUnder(s.nodes)
		return w.Ground.Map(), nil
	})

	st.Handle("terrain.allow", func(w *state.World, p any) (any, error) {
		// Allow by default: this verb exists to grant permission, and the
		// caller who wrote no argument at all wrote the common case.
		on := true
		if v, ok := boolField(p, "on"); ok {
			on = v
		}
		was := s.terrainAllowed()
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
		// A matrix measured under the old answer is a matrix of different
		// ground, so switching either way remeasures. Turning downloads on is
		// the case that matters - what was walked over bare earth was the
		// optimistic answer - but turning them off matters too, because a
		// session that keeps showing links measured over terrain it is no
		// longer allowed to fetch is claiming a precision it will not have for
		// the next region opened.
		warming := false
		if was != on && len(s.nodes) > 0 {
			s.warmMu.Lock()
			s.warmed = false
			s.warmMu.Unlock()
			s.warm(st, len(s.nodes))
			warming = true
		}
		w.Ground = s.GroundUnder(s.nodes)
		// The readiness page reports this answer, and this verb is how the
		// Configuration switch and a script both give it. Left alone, the page
		// kept asking a question it had already been told the answer to.
		rebuildSetup(s, w)
		return map[string]any{"on": on, "warming": warming}, nil
	})
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
