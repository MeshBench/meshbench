package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// Does the ridge break the links it should?
//
// The Lomond Hills sit between The Mysterons, in the Howe of Fife at 59 m, and
// the nodes in the valley south of them. A link across that ridge should not
// exist, and if one is drawn then either the DEM is not resolving the hill or
// the profile is not reaching the loss calculation.
func TestTheLomondRidgeBlocksWhatItShould(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip(err)
	}
	st, err := terrain.NewTileStore(filepath.Join(cache, "meshcoresim", "terrain"))
	if err != nil {
		t.Skip(err)
	}
	st.Zoom = terrain.DefaultZoom

	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	snap := store.Snapshot()
	at := map[string]state.Node{}
	for _, n := range snap.Nodes {
		at[n.Name] = n
	}
	from, ok := at["The Mysterons"]
	if !ok {
		t.Skip("The Mysterons is not in this fixture")
	}

	for _, name := range []string{"Bishop Hill ☀️🔋", "Leslie 🏘️", "Cadham Village 🏘️", "West Lomond ⛰️☀️"} {
		to, ok := at[name]
		if !ok {
			// Names carry emoji; find by prefix instead of failing.
			for n, v := range at {
				if len(n) >= 6 && len(name) >= 6 && n[:6] == name[:6] {
					to, ok = v, true
					break
				}
			}
		}
		if !ok {
			t.Logf("%-22s not in the fixture", name)
			continue
		}
		profileReport(t, st, from, to)
	}
}

func profileReport(t *testing.T, st *terrain.TileStore, from, to state.Node) {
	t.Helper()
	const steps = 300
	gA, _ := st.ElevationM(from.Lat, from.Lon)
	gB, _ := st.ElevationM(to.Lat, to.Lon)
	a, b := gA+from.HeightM, gB+to.HeightM

	worst, worstAt, worstH := 0.0, 0.0, 0.0
	first := true
	for i := 1; i < steps; i++ {
		f := float64(i) / steps
		h, ok := st.ElevationM(from.Lat+(to.Lat-from.Lat)*f, from.Lon+(to.Lon-from.Lon)*f)
		if !ok {
			continue
		}
		los := a + (b-a)*f
		if d := h - los; first || d > worst {
			worst, worstAt, worstH, first = d, f, h, false
		}
	}
	verdict := "clear"
	if worst > 0 {
		verdict = "BLOCKED"
	}
	t.Logf("%-24s ground %4.0f m -> %4.0f m | worst obstruction %+6.1f m (terrain %4.0f m at %2.0f%%) | %s",
		to.Name, gA, gB, worst, worstH, worstAt*100, verdict)
}

// What the engine says about the same pairs.
//
// The profile above proves the ridge is in the data. This asks whether the
// loss calculation sees it, which is the difference between "the terrain is
// missing" and "the terrain is there and something ignores it".
func TestTheEngineChargesForTheRidge(t *testing.T) {
	store := state.New(10)
	sim := &Sim{}
	Register(store, sim)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go store.Run(ctx)
	if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}

	idx := map[string]int{}
	for i, n := range sim.nodes {
		idx[n.Name] = i
	}
	find := func(prefix string) (int, bool) {
		for n, i := range idx {
			if len(n) >= len(prefix) && n[:len(prefix)] == prefix {
				return i, true
			}
		}
		return 0, false
	}
	a, ok := find("The Mysterons")
	if !ok {
		t.Skip("no Mysterons")
	}
	for _, prefix := range []string{"Bishop", "Leslie", "Cadham", "West Lom"} {
		b, ok := find(prefix)
		if !ok {
			continue
		}
		loss, ok := sim.eng.PathLossForTest(a, b)
		na, nb := sim.nodes[a], sim.nodes[b]
		km := geo.DistanceKm(na.Position.Lat, na.Position.Lon, nb.Position.Lat, nb.Position.Lon)
		fspl := terrain.FSPLdB(km, 869.618)
		t.Logf("%-12s %5.1f km | free space %6.1f dB | actual %6.1f dB | terrain charges %+6.1f dB (usable=%v)",
			prefix, km, fspl, loss, loss-fspl, ok)
	}

	// And what the map would draw.
	links := sim.links()
	drawn := map[[2]int]float64{}
	for _, l := range links {
		drawn[[2]int{l.A, l.B}] = l.MarginDB
		drawn[[2]int{l.B, l.A}] = l.MarginDB
	}
	t.Logf("links the map would draw: %d", len(links))
	for _, prefix := range []string{"Bishop", "Leslie", "Cadham"} {
		b, ok := find(prefix)
		if !ok {
			continue
		}
		margin, got := drawn[[2]int{a, b}]
		t.Logf("  %-12s drawn from The Mysterons: %v (margin %+.1f dB)", prefix, got, margin)
	}
}

// What excess loss does to the paths that should not close.
//
// Not a pass/fail: the right value is a calibration decision, and this is the
// evidence for making it. The Mysterons is in the Howe of Fife at 59 m and the
// ridge is between it and the valley beyond.
func TestExcessLossClosesTheRidgePaths(t *testing.T) {
	for _, db := range []float64{0, 5, 10, 15, 20} {
		store := state.New(10)
		sim := &Sim{excessLossDB: db}
		Register(store, sim)
		ctx, cancel := context.WithCancel(context.Background())
		go store.Run(ctx)
		if _, err := store.Do(ctx, "project.open", "../../../fixtures/fixture-fife-strict.json"); err != nil {
			cancel()
			t.Skip("no fixture:", err)
		}
		idx := map[string]int{}
		for i, n := range sim.nodes {
			idx[n.Name] = i
		}
		find := func(prefix string) (int, bool) {
			for n, i := range idx {
				if len(n) >= len(prefix) && n[:len(prefix)] == prefix {
					return i, true
				}
			}
			return 0, false
		}
		a, _ := find("The Mysterons")
		links := sim.links()
		margin := map[[2]int]float64{}
		for _, l := range links {
			margin[[2]int{l.A, l.B}] = l.MarginDB
			margin[[2]int{l.B, l.A}] = l.MarginDB
		}
		line := ""
		for _, prefix := range []string{"Leslie", "Cadham", "Bishop", "West Lom"} {
			b, ok := find(prefix)
			if !ok {
				continue
			}
			if m, drawn := margin[[2]int{a, b}]; drawn {
				line += fmt.Sprintf("  %s %+.1f", prefix, m)
			} else {
				line += fmt.Sprintf("  %s gone", prefix)
			}
		}
		t.Logf("excess %4.0f dB | links %4d |%s", db, len(links), line)
		cancel()
	}
}
