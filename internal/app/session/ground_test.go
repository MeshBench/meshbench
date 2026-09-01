package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// The three states are three states, and the middle one is not rounded to
// either end: a study over half a country of real ridges and half a country of
// sea level is the case that reads as an answer and is not one.
func TestGroundIsThreeStatesAndNotTwo(t *testing.T) {
	for _, c := range []struct {
		what  string
		est   terrain.Estimate
		state string
	}{
		{"nothing cached", terrain.Estimate{Tiles: 40}, state.GroundBare},
		{"one tile short", terrain.Estimate{Tiles: 40, Cached: 39}, state.GroundPartial},
		{"one tile only", terrain.Estimate{Tiles: 40, Cached: 1}, state.GroundPartial},
		{"all of it", terrain.Estimate{Tiles: 40, Cached: 40}, state.GroundTerrain},
	} {
		got := groundFrom(c.est, true)
		if got.State != c.state {
			t.Errorf("%s: ground is %q, not %q", c.what, got.State, c.state)
		}
		if (got.Note == "") != (c.state == state.GroundTerrain) {
			t.Errorf("%s: ground %q says %q, which is the wrong way round",
				c.what, got.State, got.Note)
		}
	}
}

// The note is the honesty line, so it has to name the consequence rather than
// the missing file. "No terrain" reads as a missing decoration; free space is
// a claim about every ridge in the study area.
func TestTheGroundNoteNamesFreeSpaceAndNotJustTheGap(t *testing.T) {
	bare := groundFrom(terrain.Estimate{Tiles: 40}, true)
	for _, want := range []string{"free space", "most optimistic", "bare earth"} {
		if !strings.Contains(bare.Note, want) {
			t.Errorf("a bare-earth study never says %q: %s", want, bare.Note)
		}
	}
	part := groundFrom(terrain.Estimate{Tiles: 40, Cached: 12}, true)
	if !strings.Contains(part.Note, "12 of 40") {
		t.Errorf("a partial study does not say how much of it is missing: %s", part.Note)
	}
}

// Chosen is the whole distinction: an offline run somebody asked for is a
// legitimate result, and one nobody was asked about is the model being quietly
// more optimistic than its own documented best case.
func TestAChosenBareEarthRunIsNotTheSameAsASilentOne(t *testing.T) {
	silent := groundFrom(terrain.Estimate{Tiles: 40}, false)
	chosen := groundFrom(terrain.Estimate{Tiles: 40}, true)
	if silent.Chosen || !chosen.Chosen {
		t.Fatalf("chosen is not being carried: silent=%v chosen=%v",
			silent.Chosen, chosen.Chosen)
	}
	if !strings.Contains(silent.Note, "terrain.allow") {
		t.Errorf("the unanswered case does not name the way to answer it: %s", silent.Note)
	}
	if strings.Contains(chosen.Note, "nobody has been asked") {
		t.Errorf("an answered machine is told nobody answered: %s", chosen.Note)
	}
}

// The refusal, and the two cases that must survive it. A study is refused only
// over ground nobody chose to do without; a chosen offline run still answers,
// and still says what it answered over.
func TestOnlyTheUnchosenBareEarthStudyIsRefused(t *testing.T) {
	w := &state.World{}
	silent := groundFrom(terrain.Estimate{Tiles: 40}, false)
	err := StudyGround(w, "coverage.map", silent)
	if err == nil {
		t.Fatal("a study over ground nobody chose to do without answered anyway")
	}
	if !strings.Contains(err.Error(), "coverage.map") ||
		!strings.Contains(err.Error(), "free space") {
		t.Errorf("the refusal does not say which verb or why: %v", err)
	}
	if w.Ground.State != state.GroundBare {
		t.Errorf("a refused study did not record what it refused over: %+v", w.Ground)
	}

	w = &state.World{}
	if err := StudyGround(w, "coverage.map",
		groundFrom(terrain.Estimate{Tiles: 40}, true)); err != nil {
		t.Fatalf("a deliberate offline run was refused: %v", err)
	}
	if !strings.Contains(strings.Join(w.Log, "\n"), "free space") {
		t.Errorf("a deliberate offline run said nothing about its ground: %v", w.Log)
	}

	w = &state.World{}
	if err := StudyGround(w, "coverage.map",
		groundFrom(terrain.Estimate{Tiles: 40, Cached: 40}, false)); err != nil {
		t.Fatalf("a study with all its ground was refused: %v", err)
	}
	if len(w.Log) != 0 {
		t.Errorf("a study with all its ground still made an excuse: %v", w.Log)
	}
}

// The caveat the chrome shows is a shout for the bare case, because it is the
// only one of the honesty line's clauses that can be true without anybody
// having chosen it.
func TestTheChromeCaveatSaysNoTerrainOnlyWhenThereIsNone(t *testing.T) {
	if got := (state.Ground{State: state.GroundBare}).Caveat(); got != "NO TERRAIN" {
		t.Errorf("the bare caveat is %q", got)
	}
	part := state.Ground{State: state.GroundPartial, Sampled: 40, Cached: 10}.Caveat()
	if !strings.Contains(part, "25%") {
		t.Errorf("the partial caveat does not say how much: %q", part)
	}
	if got := (state.Ground{State: state.GroundTerrain}).Caveat(); got != "" {
		t.Errorf("a study with its ground still carries a caveat: %q", got)
	}
	// The zero value is a session nothing has looked at, which must not read
	// as bare earth: a caveat drawn before any study would be a wrong caveat.
	if got := (state.Ground{}).Caveat(); got != "" {
		t.Errorf("an unexamined session claims a ground state: %q", got)
	}
}

// An empty cache with nowhere to answer the question is the fresh-install
// state, read end to end off a real tile store rather than off an estimate.
func TestAFreshCacheReportsBareEarthThroughTheTileStore(t *testing.T) {
	s, _, _ := consentSim(t)
	g := s.GroundOver(56.0, 56.3, -4.4, -3.9)
	if g.State != state.GroundBare {
		t.Fatalf("an empty tile cache reports %q with %d of %d tiles",
			g.State, g.Cached, g.Sampled)
	}
	if g.Sampled == 0 {
		t.Error("the ground was reported without a tile being looked for")
	}
	if g.Chosen {
		t.Error("a machine nobody has asked reports its bare earth as a choice")
	}
}
