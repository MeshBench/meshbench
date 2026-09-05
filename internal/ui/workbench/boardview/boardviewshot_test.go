// Draw the board view and look at it.
//
// The same reasoning as the Hardware tab's own picture test: a panel, a table
// of verdicts and a wrapped quotation are three things that cannot be checked
// by asserting on numbers, and this project has let a smeared pill and a
// drifting column through a green build before.
//
// The screen bits are a real capture from an emulated T-Deck, borrowed from the
// node view's testdata, because a mock would prove the layout and nothing about
// the pipeline.
package boardview

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

func TestDrawTheBoardView(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bits, w, h := capturedTDeck(t)

	mono, mw, mh := capturedHeltec(t)

	healthy := func(st *state.NodeStat) *state.NodeStat {
		st.Radio.Boosted, st.Radio.GainReg = true, 0x96
		st.IRQReads, st.Spurious = 41, 0
		st.Radio.IRQFlags = 0x0002
		return st
	}
	cases := []struct {
		name  string
		tab   Tab
		scale int
		board string
		stat  func() *state.NodeStat
	}{
		// The state the window exists for: a board with something wrong.
		{"radio-faults", TabRadio, 0, "LilyGo_TDeck",
			func() *state.NodeStat { return tdeckStat(bits, w, h) }},
		// And one with nothing wrong, so a healthy board is not dressed up.
		{"radio-healthy", TabRadio, 0, "LilyGo_TDeck",
			func() *state.NodeStat { return healthy(tdeckStat(bits, w, h)) }},
		{"wiring", TabWiring, 0, "LilyGo_TDeck",
			func() *state.NodeStat { return tdeckStat(bits, w, h) }},
		{"wiring-2to1", TabWiring, 2, "LilyGo_TDeck",
			func() *state.NodeStat { return tdeckStat(bits, w, h) }},
		{"wiring-3to1", TabWiring, 3, "LilyGo_TDeck",
			func() *state.NodeStat { return tdeckStat(bits, w, h) }},
		// A mono panel on another board: the other half of the pixel path,
		// and the sizing rule going the other way.
		{"oled-2to1", TabWiring, 0, "LilyGo_T3S3_sx1262", func() *state.NodeStat {
			return &state.NodeStat{Name: "Deck", Board: "LilyGo_T3S3_sx1262",
				Backend: "emulated", Running: true,
				Screen: &state.Screen{Width: mw, Height: mh, BPP: 1, On: true, Bits: mono},
				Radio: state.RadioState{Reported: true, Boosted: true, GainReg: 0x96,
					TxPowerDBm: 20, Mode: 1, SF: 10, CR: 5, FreqHz: 869618000,
					BandwidthHz: 250000, IRQMask: 2}, IRQReads: 9}
		}},
		// The panel asleep, which is the firmware doing what it should.
		{"asleep", TabWiring, 0, "LilyGo_TDeck", func() *state.NodeStat {
			st := tdeckStat(bits, w, h)
			st.Screen.On = false
			return st
		}},
		// Not powered at all.
		{"stopped", TabWiring, 0, "LilyGo_TDeck", func() *state.NodeStat {
			return &state.NodeStat{Name: "Deck", Board: "LilyGo_TDeck",
				Backend: "emulated", Running: false, State: "stopped"}
		}},
		// A chip that has said nothing: the radio table has nothing to compare
		// and says so rather than inventing healthy rows.
		{"radio-silent", TabRadio, 0, "LilyGo_TDeck", func() *state.NodeStat {
			return &state.NodeStat{Name: "Deck", Board: "LilyGo_TDeck",
				Backend: "emulated", Running: true}
		}},
		// An nRF52 board under Renode, drawing its own panel. The picture is a
		// real capture off a Heltec T114 booting its published image, as the
		// T-Deck's is off a T-Deck: the point of these is that the window is
		// shown against what a board actually drew, and a board under the other
		// emulator has to be held to the same standard.
		{"renode-radio", TabRadio, 0, "Heltec_t114", func() *state.NodeStat {
			return t114Stat(t)
		}},
		{"renode-wiring", TabWiring, 0, "Heltec_t114", func() *state.NodeStat {
			return t114Stat(t)
		}},
		// A node on a host build, which has no board to check.
		{"no-board", TabRadio, 0, "", func() *state.NodeStat {
			return &state.NodeStat{Name: "Deck", Backend: "native", Running: true}
		}},
	}
	for _, c := range cases {
		st := c.stat()
		p := &Panel{Node: "Deck", Tab: c.tab, scale: c.scale}
		snap := &state.Snapshot{
			Stats: []state.NodeStat{*st},
			// The node's own row, so the slot's controls are drawn: a board
			// with no row correctly offers none, and a picture of that proves
			// only the guard.
			Nodes: []state.Node{{Name: "Deck", Kind: "companion",
				CardSlot: true, CardFitted: true}},
			// Both voices, so the strip draws one and offers the other.
			Outputs: []state.OutputPane{
				{Node: "Deck", Source: "serial", Total: 412, Lines: []string{
					"[  6494][E][Wire.cpp:137] setPins(): bus already initialized",
					"[  6604][E][vfs_api.cpp:105] open(): /littlefs/Messages_default.msgs",
					"[ 12038][I] RadioLib SX1262 begin() -> 0",
					"[ 12040][I] listen() entered - waiting on DIO1",
				}},
				{Node: "Deck", Source: "emulator", Total: 9, Lines: []string{
					"qemu: esp32s3 machine, 8 MB octal PSRAM",
					"sx1262: chip library loaded, noise seed 0x4f2a",
				}},
			},
		}
		width := 1180
		if c.board != "" {
			width = railWidth(boardOrTDeck(t, c.board), c.scale) + 700 + 260
		}
		img := uitest.RenderWidget(t, width, 720,
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return p.Draw(th, gtx, snap)
			})
		out := filepath.Join(dir, "boardview-"+c.name+".png")
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

// boardOrTDeck is the named board, for sizing the canvas.
func boardOrTDeck(t *testing.T, name string) hw.Board {
	t.Helper()
	b, err := hw.BoardByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// capturedHeltec reads the mono capture the node view keeps, as page bits.
func capturedHeltec(t *testing.T) ([]byte, int, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "nodeview", "testdata",
		"heltec_v3_screen.txt"))
	if err != nil {
		t.Fatalf("reading the captured mono screen: %v", err)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line != "" {
			rows = append(rows, line)
		}
	}
	h := len(rows)
	w := len(rows[0])
	bits := make([]byte, w*h/8)
	for y, row := range rows {
		for x := 0; x < w && x < len(row); x++ {
			if row[x] == '#' {
				bits[(y/8)*w+x] |= 1 << (y % 8)
			}
		}
	}
	return bits, w, h
}

// capturedTDeck reads the colour capture the node view keeps, as RGB565.
func capturedTDeck(t *testing.T) ([]byte, int, int) {
	t.Helper()
	return colourCapture(t, filepath.Join("..", "nodeview", "testdata",
		"tdeck_screen.png"))
}

// colourCapture reads a captured colour panel as the RGB565 a frame carries.
func colourCapture(t *testing.T, path string) ([]byte, int, int) {
	t.Helper()
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
func tdeckStat(bits []byte, w, h int) *state.NodeStat {
	return &state.NodeStat{
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

// Draw the panel's own window, which is the other half of "bigger": it takes
// whatever whole scale the space allows and says which one that was.
func TestDrawTheScreenWindow(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	bits, w, h := capturedTDeck(t)
	for _, c := range []struct {
		name string
		w, h int
	}{
		// Room for 1:1, for 2:1, and for 3:1 - the same window dragged.
		{"screen-window-small", 420, 360},
		{"screen-window-medium", 720, 560},
		{"screen-window-large", 1040, 800},
	} {
		st := tdeckStat(bits, w, h)
		sp := &ScreenPanel{Node: "Deck", view: &ScreenView{}}
		snap := &state.Snapshot{Stats: []state.NodeStat{*st}}
		img := uitest.RenderWidget(t, c.w, c.h,
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return sp.Draw(th, gtx, snap)
			})
		f, err := os.Create(filepath.Join(dir, "boardview-"+c.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}
}

// t114Stat is a Heltec T114 part way through a run under Renode: its panel
// drawing, its radio configured, its cell read.
//
// The capture is the firmware's own status screen - node name, frequency,
// spreading factor, bandwidth and coding rate - which is what makes it worth
// keeping: it is a picture of MeshCore's own arithmetic about the radio the
// simulation configured, not a mock of a panel.
func t114Stat(t *testing.T) *state.NodeStat {
	t.Helper()
	bits, w, h := capturedT114(t)
	// Named as the others are: the panel asks the snapshot for one node by
	// name, and a fixture that names it something else lands on a row nothing
	// reads - which draws as a node running no board at all.
	return &state.NodeStat{Name: "Deck", Board: "Heltec_t114",
		Backend: "emulated", Running: true, IRQReads: 22,
		Screen: &state.Screen{Width: w, Height: h, On: true, BPP: 16, Bits: bits},
		Radio: state.RadioState{Reported: true, Boosted: true, GainReg: 0x96,
			TxPowerDBm: 22, Mode: 1, SF: 8, CR: 5, FreqHz: 869618000,
			BandwidthHz: 62500, IRQMask: 2, IRQFlags: 2}}
}

// capturedT114 reads that capture, as RGB565.
func capturedT114(t *testing.T) ([]byte, int, int) {
	t.Helper()
	return colourCapture(t, filepath.Join("testdata", "t114_screen.png"))
}
