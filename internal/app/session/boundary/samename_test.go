package boundary

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	worldb "github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Two places with one name are two choices, and the one picked is the one used.
//
// A search for "Perth" returns Perth in Western Australia and Perth in
// Scotland. The chooser listed both as "Perth", and accept, matching on that
// name, took the first whichever row was pressed - so somebody importing a
// Scottish mesh got a study area on the other side of the world.
func TestPickingTheSecondPerthAcceptsThatPerth(t *testing.T) {
	st := state.New(10)
	s := &session.Sim{}
	session.Register(st, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	square := func(lat, lon float64) []scenario.Boundary {
		return []scenario.Boundary{{Name: "Perth", Rings: []scenario.Ring{{
			{Lat: lat, Lon: lon}, {Lat: lat + 0.2, Lon: lon},
			{Lat: lat + 0.2, Lon: lon + 0.2}, {Lat: lat, Lon: lon + 0.2}, {Lat: lat, Lon: lon},
		}}}}
	}
	s.SetFoundAreas([]worldb.Found{
		{Name: "Perth", DisplayName: "Perth, Western Australia, 6000, Australia",
			Boundaries: square(-32.0, 115.8)},
		{Name: "Perth", DisplayName: "Perth, Perth and Kinross, Alba / Scotland, United Kingdom",
			Boundaries: square(56.3, -3.5)},
	})
	if _, err := st.Do(ctx, "boundary.accept", map[string]any{
		"name": "Perth, Perth and Kinross, Alba / Scotland, United Kingdom"}); err != nil {
		t.Fatal(err)
	}
	got := s.Areas()
	if len(got) != 1 || len(got[0].Rings) == 0 || got[0].Rings[0][0].Lat < 0 {
		t.Fatalf("picking Perth in Scotland accepted %v", got)
	}
}
