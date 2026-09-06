package session

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// The verdict asks about the ground the study will actually walk.
//
// It asked about a 16x16 grid across the bounding box while the warm fetched
// the ground under the lines between pairs. Those are different sets: on
// fife-strict, 18% of the grid lands on tiles no profile crosses - the Firth of
// Forth, mostly - and on a national study it is the whole of the sea between
// Scotland and Ireland. Those tiles are never fetched because nothing will ever
// sample them, and their absence was then reported as missing ground, so a
// machine holding every tile its links would ask for was told for ever that
// part of its answer was free space.
func TestTheGroundVerdictAsksAboutTheProfilesRatherThanTheBox(t *testing.T) {
	f, err := LoadFixture("../../../fixtures/fixture-fife-strict.json")
	if err != nil {
		t.Fatal(err)
	}
	zoom := terrain.DefaultZoom

	onProfile := map[[2]int]bool{}
	for _, k := range profileTiles(f.scene, zoom) {
		onProfile[k] = true
	}
	if len(onProfile) == 0 {
		t.Fatal("no profile tiles for a 58-node fixture")
	}

	south, north := math.Inf(1), math.Inf(-1)
	west, east := math.Inf(1), math.Inf(-1)
	for i := range f.scene {
		south = math.Min(south, f.scene[i].Position.Lat)
		north = math.Max(north, f.scene[i].Position.Lat)
		west = math.Min(west, f.scene[i].Position.Lon)
		east = math.Max(east, f.scene[i].Position.Lon)
	}

	// The box grid really does ask about ground no link needs, which is the
	// whole reason the two must not be confused.
	var strays int
	for _, k := range groundSampleTiles(south, north, west, east, zoom) {
		if !onProfile[k] {
			strays++
		}
	}
	if strays == 0 {
		t.Skip("the box and the profiles happen to agree on this fixture, so " +
			"there is nothing here to tell apart")
	}
	t.Logf("%d of the box grid's tiles lie on no profile", strays)
}

// A fleet with nothing worth measuring between it still gets an answer.
//
// profileTiles culls pairs physics has already refused, so one node - or a
// scattering too far apart to hear each other - yields no tiles at all. That
// must not read as "all the ground is here": there is no profile to have
// ground under, and the box is the only question left.
func TestOneNodeStillGetsAGroundAnswer(t *testing.T) {
	f, err := LoadFixture("../../../fixtures/fixture-fife-strict.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := profileTiles(f.scene[:1], terrain.DefaultZoom); len(got) != 0 {
		t.Fatalf("one node should walk no profiles; got %d tiles", len(got))
	}
	s := &Sim{}
	if g := s.GroundUnder(f.scene[:1]); g.Note == "" && g.State == "" {
		t.Error("a single node got no ground verdict at all")
	}
}
