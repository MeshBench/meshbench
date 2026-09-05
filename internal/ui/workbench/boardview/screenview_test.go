// The touchscreen has to keep working at every scale the panel can be drawn at.
//
// This is the regression a scale control invites, and it is silent both ways:
// a press that is not divided by the scale lands at a multiple of the right
// coordinate, and one that is not turned back out of the panel's mounting lands
// on the wrong axis. Either reads exactly like a touch layer that was never
// wired, which is a fault this project has spent weeks on before.
package boardview

import (
	"testing"

	"gioui.org/layout"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

func TestATapReachesTheSamePanelPointAtEveryScale(t *testing.T) {
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	sc := b.Hardware.Screen
	touch := b.Hardware.PartsOfKind(hw.Touch)
	if len(touch) == 0 {
		t.Fatal("the T-Deck is the board with a touch layer, and it has none")
	}

	// One place on the picture, pressed at each scale. What the press lands on
	// in the window differs by the scale; what the firmware is told must not.
	const px, py = 100, 60
	var want [2]int
	for _, scale := range []int{1, 2, 3} {
		rx, ry, ok := TouchPoint(sc, touch[0], scale, px*scale, py*scale)
		if !ok {
			t.Fatalf("%d:1 refused a press inside the panel", scale)
		}
		if scale == 1 {
			want = [2]int{rx, ry}
			continue
		}
		if rx != want[0] || ry != want[1] {
			t.Errorf("%d:1 reported panel %d,%d and 1:1 reported %d,%d - the "+
				"same place on the picture must be the same place on the panel",
				scale, rx, ry, want[0], want[1])
		}
	}

	// And the mounting is undone rather than ignored. This panel is fitted a
	// quarter turn, so the point the firmware reads is not the point pressed.
	if want[0] == px && want[1] == py {
		t.Errorf("the panel point %v is the picture point, so the quarter turn "+
			"this board's touch layer is mounted at was not undone", want)
	}
}

// A press beside the panel is not a press on the board.
func TestAPressOutsideThePanelIsNotATap(t *testing.T) {
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	sc := b.Hardware.Screen
	touch := b.Hardware.PartsOfKind(hw.Touch)[0]
	for _, c := range []struct {
		what          string
		scale, px, py int
	}{
		{"one past the right edge", 1, sc.WidthPx, 0},
		{"one past the bottom", 1, 0, sc.HeightPx},
		{"past the right edge at 2:1", 2, sc.WidthPx * 2, 0},
		{"a scale nothing was drawn at", 0, 5, 5},
	} {
		if _, _, ok := TouchPoint(sc, touch, c.scale, c.px, c.py); ok {
			t.Errorf("%s was accepted as a tap", c.what)
		}
	}
}

// The panel cannot be shrunk, and the rail is sized to it rather than the other
// way round.
func TestThePanelIsNeverSmallerThanItself(t *testing.T) {
	for _, name := range []string{"LilyGo_TDeck", "Heltec_v3"} {
		b, err := hw.BoardByName(name)
		if err != nil || b.Hardware == nil || b.Hardware.Screen == nil {
			continue
		}
		sc := b.Hardware.Screen
		scale, w, h := boxFor(b, 0)
		if scale < 1 {
			t.Errorf("%s: scale %d, which is not a whole multiple", name, scale)
		}
		if w < sc.WidthPx || h < sc.HeightPx {
			t.Errorf("%s: drawn %dx%d, smaller than the panel's own %dx%d - a "+
				"shrunk panel is a picture of something the firmware did not draw",
				name, w, h, sc.WidthPx, sc.HeightPx)
		}
		if got := railFor(b, 0); got < w {
			t.Errorf("%s: the rail is %d for a panel %d wide, so the panel is "+
				"being squeezed into the rail rather than the rail sized to it",
				name, got, w)
		}
	}
}

// fitScale never returns a scale that would draw a panel smaller than itself,
// however little room it is given.
func TestFitScaleNeverShrinks(t *testing.T) {
	for _, box := range []int{0, 1, 10, 100, 319} {
		if n := fitScale(320, 240, box, box); n < 1 {
			t.Errorf("a %dpx box gave scale %d; the floor is 1:1 and clipped", box, n)
		}
	}
	if n := fitScale(128, 64, 320, 320); n != 2 {
		t.Errorf("a 128-wide panel in a 320 budget came to %d:1, want 2:1", n)
	}
	if n := fitScale(320, 240, 320, 4000); n != 1 {
		t.Errorf("a 320-wide panel in a 320 budget came to %d:1, want 1:1", n)
	}
}

// The panel drawn at a scale is that many times its own size.
//
// The visible outcome rather than a field: the view used to keep the scale it
// last drew at so the popped-out window could read it back, and because a Flex
// lays rigid children out before its flexed one, the caption asked before the
// panel had chosen and printed 0:1. Nothing holds it now, so this checks the
// thing a reader can see - the panel really is n times as wide - which is also
// what says the number a press is divided by was the number used.
func TestThePanelIsDrawnAtTheScaleItWasAskedFor(t *testing.T) {
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	sc := b.Hardware.Screen
	for _, want := range []int{1, 2, 3} {
		var v ScreenView
		var got layout.Dimensions
		_, w, h := boxFor(b, want)
		uitest.RenderWidget(t, w+40, h+40,
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				got = v.Layout(th, gtx, b, nil, want, nil, "Deck")
				return got
			})
		if got.Size.X != sc.WidthPx*want || got.Size.Y != sc.HeightPx*want {
			t.Errorf("asked for %d:1 and drew %dx%d, want %dx%d", want,
				got.Size.X, got.Size.Y, sc.WidthPx*want, sc.HeightPx*want)
		}
	}
}

// FitIn never answers zero, however little room it is given: a zero scale is a
// caption reading "0:1" and a division that throws away every press.
func TestFitInNeverAnswersZero(t *testing.T) {
	b, err := hw.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatal(err)
	}
	sc := b.Hardware.Screen
	for _, size := range [][2]int{{1, 1}, {40, 40}, {319, 239}, {1040, 800}} {
		got := 0
		uitest.RenderWidget(t, size[0], size[1],
			func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				got = FitIn(sc, gtx)
				return layout.Dimensions{}
			})
		if got < 1 {
			t.Errorf("a %dx%d window fitted the panel at %d:1", size[0], size[1], got)
		}
	}
}

// A control that can only fail is not offered.
//
// The picture button photographs the panel, so a board with none showed a
// button that could do nothing but refuse - and an operator reports that as
// broken rather than as absent.
func TestNoPictureButtonWithoutAPanel(t *testing.T) {
	// Real boards for the positive case, and a constructed one for the
	// absence. Naming a board that has no panel today is a test that rots the
	// moment somebody transcribes one - which is what happened to the T114
	// the day after this was written.
	for _, name := range []string{"LilyGo_TDeck", "Heltec_v3"} {
		b, err := hw.BoardByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if !hasScreen(b) {
			t.Errorf("%s declares a screen and hasScreen says otherwise", name)
		}
	}
	if hasScreen(hw.Board{Name: "Unrecorded"}) {
		t.Error("a board nobody has transcribed offered a picture button")
	}
	// And a board recorded as carrying nothing is not the same as one nobody
	// has looked at, though neither has a panel to photograph.
	empty := hw.Board{Name: "Recorded", Hardware: &hw.Panel{}}
	if hasScreen(empty) {
		t.Error("a board that declares an empty panel offered a picture button")
	}
	if !hasPanel(empty) {
		t.Error("a board that declares an empty panel reads as one nobody has " +
			"looked at, which is a different fact")
	}
}
