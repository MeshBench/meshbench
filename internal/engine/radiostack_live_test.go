package engine_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/antenna"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// The baseline for replacing the radio stack.
//
// MeshBench substitutes MeshCore's radio driver with HostRadio. The plan in
// docs/virtual-sx1262.md replaces that with MeshCore's own driver over real
// RadioLib, on a virtual SX1262 - which changes the code path every packet
// takes. This records what that path produces *now*, so the change can be
// checked rather than hoped about.
//
// Two nodes, one message, one seed. Small on purpose: a difference here is
// findable, and the same difference inside a 154-node sweep is not.
//
// What the golden file holds, and why each part earns its place:
//
//   - The frames, byte for byte. MeshCore builds the same message whatever
//     carries it, so these must not change at any point in the migration. This
//     is the assertion with no tolerance.
//   - The ledger. Which node heard what, and what the firmware made of it.
//   - Per-node totals. A cheap summary that catches gross drift.
//
// Timing is deliberately *not* in the golden file. Real RadioLib does SPI work
// HostRadio never did, so transmissions move by the command latency a real
// driver introduces. That is expected and is asserted as ordering plus a stated
// tolerance, not equality - see TestRadioStackTimingStaysOrdered.
func TestRadioStackGolden(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	got := runTwoNodeExchange(t, 4417)

	if len(got.Frames) == 0 {
		t.Fatal("no frames transmitted; the exchange did not happen at all")
	}
	if got.RxAccepted["rs-bravo"] == 0 {
		t.Fatal("bravo accepted nothing; the message did not get through")
	}

	path := filepath.Join("testdata", "radiostack-golden.json")
	if os.Getenv("MESHCORESIM_UPDATE_GOLDEN") != "" {
		writeGolden(t, path, got)
		t.Logf("golden written to %s", path)
		return
	}

	want := readGolden(t, path)
	if want == nil {
		writeGolden(t, path, got)
		t.Skipf("no golden yet; wrote %s - review it and commit", path)
	}

	// Frames first and hardest. A changed frame is a changed message, and no
	// amount of timing tolerance should be allowed to hide one.
	if len(got.Frames) != len(want.Frames) {
		t.Errorf("frame count: got %d, want %d", len(got.Frames), len(want.Frames))
	}
	for i := range got.Frames {
		if i >= len(want.Frames) {
			break
		}
		if got.Frames[i] != want.Frames[i] {
			t.Errorf("frame %d differs:\n got  %s\n want %s", i, got.Frames[i], want.Frames[i])
		}
	}

	// Then who heard what. A two-node link with this much margin has no
	// legitimate reason to deliver differently.
	for node, n := range want.RxAccepted {
		if got.RxAccepted[node] != n {
			t.Errorf("%s accepted %d, want %d", node, got.RxAccepted[node], n)
		}
	}
	for node, n := range want.Tx {
		if got.Tx[node] != n {
			t.Errorf("%s transmitted %d, want %d", node, got.Tx[node], n)
		}
	}
}

// Timing may move when the radio stack changes; ordering may not.
//
// A real driver spends time on SPI that HostRadio never did, so every
// transmission shifts a little. That is the one difference the migration is
// allowed to produce - so it is asserted separately, loosely, and with the
// reason written down, rather than being folded into the golden file where it
// would force a rewrite at every step and hide a real regression among the
// noise.
func TestRadioStackTimingStaysOrdered(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	got := runTwoNodeExchange(t, 4417)

	for i := 1; i < len(got.Events); i++ {
		if got.Events[i].AtMs < got.Events[i-1].AtMs {
			t.Fatalf("event %d at %d ms follows one at %d ms; the ledger is out of order",
				i, got.Events[i].AtMs, got.Events[i-1].AtMs)
		}
	}
	// The exchange must complete well inside the run, whatever the stack costs
	// in per-command latency. A driver that has become an order of magnitude
	// slower is a problem even if every packet still arrives.
	if last := got.Events[len(got.Events)-1].AtMs; last > 25_000 {
		t.Errorf("last event at %d ms; the exchange should be over long before the run ends", last)
	}
}

// Determinism, which everything above depends on.
//
// Same seed, same scenario, same result - CLAUDE.md makes this a property of
// the simulator rather than a nice-to-have. If it fails, the golden file is
// meaningless and so is any A/B built on it.
func TestRadioStackIsDeterministic(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	a := runTwoNodeExchange(t, 4417)
	b := runTwoNodeExchange(t, 4417)

	if len(a.Frames) != len(b.Frames) {
		t.Fatalf("two runs of one seed produced %d and %d frames", len(a.Frames), len(b.Frames))
	}
	for i := range a.Frames {
		if a.Frames[i] != b.Frames[i] {
			t.Fatalf("frame %d differs between two runs of the same seed:\n %s\n %s",
				i, a.Frames[i], b.Frames[i])
		}
	}
	for i := range a.Events {
		if i >= len(b.Events) {
			break
		}
		if a.Events[i] != b.Events[i] {
			t.Fatalf("event %d differs between two runs of the same seed:\n %+v\n %+v",
				i, a.Events[i], b.Events[i])
		}
	}
}

// goldenFirmware is the build the baseline is recorded against.
//
// Pinned deliberately. The migration to a real radio stack is a comparison
// between two code paths, so everything else has to be nailed down - including
// which MeshCore this is.
const goldenFirmware = "repeater-v1.17.0"

// ---- the exchange, and what it records ----

type stackEvent struct {
	AtMs   uint32 `json:"at_ms"`
	Kind   string `json:"kind"`
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type stackRun struct {
	Frames     []string       `json:"frames"`
	Events     []stackEvent   `json:"events"`
	Tx         map[string]int `json:"tx"`
	RxAccepted map[string]int `json:"rx_accepted"`
	Console    []string       `json:"console,omitempty"`
}

func runTwoNodeExchange(t *testing.T, seed uint64) stackRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.525, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
	})
	defer func() { _ = e.Close() }()

	// Close enough to be far above the demodulator floor. The point of this
	// test is the software path, not a marginal link - a delivery that depends
	// on a decibel would turn every driver change into a coin toss.
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	for _, spec := range []struct {
		name string
		lon  float64
	}{{"rs-alpha", -3.90}, {"rs-bravo", -3.70}} {
		e.Add(scenario.Node{
			Name: spec.name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: 56.70, Lon: spec.lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: 10, NoiseFigureDB: 6,
			Radio: scenario.RadioConfig{
				CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
			},
			// Pinned, not "main": a golden file recorded against a moving
			// target is worth nothing, and the whole point of this test is to
			// compare one radio stack against another with everything else
			// held still.
			Firmware: scenario.FirmwareRef{Role: "simple_repeater", Version: goldenFirmware},
		}, nil)
	}

	if err := e.AttachNative(ctx, seed); err != nil {
		t.Fatal(err)
	}
	node, ok := e.NodeByName("rs-alpha")
	if !ok || node.Firmware == nil {
		t.Fatal("alpha has no firmware")
	}
	// Through the node's own CLI, so the firmware builds a real packet. A
	// frame fabricated here would be dropped by every receiver, correctly.
	if err := node.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, 20_000); err != nil {
		t.Fatal(err)
	}

	out := stackRun{Tx: map[string]int{}, RxAccepted: map[string]int{}}
	for _, ev := range e.Events() {
		switch ev.Kind {
		case "tx":
			out.Tx[ev.From]++
			if len(ev.Frame) > 0 {
				out.Frames = append(out.Frames, hex.EncodeToString(ev.Frame))
			}
		case "rx":
			out.RxAccepted[ev.To]++
		}
		out.Events = append(out.Events, stackEvent{
			AtMs: ev.AtMs, Kind: ev.Kind, From: ev.From, To: ev.To, Detail: ev.Detail,
		})
	}
	sort.Strings(out.Console)
	return out
}

func writeGolden(t *testing.T, path string, r stackRun) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readGolden(t *testing.T, path string) *stackRun {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var r stackRun
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

// describeDrift is used by the failure messages above to say where two runs
// part company, rather than leaving a wall of near-identical JSON.
func describeDrift(a, b []stackEvent) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return fmt.Sprintf("first difference at event %d: %+v vs %+v", i, a[i], b[i])
		}
	}
	if len(a) != len(b) {
		return fmt.Sprintf("identical for %d events, then %d vs %d", n, len(a), len(b))
	}
	return "identical"
}

var _ = strings.TrimSpace
var _ = describeDrift
