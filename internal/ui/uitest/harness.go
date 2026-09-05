// Driving one panel through real frames, for a test.
//
// A panel is a struct with a Draw that takes a theme, a layout context and a
// snapshot, so a test can lay one out for real - clicks, typing, the router and
// all - without a window. That is how the interface is tested here, and it
// worked while every panel lived in one package.
//
// It is its own package because the panels no longer do. A harness in a _test.go
// cannot cross a package boundary, and the alternative was a copy per package,
// which is a harness that drifts.
package uitest

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/gpu/headless"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

type Harness struct {
	draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	th   *theme.Theme
	R    input.Router
	ops  op.Ops
	Size image.Point
	Snap *state.Snapshot
}

func New(draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions,
	snap *state.Snapshot) *Harness {
	return &Harness{
		draw: draw,
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(brandfont.Collection()))),
		Size: image.Pt(1200, 800),
		Snap: snap,
	}
}

func (h *Harness) Frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.Size),
		Source:      h.R.Source(),
	}
	h.draw(h.th, gtx, h.Snap)
	h.R.Frame(&h.ops)
}

func (h *Harness) Click(at f32.Point) {
	h.R.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: pointer.ButtonPrimary},
	)
	h.Frame()
}

// Focused reports whether a tag holds the keyboard, which is the question a
// test of anything typed has to answer before it types.
func (h *Harness) Focused(tag event.Tag) bool {
	return h.R.Source().Focused(tag)
}

func (h *Harness) TypeText(s string) {
	h.R.Queue(key.EditEvent{Text: s})
	h.Frame()
}

// TypeOn types as a keyboard does: a key event and an edit event for every
// printable character, in that order, then the release.
//
// TypeText sends only the edit event, which is the half a text field reads -
// and a widget that also reads the key event is one this harness could not
// tell apart from a correct one. The board's keyboard did exactly that and put
// every letter in twice, and the test that was meant to cover it typed through
// TypeText and passed.
func (h *Harness) TypeOn(s string) {
	for _, r := range s {
		name := key.Name(strings.ToUpper(string(r)))
		h.R.Queue(
			key.Event{Name: name, State: key.Press},
			key.EditEvent{Text: string(r)},
			key.Event{Name: name, State: key.Release},
		)
		h.Frame()
	}
}

// Snapshot is a network with something in every list, so a panel that
// draws its controls only when it has data draws them.
func Snapshot() *state.Snapshot {
	return &state.Snapshot{
		Nodes: []state.Node{
			{Name: "Abernethy Repeater", Kind: "repeater", Lat: 56.3, Lon: -3.3, Selected: true},
			{Name: "Bishop Hill", Kind: "repeater", Lat: 56.2, Lon: -3.2},
			{Name: "AngusOutlaw1", Kind: "companion", Lat: 56.5, Lon: -3.0},
		},
		Stats: []state.NodeStat{
			{Name: "Abernethy Repeater", Backend: "native", Running: true, RSSBytes: 4 << 20},
			{Name: "Bishop Hill", Backend: "native", Running: true, RSSBytes: 4 << 20},
		},
	}
}

// pressAlong clicks every few pixels across a row, so a button is found by
// being there rather than by a coordinate written down in advance.
func (h *Harness) PressAlong(y float32) {
	for x := float32(8); x < float32(h.Size.X); x += 12 {
		h.Click(f32.Pt(x, y))
	}
}

// brandFaces is the collection every shaper in the application is built from:
// the three faces the identity is set in, then the machine's colour emoji.
//
// Gio's own faces are not in it. They were the default and nothing chose them;
// leaving them in means a style that names no typeface gets Go Sans while one
// that names Inter gets Inter, and the interface is then set in two families
// nobody picked.
func BrandFaces() []font.FontFace {
	return withEmoji(brandfont.Collection())
}

func withEmoji(base []font.FontFace) []font.FontFace {
	// The bundle carries the font beside the binary, so emoji in node names
	// do not depend on what the machine happens to have installed.
	paths := []string{
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	}
	if exe, err := os.Executable(); err == nil {
		paths = append([]string{
			filepath.Join(filepath.Dir(exe), "fonts", "NotoColorEmoji.ttf"),
		}, paths...)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if faces, err := opentype.ParseCollection(b); err == nil {
			return append(base, faces...)
		}
	}
	return base
}

// RenderAt is the same at a display scale, for the faults that only exist on
// one.
//
// A widget whose box is measured in dp and whose content is painted in
// framebuffer pixels is exact at 100% and wrong everywhere else, and every
// harness here ran at 100% - so the class was untestable until a board panel
// was dragged onto a 4K screen and drew a quarter size.
//
// w and h stay framebuffer pixels, as they are everywhere else, so the same
// window at 2x is the same numbers with half the room in dp.
func RenderAt(t *testing.T, w, h int, pxPerDp float32,
	draw func(layout.Context, *theme.Theme) layout.Dimensions) image.Image {
	t.Helper()
	return renderMode(t, w, h, pxPerDp, theme.Dark, draw)
}

// renderMode is the same on a chosen ground, for the pictures that are about
// the ground itself.
func RenderMode(t *testing.T, w, h int, mode theme.Mode,
	draw func(layout.Context, *theme.Theme) layout.Dimensions) image.Image {
	t.Helper()
	return renderMode(t, w, h, 1, mode, draw)
}

func renderMode(t *testing.T, w, h int, pxPerDp float32, mode theme.Mode,
	draw func(layout.Context, *theme.Theme) layout.Dimensions) image.Image {
	t.Helper()
	win, err := headless.NewWindow(w, h)
	if err != nil {
		t.Skipf("no GPU for headless rendering here: %v", err)
	}
	defer win.Release()

	th := theme.New(mode, theme.Default,
		text.NewShaper(text.WithCollection(BrandFaces())))
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(w, h)),
		Metric:      unit.Metric{PxPerDp: pxPerDp, PxPerSp: pxPerDp},
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

// hover moves the pointer over a point and lets the frame settle.
//
// Twice, deliberately: the panels put their footer above the table in the flex
// so the footer is measured first, and the row under the pointer is therefore
// named on the frame after the one that found it.
func (h *Harness) Hover(at f32.Point) {
	for i := 0; i < 2; i++ {
		h.R.Queue(pointer.Event{Kind: pointer.Move, Position: at})
		h.Frame()
	}
}

// renderWidget draws one widget into an image, with no window and no display.
func RenderWidget(t *testing.T, w, h int, draw func(layout.Context, *theme.Theme) layout.Dimensions) image.Image {
	t.Helper()
	return RenderMode(t, w, h, theme.Dark, draw)
}
