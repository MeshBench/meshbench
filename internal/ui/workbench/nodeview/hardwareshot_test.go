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

// Draw the Hardware tab and look at it.
//
// Green tests have let a smeared pill and an invisible layer through this
// project before, and a display is the one thing that cannot be checked by
// asserting on numbers: the questions are whether the panel is the shape the
// board's panel is, whether the picture inside it is the right way up, and
// whether the lamps and buttons are where somebody would look for them. So
// this renders to a file, and a person opens it.
//
// The screen bits are a real capture from an emulated Heltec V3, not a
// pattern: a mock would prove the layout and nothing about the pipeline.
func TestDrawTheHardwareTab(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	bits, w, h := loadCapturedScreen(t)
	cbits, cw, ch := loadCapturedColourScreen(t)

	cases := []struct {
		name  string
		board string
		stat  state.NodeStat
	}{
		{"heltec-v3-drawing", "Heltec_v3", state.NodeStat{
			Name: "node-1", Board: "Heltec_v3", Running: true,
			Screen: &state.Screen{Width: w, Height: h, BPP: 1, On: true, Bits: bits},
		}},
		{"heltec-v3-asleep", "Heltec_v3", state.NodeStat{
			Name: "node-1", Board: "Heltec_v3", Running: true,
			Screen: &state.Screen{Width: w, Height: h, BPP: 1, On: false, Bits: bits},
		}},
		{"heltec-v3-stopped", "Heltec_v3", state.NodeStat{
			Name: "node-1", Board: "Heltec_v3", Running: false,
		}},
		// A board with no screen at all: the tab still has something to say.
		{"generic-e22-no-screen", "Generic_E22_sx1262", state.NodeStat{
			Name: "node-1", Board: "Generic_E22_sx1262", Running: true,
		}},
		// A board that declares it has no button, which is a fact rather than
		// a gap.
		{"xiao-s3-no-button", "Xiao_S3", state.NodeStat{
			Name: "node-1", Board: "Xiao_S3", Running: true,
		}},
		// A colour panel on the radio's own SPI bus, three times the pixels
		// and a different shape - the layout has to hold for both.
		{"tdeck-colour", "LilyGo_TDeck", state.NodeStat{
			Name: "node-1", Board: "LilyGo_TDeck", Running: true,
			Screen: &state.Screen{Width: cw, Height: ch, BPP: 16, On: true, Bits: cbits},
		}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			snap := &state.Snapshot{Stats: []state.NodeStat{c.stat}}
			p := &WindowPanel{Node: "node-1", Kind: "simple_repeater"}
			p.Tab = TabHardware

			img := uitest.RenderWidget(t, 900, 460, func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return p.hardware(th, gtx, snap)
			})
			out := filepath.Join(dir, "hardware-"+c.name+".png")
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

// loadCapturedScreen reads a screen captured from a real emulated board.
//
// Stored as text - one character a pixel - because a person can read it in a
// diff, and a picture nobody can read is a picture nobody checks.
func loadCapturedScreen(t *testing.T) ([]byte, int, int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "heltec_v3_screen.txt"))
	if err != nil {
		t.Fatalf("reading the captured screen: %v", err)
	}
	var rows []string
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i == len(raw) || raw[i] == '\n' {
			if i > start {
				rows = append(rows, string(raw[start:i]))
			}
			start = i + 1
		}
	}
	if len(rows) == 0 {
		t.Fatal("the captured screen is empty")
	}
	h := len(rows)
	w := len(rows[0])
	if h%8 != 0 {
		t.Fatalf("a capture is whole pages: %d rows", h)
	}
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

// loadCapturedColourScreen reads a colour capture from a real emulated board.
//
// Stored as a PNG rather than as raw pixels: a hundred and fifty kilobytes of
// RGB565 in a repository is a file nobody will ever open, and this one can be
// looked at before it is trusted.
func loadCapturedColourScreen(t *testing.T) ([]byte, int, int) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "tdeck_screen.png"))
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
			out[i], out[i+1] = byte(v>>8), byte(v)
		}
	}
	return out, w, h
}

// The captures the picture test draws from are where it expects them.
//
// Separate, and with no env var in front of it, because the test above writes
// pictures for a person to look at and so is skipped unless somebody asked for
// them - which means a broken fixture path is invisible to CI. That is exactly
// what happened: the tab's test moved to this package and its testdata stayed
// behind in workbench, and every run went green for a fortnight because every
// run skipped.
//
// This one costs a few milliseconds and fails on the machine that moved them.
func TestTheCapturedScreensAreWhereTheTestLooks(t *testing.T) {
	bits, w, h := loadCapturedScreen(t)
	if w != 128 || h != 64 {
		t.Errorf("the Heltec capture is %dx%d, want 128x64", w, h)
	}
	if len(bits) != w*h/8 {
		t.Errorf("%d bytes for a %dx%d mono capture, want %d", len(bits), w, h, w*h/8)
	}
	cbits, cw, ch := loadCapturedColourScreen(t)
	if cw != 320 || ch != 240 {
		t.Errorf("the T-Deck capture is %dx%d, want 320x240", cw, ch)
	}
	if len(cbits) != cw*ch*2 {
		t.Errorf("%d bytes for a %dx%d RGB565 capture, want %d",
			len(cbits), cw, ch, cw*ch*2)
	}
}
