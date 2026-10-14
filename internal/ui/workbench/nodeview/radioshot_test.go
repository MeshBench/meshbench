package nodeview

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// Draw the radio tab for the three answers a chip can give about its mask, and
// look at it.
//
// The figures are one repeater's from a scotland-strict cell: 1.16 enables no
// PreambleDetected for receive and 1.17 does. The third chip predates the count,
// and the row has to say so rather than read as a node that missed nothing.
func TestDrawTheRadioTab(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	radio := func(mask uint16) state.RadioState {
		return state.RadioState{
			Reported: true, GainReg: 0x96, Boosted: true, TxPowerDBm: 22,
			Mode: 1, SF: 8, CR: 8, FreqHz: 869618000, BandwidthHz: 62500,
			PreambleSyms: 32, IRQMask: mask, IRQFlags: 0, DIO1Mask: 0x0002,
			DIO1Reported: true,
		}
	}
	cases := []struct {
		name string
		stat state.NodeStat
	}{
		{"meshcore-1.16", state.NodeStat{
			Name: "node-1", Running: true, Radio: radio(0x0272),
			IRQReads: 2566, BusyReads: 224, BusyMs: 4478,
			IRQSuppressed: 14, SuppressedReported: true,
		}},
		{"meshcore-1.17", state.NodeStat{
			Name: "node-1", Running: true, Radio: radio(0x0276),
			IRQReads: 2501, BusyReads: 67, BusyMs: 4821,
			IRQSuppressed: 0, SuppressedReported: true,
		}},
		{"chip-without-the-count", state.NodeStat{
			Name: "node-1", Running: true, Radio: radio(0x0276),
			IRQReads: 2501, BusyReads: 67, BusyMs: 4821,
		}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := &state.Snapshot{Stats: []state.NodeStat{c.stat}}
			p := &WindowPanel{Node: "node-1", Kind: "simple_repeater"}
			p.Tab = TabRadio

			img := uitest.RenderWidget(t, 900, 760, func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return p.radio(th, gtx, snap)
			})
			out := filepath.Join(dir, "radio-"+c.name+".png")
			f, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", out)
		})
	}
}
