package workbench

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
	if os.Getenv("MESHCORESIM_SHOTS") == "" {
		t.Skip("set MESHCORESIM_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHCORESIM_SHOTS")
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
			p := &nodeWindowPanel{node: "node-1", Kind: "simple_repeater"}
			p.tab = tabHardware

			img := renderWidget(t, 900, 460, func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
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

// renderWidget draws one widget into an image, with no window and no display.
func renderWidget(t *testing.T, w, h int, draw func(layout.Context, *theme.Theme) layout.Dimensions) image.Image {
	t.Helper()
	win, err := headless.NewWindow(w, h)
	if err != nil {
		t.Skipf("no GPU for headless rendering here: %v", err)
	}
	defer win.Release()

	th := theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(gofont.Collection())))
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(w, h)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}
	fillGround(gtx, th)
	draw(gtx, th)
	if err := win.Frame(gtx.Ops); err != nil {
		t.Fatalf("rendering: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("reading the frame back: %v", err)
	}
	return img
}

// fillGround paints the window's own background, so a capture is not a widget
// floating on whatever the buffer happened to hold.
func fillGround(gtx layout.Context, t *theme.Theme) {
	paint.FillShape(gtx.Ops, t.P.Ground, clip.Rect{Max: gtx.Constraints.Max}.Op())
}
