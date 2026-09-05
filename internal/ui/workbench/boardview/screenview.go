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
// Nothing mutable may go in here.
//
// Two windows share one of these on purpose - the board view and the screen it
// pops out - and they are two frame loops on two goroutines drawing at two
// scales. The tags below are addresses and nothing else, which is what makes
// sharing safe; a cached picture was put here once and the pair of windows
// crashed on the first frame they were both open, one reallocating the buffer
// while the other wrote into it. Anything a drawing keeps belongs to the
// drawing: pass it in, as Layout takes its image.
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
// pic is the caller's image, not the view's, and that is the whole of why it is
// a parameter rather than a field.
//
// The popped-out screen window shares this view with the board view it came
// from, deliberately - the touch mapping and the key focus are one set of facts
// and two copies would be two chances to divide by the wrong number. But the
// two windows are two frame loops on two goroutines drawing at two scales, so a
// cached image inside the view is one buffer being reallocated by one of them
// while the other writes into it. That crashed on the first frame after both
// were open.
func (v *ScreenView) Layout(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat, want int, onDo func(string, any), node string,
	pic *comp.ScreenImage) layout.Dimensions {

	sc := b.Hardware.Screen
	if sc == nil {
		return comp.Text(t, t.Sz.Caption, t.P.Faint, "this board has no display")(gtx)
	}
	scale, _, _ := boxFor(b, want)
	if want == 0 {
		scale = FitIn(sc, gtx)
	}
	// One panel pixel is a whole number of framebuffer pixels, and the box is
	// derived from that rather than the other way round.
	//
	// The box used to be a size in dp and the picture was painted in raw
	// pixels, which agree on a screen at 100% and on no other: dragged to a
	// display at 200% the box doubled and the picture did not, so the board
	// drew into the top-left quarter of its own frame. Deriving the box from
	// the block makes the two impossible to disagree, and keeps the panel the
	// same physical size on both screens - which is what a person moving a
	// window between them expects.
	blk := gtx.Dp(unit.Dp(scale))
	if blk < 1 {
		blk = 1
	}
	size := image.Pt(sc.WidthPx*blk, sc.HeightPx*blk)

	paint.FillShape(gtx.Ops, t.P.ScreenGround, clip.Rect{Max: size}.Op())
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	// One pointer area over the panel, and only one.
	//
	// There were two for a day, a touch layer's and a keyboard's, and the
	// keyboard's was registered second and so sat on top: it asked for presses
	// only, so it swallowed every press and left the drags and releases to the
	// touch layer underneath. A clean tap then did nothing at all and a tap
	// that slid a pixel worked, because a drag carries down as surely as a
	// press does - which is what "hit and miss" was.
	//
	// The touch layer owns the area where there is one, because it is the part
	// that needs a position; the keyboard only needs to know the board was
	// pressed, and gets that from the same reader.
	switch {
	case hasTouch(b):
		event.Op(gtx.Ops, &v.touchTag)
	case hasKeys(b):
		event.Op(gtx.Ops, &v.keyTag)
	}
	v.readTouch(gtx, b, blk, onDo, node)
	v.takeFocus(gtx, b)
	v.readKeys(gtx, b, onDo, node)

	if st != nil && st.Screen != nil && st.Screen.On {
		pic.Layout(t, gtx, st.Screen, blk)
	}
	comp.Border(gtx, size, 0, 1, t.P.Rule)
	return layout.Dimensions{Size: size}
}

// readTouch turns a press on the drawn picture into the point the panel itself
// would report, and sends it.
//
// Two conversions, and leaving out either is silent. The scale is ours and not
// the board's, so a press at 2:1 is at twice the coordinate the firmware works
// in; and the panel may be mounted turned, which on the one board here with a
// touch layer it is. Either mistake puts the tap somewhere else and reads
// exactly like a touch layer that was never wired.
// takeFocus puts the keyboard on the board when the board is pressed, the way
// picking a handheld up precedes typing on it.
//
// Only for a board with a keyboard and no touch layer. Where there is a touch
// layer it owns the pointer area - two areas over one panel is what made a
// clean tap do nothing - and the touch reader asks for the focus itself.
func (v *ScreenView) takeFocus(gtx layout.Context, b hw.Board) {
	if !hasKeys(b) || hasTouch(b) {
		return
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &v.keyTag, Kinds: pointer.Press,
		})
		if !ok {
			return
		}
		if pe, ok := ev.(pointer.Event); ok && pe.Kind == pointer.Press {
			gtx.Execute(key.FocusCmd{Tag: &v.keyTag})
		}
	}
}

// readTouch turns presses on the drawn panel into points on the board's own.
//
// blk rather than the scale: a pointer arrives in framebuffer pixels, so the
// number to divide by is how many of those one panel pixel occupies. Dividing
// by the scale worked at 100% and put every tap at twice its true coordinate
// on a display at 200%.
func (v *ScreenView) readTouch(gtx layout.Context, b hw.Board, blk int,
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
		rx, ry, in := TouchPoint(sc, touch[0], blk,
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
func TouchPoint(sc *hw.Screen, touch hw.Part, blk, px, py int) (rx, ry int, ok bool) {
	if blk <= 0 || sc == nil {
		return 0, 0, false
	}
	x, y := px/blk, py/blk
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
		// Both filters, and both kinds of event. The focus filter is what makes
		// the tag focusable at all: without it the panel took focus on a press
		// and then received nothing, which is a keyboard that looks wired and
		// is not. And typed characters arrive as an edit event - a key event
		// carries a name, and deriving a character from the name gets the
		// letters and misses everything a layout decides, which is most of what
		// somebody types.
		ev, ok := gtx.Event(
			key.FocusFilter{Target: &v.keyTag},
			key.Filter{Focus: &v.keyTag},
		)
		if !ok {
			return
		}
		// board.key, not board.type: the board holds the last key pressed and
		// the firmware polls it, and the verb takes the characters as text.
		// The name matters more than it looks - a verb nothing registers is
		// refused into the void, and a keyboard that silently does nothing is
		// one somebody decides is broken.
		switch e := ev.(type) {
		case key.EditEvent:
			if e.Text != "" {
				onDo("board.key", map[string]any{"node": node, "text": e.Text})
			}
		case key.Event:
			if e.State != key.Press {
				continue
			}
			// Only the keys that carry no character. Gio sends both a key
			// event and an edit event for a printable key, so taking the
			// character from both put every letter in twice - which is a
			// keyboard that works and cannot be used.
			if txt := namedKey(e); txt != "" {
				onDo("board.key", map[string]any{"node": node, "text": txt})
			}
		}
	}
}

// namedKey is the byte a key with no character of its own stands for, or ""
// for every key that has one.
//
// Backspace and return are what a field somebody is typing a name into cannot
// do without, and neither arrives as text. Everything that does arrive as text
// is left to the edit event, because a printable key produces both and taking
// it from both types it twice.
//
// Gio names a printable key by the character itself, so the common case is one
// rune. The few this board's keyboard does have a code for are named, because a
// handheld's keyboard sends a backspace and an enter like any other.
func namedKey(ke key.Event) string {
	switch ke.Name {
	case key.NameReturn, key.NameEnter:
		return "\r"
	case key.NameDeleteBackward:
		return "\b"
	}
	return ""
}

// hasScreen reports whether this board has a panel to draw or photograph.
func hasScreen(b hw.Board) bool { return hasPanel(b) && b.Hardware.Screen != nil }

func hasTouch(b hw.Board) bool {
	return hasPanel(b) && len(b.Hardware.PartsOfKind(hw.Touch)) > 0
}

func hasKeys(b hw.Board) bool {
	return hasPanel(b) && len(b.Hardware.PartsOfKind(hw.Keys)) > 0
}
