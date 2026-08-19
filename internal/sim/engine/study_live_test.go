package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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

	// A real network if one is named, and a grid otherwise.
	//
	// The grid exists because the shipped fixtures do not yet, and it is honest
	// about what it is: regular spacing, uniform power, no terrain variety. A
	// saved import has none of those conveniences, which is the point of running
	// both - an effect that only appears on a lattice is an artefact of the
	// lattice.
	if path := os.Getenv("STUDY_SCENARIO"); path != "" {
		names := studyLoadProject(t, e, path)
		if err := e.AttachNativeProgress(ctx, seed, nil); err != nil {
			t.Fatal(err)
		}
		return studyOriginate(t, ctx, e, names)
	}

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
	return studyOriginate(t, ctx, e, names)
}

// studyLoadProject adds the nodes of a saved project to the engine.
//
// Only the nodes: the saved areas and map position are for the workbench, and
// the radio configuration comes from the run rather than the file so every arm
// is measured on the same channel.
func studyLoadProject(t *testing.T, e *engine.Engine, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Nodes []scenario.Node `json:"nodes"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	// The preset the imported network actually runs: EU/UK (Narrow). Using the
	// synthetic SF10/250k here would measure a different radio from the one
	// these nodes were sited for.
	imported := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
		SpreadFactor: 8, CodingRate: 4}

	// A version is a per-role release tag, so each role is pinned separately.
	// A companion asked for repeater-v1.17.0 resolves nothing, and the failure
	// arrives minutes later as "runs no firmware".
	version := map[string]string{
		"simple_repeater":    "repeater-v1.17.0",
		"companion_radio":    "companion-v1.17.0",
		"simple_room_server": "room-server-v1.17.0",
	}

	var names []string
	for i := range doc.Nodes {
		n := doc.Nodes[i]
		if !n.Kind.RunsFirmware() {
			continue // observers and emitters carry no firmware to compare
		}
		n.Radio = imported
		role := n.Firmware.Role
		if role == "" {
			role = n.Kind.Application()
			n.Firmware.Role = role
		}
		if v, ok := version[role]; ok {
			n.Firmware.Version = v
		}
		// Never emulated here: an emulated node runs on wall time and would make
		// the arm unrepeatable, which is the one thing this harness needs.
		n.Firmware.Board = ""
		e.Add(n, nil)
		names = append(names, n.Name)
	}
	t.Logf("loaded %d nodes from %s", len(names), path)
	return names
}

// studyOriginate sends one advert and counts what the mesh did with it.
func studyOriginate(t *testing.T, ctx context.Context, e *engine.Engine,
	names []string) (out struct {
	tx, rx, dupes, delivered, reachable int
}) {
	t.Helper()
	if len(names) == 0 {
		t.Fatal("no nodes")
	}
	// Originate from a repeater, never a companion. A companion has no command
	// line - it speaks the companion protocol over its serial link - so typing
	// at one does nothing at all, with no error, which reads as a mesh that
	// dropped the first hop.
	var src *engine.Node
	for _, name := range names {
		n, ok := e.NodeByName(name)
		if ok && n.Firmware != nil && n.Spec.Kind == scenario.SimpleRepeater {
			src = n
			break
		}
	}
	if src == nil {
		t.Fatal("no repeater to originate from")
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
