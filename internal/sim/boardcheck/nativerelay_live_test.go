package boardcheck

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Does anything relay?
//
// The flood row measures an emulated board in the middle of three nodes and no
// board passes it. This puts a native node there instead, on the identical
// geometry, so the answer separates two very different faults: firmware that
// does not relay under emulation, or a harness in which nothing would.
func TestANativeNodeInTheMiddleRelays(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
		UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()

	// The same three positions and powers, with the middle node native.
	for _, n := range probeGeometry("", nativePeerVersion) {
		if n.Name == "bc-under-test" {
			n.Firmware = scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}
		}
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4419); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	sender, ok := e.NodeByName("bc-sender")
	if !ok || sender.Firmware == nil {
		t.Fatal("the sender never came up")
	}
	if err := e.Run(ctx, e.NowMs()+90_000); err != nil {
		t.Fatalf("settling: %v", err)
	}
	_ = sender.Firmware.Bridge.Type([]byte("time 1754703600\r\n"))
	_ = e.Run(ctx, e.NowMs()+1_000)
	if err := sender.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
		t.Fatalf("commanding the sender: %v", err)
	}

	fromSender := map[uint64]bool{}
	spoke := 0
	_, relayed := waitForEvent(ctx, e, advertBudgetMs, func(ev engine.Event) bool {
		if ev.Kind != "tx" {
			return false
		}
		if ev.From == "bc-sender" {
			fromSender[ev.MessageID] = true
			return false
		}
		if ev.From != "bc-under-test" {
			return false
		}
		spoke++
		return fromSender[ev.MessageID]
	})
	t.Logf("native middle node: relayed=%v, own transmissions=%d, sender messages seen=%d",
		relayed, spoke, len(fromSender))
	if !relayed {
		t.Error("a native node in the middle did not relay either - the fault is not the emulation")
	}
}
