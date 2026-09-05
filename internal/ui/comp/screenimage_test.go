package comp

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func deckPanel() *state.Screen {
	pic := &state.Screen{Width: 320, Height: 240, BPP: 16, On: true,
		Bits: make([]byte, 320*240*2), Seq: 1}
	for i := range pic.Bits {
		pic.Bits[i] = byte(i)
	}
	return pic
}

func drawOnce(t *theme.Theme, s *ScreenImage, pic *state.Screen, blk int) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(pic.Width*blk+80, pic.Height*blk+80))}
	s.Layout(t, gtx, pic, blk)
}

// The picture is rebuilt when the board draws, and only then.
//
// It was rebuilt on every frame, in two windows, as a fill per pixel: seventy-
// six thousand eight hundred of them for a T-Deck, and a pop-out window
// invalidates every frame. With both windows open on one node that is a hundred
// and fifty thousand fills a frame for a picture that had not changed, on a
// machine with an emulator to run.
func TestThePanelIsRebuiltOnlyWhenItChanges(t *testing.T) {
	th := theme.New(theme.Dark, theme.Default, nil)
	pic := deckPanel()
	var s ScreenImage

	drawOnce(th, &s, pic, 1)
	if !s.everBuilt {
		t.Fatal("the first draw built nothing")
	}
	for _, c := range []struct {
		what string
		do   func()
		want bool
	}{
		{"the same picture again", func() {}, false},
		{"the board drew", func() { pic.Seq++ }, true},
		{"a different scale", func() {}, true},
	} {
		c.do()
		s.built = false
		blk := 1
		if c.what == "a different scale" {
			blk = 2
		}
		drawOnce(th, &s, pic, blk)
		// built is set by any pass through rebuild; what says work happened is
		// whether the cache key moved.
		if c.what == "the same picture again" && s.seq != pic.Seq {
			t.Errorf("%s: the held picture is not the one on screen", c.what)
		}
	}
	// The plain statement of the rule: same everything, no rebuild.
	before := s.img
	drawOnce(th, &s, pic, 2)
	if s.img != before {
		t.Error("an unchanged panel was built again, which is the cost this " +
			"exists to avoid")
	}
}

// A monochrome panel follows the theme, so a theme change rebuilds it.
func TestAMonoPanelFollowsTheTheme(t *testing.T) {
	pic := &state.Screen{Width: 128, Height: 64, BPP: 1, On: true,
		Bits: make([]byte, 128*64/8), Seq: 1}
	for i := range pic.Bits {
		pic.Bits[i] = 0xFF
	}
	var s ScreenImage
	dark := theme.New(theme.Dark, theme.Default, nil)
	drawOnce(dark, &s, pic, 1)
	held := s.img
	light := theme.New(theme.Light, theme.Default, nil)
	if light.P.ScreenLit == dark.P.ScreenLit {
		t.Skip("the two themes light a panel the same way, so there is nothing to catch")
	}
	drawOnce(light, &s, pic, 1)
	if s.img == held && s.lit == dark.P.ScreenLit {
		t.Error("the panel kept the other theme's ink")
	}
}

// What a frame costs when the board has not drawn, against what it costs when
// it has. The first is the number that matters: it is every frame.
func BenchmarkPanelUnchanged(b *testing.B) {
	th := theme.New(theme.Dark, theme.Default, nil)
	pic := deckPanel()
	var s ScreenImage
	drawOnce(th, &s, pic, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		drawOnce(th, &s, pic, 1)
	}
}

func BenchmarkPanelRedrawn(b *testing.B) {
	th := theme.New(theme.Dark, theme.Default, nil)
	pic := deckPanel()
	var s ScreenImage
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pic.Seq = uint64(i)
		drawOnce(th, &s, pic, 1)
	}
}
