package engine_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// One arm of the protocol study, over a fixed set of seeds.
//
// The arm is whichever firmware MESHCORESIM_NATIVE points at, so the same
// scenario is replayed against each build without the harness knowing or caring
// what changed in it. That is what makes the control arm meaningful: two builds
// of identical source must produce identical numbers, and if they do not, no
// difference measured here means anything.
//
//	MESHCORESIM_LIVE=1 MESHCORESIM_NATIVE=~/msim/study/00-control-a \
//	STUDY_SEEDS=1,2,3,4,5 go test ./internal/engine/ -run TestStudyArm -v -timeout 900s
func TestStudyArm(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}

	seeds := []uint64{1, 2, 3}
	if s := os.Getenv("STUDY_SEEDS"); s != "" {
		seeds = nil
		for _, f := range strings.Split(s, ",") {
			v, err := strconv.ParseUint(strings.TrimSpace(f), 10, 64)
			if err != nil {
				t.Fatalf("bad seed %q: %v", f, err)
			}
			seeds = append(seeds, v)
		}
	}

	type run struct {
		tx, rx, dupes, delivered, reachable int
	}
	var runs []run

	for _, seed := range seeds {
		r := studyRun(t, seed)
		runs = append(runs, r)
		t.Logf("seed %d: tx=%d rx=%d dupes=%d delivered=%d/%d",
			seed, r.tx, r.rx, r.dupes, r.delivered, r.reachable)
	}

	// Reported as a mean, because a single seed says nothing: the measurement
	// floor on a scenario this size is around 20%, established by running one
	// configuration repeatedly rather than by assuming.
	var tx, rx, dupes, delivered, reachable float64
	for _, r := range runs {
		tx += float64(r.tx)
		rx += float64(r.rx)
		dupes += float64(r.dupes)
		delivered += float64(r.delivered)
		reachable += float64(r.reachable)
	}
	n := float64(len(runs))
	fmt.Printf("STUDY_RESULT seeds=%d tx=%.1f rx=%.1f dupes=%.1f delivered=%.1f reachable=%.1f\n",
		len(runs), tx/n, rx/n, dupes/n, delivered/n, reachable/n)
}

// studyRun is one seed: build the mesh, originate from one node, count what
// happened.
//
// The geometry is a fixed grid rather than an imported network, because the
// shipped fixtures do not exist yet. It is stated in the report as a deviation.
// What it has to provide is several nodes that can each hear a flood and each
// decide whether to relay it - which is what every idea in the study touches.
func studyRun(t *testing.T, seed uint64) (out struct {
	tx, rx, dupes, delivered, reachable int
}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 240e9)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
	})
	defer func() { _ = e.Close() }()

	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6},
		Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.525e6, BandwidthHz: 250e3,
		SpreadFactor: 10, CodingRate: 1}

	// A 4x4 grid at ~22 km spacing: each node hears its neighbours and not the
	// far side, so a flood has to be relayed and there is real choice about who
	// relays it. Spacing chosen so a hop is comfortably decodable and two hops
	// is not - the same basis as the flood test.
	const rows, cols = 4, 4
	const stepLat, stepLon = 0.20, 0.36
	names := make([]string, 0, rows*cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			name := fmt.Sprintf("g%d%d", r, c)
			names = append(names, name)
			e.Add(scenario.Node{
				Name: name, Kind: scenario.SimpleRepeater,
				Position: scenario.LatLon{
					Lat: 56.20 + float64(r)*stepLat,
					Lon: -4.60 + float64(c)*stepLon,
				},
				HeightAGLm: 15, Antenna: mast, TxPowerDBm: 20,
				NoiseFigureDB: 6, Radio: radio,
				Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.0"},
			}, nil)
		}
	}

	if err := e.AttachNative(ctx, seed); err != nil {
		t.Fatal(err)
	}

	// One corner originates. Everything measured is what the mesh did with it.
	src, ok := e.NodeByName("g00")
	if !ok || src.Firmware == nil {
		t.Fatal("no source node")
	}
	if err := src.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 120_000); err != nil {
		t.Fatal(err)
	}

	heard := map[string]int{}
	for _, ev := range e.Events() {
		switch ev.Kind {
		case "tx":
			out.tx++
		case "rx":
			out.rx++
			heard[ev.To]++
		}
	}
	for _, n := range heard {
		if n > 1 {
			out.dupes += n - 1
		}
	}
	out.delivered = len(heard)
	out.reachable = len(names) - 1
	return out
}
