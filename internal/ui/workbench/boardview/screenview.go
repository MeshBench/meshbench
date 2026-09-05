// The board's panel, at a whole-number scale, and everything a person can do
// to it.
//
// Its own type because it is drawn in two windows: in the rail beside the
// tables, and on its own where it can be made as large as the desk allows. One
// widget in both places is what keeps the touchscreen working in both - the
// alternative is two copies of the mapping below, and the mapping is exactly
// the thing that fails silently.
package boardview

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// maxScale is how far up the steps go. Three is not a shortage: a 320-wide
// panel at 4:1 is wider than most of the window it lives in, and the popped
// out one takes whatever its own window allows anyway.
const maxScale = 3

// screenBudgetDp is the width the rail gives a panel before the operator asks
// for more. Chosen so the two panels in the fleet both land in one rail: a
// 128-wide OLED doubles to 256, and a 320-wide colour panel sits at 1:1.
const screenBudgetDp = 320

// ScreenView draws one board's panel and takes what is done to it.
//
// It holds no scale of its own. It did, so the popped-out window could read
// back what the last frame used - and a Flex lays its rigid children out before
// its flexed one, so the caption asked before the panel had chosen and printed
// 0:1. The scale is decided by whoever draws and passed in, which is also the
// number a press is divided by: one value, no order to get wrong.
type ScreenView struct {
	touchTag struct{}
	keyTag   struct{}
}

// fitScale is the largest whole-number scale at which a panel fits a box.
//
// Never zero: a panel too big for the space is drawn at 1:1 and clipped rather
// than shrunk. A smoothed panel is a picture of something the firmware did not
// draw, so the only honest sizes are multiples, and that gives a 320-wide
// panel a floor of 320 - it cannot be a thumbnail, and the rail is sized to
// the board rather than the board squeezed into the rail.
func fitScale(pw, ph, boxW, boxH int) int {
	if pw <= 0 || ph <= 0 {
		return 1
	}
	n := boxW / pw
	if v := boxH / ph; v < n {
		n = v
	}
	if n < 1 {
		n = 1
	}
	if n > 64 {
		n = 64
	}
	return n
}

// FitIn is the largest whole scale this panel fits the space in.
//
// Exported because the window that draws the panel on its own has to say which
// scale it drew at, and reading that back off the view would depend on the
// order a Flex lays its children out - which is rigids first, so the caption
// was drawn before the panel had chosen, and said 0:1.
func FitIn(sc *hw.Screen, gtx layout.Context) int {
	perDp := gtx.Dp(1)
	if perDp < 1 {
		perDp = 1
	}
	return fitScale(sc.WidthPx, sc.HeightPx,
		gtx.Constraints.Max.X/perDp, gtx.Constraints.Max.Y/perDp)
}

// boxFor is the panel's drawn size at a chosen scale, or at the rail's budget
// when asked for zero.
func boxFor(b hw.Board, want int) (scale, w, h int) {
	if !hasPanel(b) {
		return 1, 0, 0
	}
	sc := b.Hardware.Screen
	if sc == nil {
		return 1, 0, 0
	}
	scale = want
	if scale < 1 {
		scale = screenBudgetDp / sc.WidthPx
		if scale < 1 {
			scale = 1
		}
	}
	return scale, sc.WidthPx * scale, sc.HeightPx * scale
}

// railWidth is the rail's width for a board that may have no panel recorded.
func railWidth(b hw.Board, want int) int {
	if !hasPanel(b) || b.Hardware.Screen == nil {
		return 206
	}
	return railFor(b, want)
}

// railFor is the rail's width: whatever the panel needs, never less than the
// parts index wants.
func railFor(b hw.Board, want int) int {
	_, w, _ := boxFor(b, want)
	if w < 190 {
		w = 190
	}
	return w + 16
}

// Layout draws the panel at a scale and takes presses and keys on it.
//
// scale of zero means "as much as the box allows", which is what the popped
// out window passes.
func (v *ScreenView) Layout(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat, want int, onDo func(string, any), node string) layout.Dimensions {

	sc := b.Hardware.Screen
	if sc == nil {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "this board has no display")(gtx)
	}
	scale, w, h := boxFor(b, want)
	if want == 0 {
		scale = FitIn(sc, gtx)
		w, h = sc.WidthPx*scale, sc.HeightPx*scale
	}
	size := image.Pt(gtx.Dp(unit.Dp(w)), gtx.Dp(unit.Dp(h)))

	paint.FillShape(gtx.Ops, t.P.ScreenGround, clip.Rect{Max: size}.Op())
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	// The events are claimed before anything is drawn over the panel, because
	// this is the board somebody is pointing at.
	if hasTouch(b) {
		event.Op(gtx.Ops, &v.touchTag)
	}
	if hasKeys(b) {
		event.Op(gtx.Ops, &v.keyTag)
	}
	v.readTouch(gtx, b, scale, onDo, node)
	v.readKeys(gtx, b, onDo, node)

	if st != nil && st.Screen != nil && st.Screen.On {
		drawPixels(t, gtx, sc, st.Screen, scale)
	}
	comp.Border(gtx, size, 0, 1, t.P.Rule)
	return layout.Dimensions{Size: size}
}

// drawPixels paints what the firmware drew, one panel pixel per whole block.
func drawPixels(t *theme.Theme, gtx layout.Context, sc *hw.Screen,
	pic *state.Screen, scale int) {

	for y := 0; y < sc.HeightPx; y++ {
		for x := 0; x < sc.WidthPx; x++ {
			col, ok := pixel(t, pic, x, y)
			if !ok {
				continue
			}
			r := image.Rect(x*scale, y*scale, (x+1)*scale, (y+1)*scale)
			paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
		}
	}
}

// pixel is what to paint at one point, and whether to paint at all. A colour
// panel carries its own; drawing those in a theme colour would be inventing a
// picture the firmware did not send.
func pixel(t *theme.Theme, pic *state.Screen, x, y int) (color.NRGBA, bool) {
	if pic.BPP == 16 {
		r, g, b, ok := pic.At(x, y)
		if !ok || (r == 0 && g == 0 && b == 0) {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
	}
	if !pic.Lit(x, y) {
		return color.NRGBA{}, false
	}
	return t.P.ScreenLit, true
}

// readTouch turns a press on the drawn picture into the point the panel itself
// would report, and sends it.
//
// Two conversions, and leaving out either is silent. The scale is ours and not
// the board's, so a press at 2:1 is at twice the coordinate the firmware works
// in; and the panel may be mounted turned, which on the one board here with a
// touch layer it is. Either mistake puts the tap somewhere else and reads
// exactly like a touch layer that was never wired.
func (v *ScreenView) readTouch(gtx layout.Context, b hw.Board, scale int,
	onDo func(string, any), node string) {

	if !hasTouch(b) || onDo == nil {
		return
	}
	sc := b.Hardware.Screen
	touch := b.Hardware.PartsOfKind(hw.Touch)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &v.touchTag,
			Kinds:  pointer.Press | pointer.Release | pointer.Drag,
		})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		rx, ry, in := TouchPoint(sc, touch[0], scale,
			int(pe.Position.X), int(pe.Position.Y))
		if !in {
			continue
		}
		// Pressing the board is also what puts the keyboard on it, the way
		// picking a handheld up precedes typing on it.
		if pe.Kind == pointer.Press && hasKeys(b) {
			gtx.Execute(key.FocusCmd{Tag: &v.keyTag})
		}
		onDo("board.touch", map[string]any{
			"node": node, "x": rx, "y": ry, "down": pe.Kind != pointer.Release,
		})
	}
}

// TouchPoint is the drawn point turned into the panel's own, and whether it
// landed on the panel at all.
//
// Exported and pure because it is the part that breaks quietly, and a pure
// function is the only shape a test can hold still: the same place on the
// picture has to reach the same place on the panel at every scale.
func TouchPoint(sc *hw.Screen, touch hw.Part, scale, px, py int) (rx, ry int, ok bool) {
	if scale <= 0 || sc == nil {
		return 0, 0, false
	}
	x, y := px/scale, py/scale
	if x < 0 || y < 0 || x >= sc.WidthPx || y >= sc.HeightPx {
		return 0, 0, false
	}
	rx, ry = touch.RawPoint(x, y, sc.WidthPx, sc.HeightPx)
	return rx, ry, true
}

// readKeys sends what is typed to the board's own keyboard.
func (v *ScreenView) readKeys(gtx layout.Context, b hw.Board,
	onDo func(string, any), node string) {

	if !hasKeys(b) || onDo == nil {
		return
	}
	for {
		ev, ok := gtx.Event(key.Filter{Focus: &v.keyTag})
		if !ok {
			return
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		// board.key, not board.type: the board holds the last key pressed and
		// the firmware polls it, and the verb takes the characters as text.
		// The name matters more than it looks - a verb nothing registers is
		// refused into the void, and a keyboard that silently does nothing is
		// one somebody decides is broken.
		if txt := typedText(ke); txt != "" {
			onDo("board.key", map[string]any{"node": node, "text": txt})
		}
	}
}

// typedText is the character a key event carries, or "" for a key the board's
// own keyboard has no character for.
//
// Gio names a printable key by the character itself, so the common case is one
// rune. The few this board's keyboard does have a code for are named, because a
// handheld's keyboard sends a backspace and an enter like any other.
func typedText(ke key.Event) string {
	switch ke.Name {
	case key.NameReturn, key.NameEnter:
		return "\r"
	case key.NameDeleteBackward:
		return "\b"
	case key.NameSpace:
		return " "
	}
	n := string(ke.Name)
	if len([]rune(n)) != 1 {
		return ""
	}
	// Gio reports an unshifted name; the shift is in the modifiers.
	if ke.Modifiers.Contain(key.ModShift) {
		return n
	}
	return lowerASCII(n)
}

// lowerASCII lowers a single ASCII letter, which is what an unshifted key on a
// handheld sends.
func lowerASCII(s string) string {
	if len(s) == 1 && s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0] + 32)
	}
	return s
}

func hasTouch(b hw.Board) bool {
	return hasPanel(b) && len(b.Hardware.PartsOfKind(hw.Touch)) > 0
}

func hasKeys(b hw.Board) bool {
	return hasPanel(b) && len(b.Hardware.PartsOfKind(hw.Keys)) > 0
}
