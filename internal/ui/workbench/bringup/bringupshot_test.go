// Draw the Bring-up window and look at it.
//
// The same reasoning as the Hardware tab's own picture test: a panel, a table
// of verdicts and a wrapped quotation are three things that cannot be checked
// by asserting on numbers, and this project has let a smeared pill and a
// drifting column through a green build before.
//
// The screen bits are a real capture from an emulated T-Deck, borrowed from the
// node view's testdata, because a mock would prove the layout and nothing about
// the pipeline.
package bringup

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

func TestDrawTheBringUpWindow(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bits, w, h := capturedTDeck(t)

	cases := []struct {
		name  string
		tab   Tab
		scale int
	}{
		{"radio", TabRadio, 0},
		{"wiring", TabWiring, 0},
		{"wiring-2to1", TabWiring, 2},
	}
	for _, c := range cases {
		st := tdeckStat(bits, w, h)
		p := &Panel{Node: "Deck", Tab: c.tab, scale: c.scale}
		snap := &state.Snapshot{Stats: []state.NodeStat{st}}
		img := uitest.RenderWidget(t, railFor(tdeck(t), c.scale)+700+260, 720,
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return p.Draw(th, gtx, snap)
			})
		out := filepath.Join(dir, "bringup-"+c.name+".png")
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
		t.Log("wrote", out)
	}
}

// capturedTDeck reads the colour capture the node view keeps, as RGB565.
func capturedTDeck(t *testing.T) ([]byte, int, int) {
	t.Helper()
	path := filepath.Join("..", "nodeview", "testdata", "tdeck_screen.png")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading the captured colour screen: %v", err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decoding it: %v", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			v := uint16(r>>11)<<11 | uint16(g>>10)<<5 | uint16(bl>>11)
			i := (y*w + x) * 2
			out[i], out[i+1] = byte(v), byte(v>>8)
		}
	}
	return out, w, h
}

// tdeckStat is a T-Deck part way through a run: drawing, receiving, and with
// one thing wrong that the window is meant to find.
func tdeckStat(bits []byte, w, h int) state.NodeStat {
	return state.NodeStat{
		Name: "Deck", Board: "LilyGo_TDeck", Backend: "emulated",
		Running: true, State: "running", Firmware: "v1.17.1",
		Screen: &state.Screen{Width: w, Height: h, BPP: 16, On: true, Bits: bits},
		// Read once and then not again, which is what a receive path that is
		// not being woken looks like from here.
		IRQReads: 0, Spurious: 3,
		Radio: state.RadioState{
			Reported: true, GainReg: 0x94, Boosted: false, TxPowerDBm: 22,
			Mode: 1, SF: 10, CR: 5, FreqHz: 869618000, BandwidthHz: 250000,
			IRQMask: 0x0002, IRQFlags: 0x0000,
		},
	}
}

// tdeck is the board the pictures are drawn for.
func tdeck(t *testing.T) hw.Board {
	t.Helper()
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
