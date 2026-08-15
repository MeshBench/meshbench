package comp

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Splitter is a rule between two panels that can be dragged to resize them.
//
// A rule that cannot be dragged is why a column ends up a width somebody chose
// once for everybody: the Runs table clipped its own seed and build columns at
// 460dp, and nothing in the window could give it another fifty.
//
// It reports movement rather than applying it. What a drag means - a column's
// width, a row's share of the height - belongs to whatever declared the
// arrangement, and the splitter has no idea which side of it is fixed.
type Splitter struct {
	// Vertical is a rule standing up, splitting left from right, so it is
	// dragged along X. Otherwise it lies down and is dragged along Y.
	Vertical bool

	// Delta is how far the pointer moved since the last frame, in pixels, and
	// is zero on any frame without a drag. Read it after Layout.
	Delta float32

	dragging bool
	last     f32.Point
}

// grab is how wide the invisible handle is either side of the drawn line. One
// pixel is honest about where the boundary is and impossible to hit.
const grab = 3

func (s *Splitter) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	s.Delta = 0

	sz := image.Pt(2*grab+1, gtx.Constraints.Max.Y)
	if !s.Vertical {
		sz = image.Pt(gtx.Constraints.Max.X, 2*grab+1)
	}

	// The hit area first, so the events are claimed before anything is drawn
	// over it, then the line down the middle of it.
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, s)
	if s.Vertical {
		pointer.CursorColResize.Add(gtx.Ops)
	} else {
		pointer.CursorRowResize.Add(gtx.Ops)
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: s,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			s.dragging, s.last = true, e.Position
		case pointer.Drag:
			if !s.dragging {
				continue
			}
			if s.Vertical {
				s.Delta += e.Position.X - s.last.X
			} else {
				s.Delta += e.Position.Y - s.last.Y
			}
			s.last = e.Position
		case pointer.Release, pointer.Cancel:
			s.dragging = false
		}
	}

	line := image.Pt(1, sz.Y)
	at := image.Pt(grab, 0)
	if !s.Vertical {
		line, at = image.Pt(sz.X, 1), image.Pt(0, grab)
	}
	colour := t.P.Rule
	if s.dragging {
		colour = t.P.Accent
	}
	defer op.Offset(at).Push(gtx.Ops).Pop()
	FillRect(gtx, line, colour)
	return layout.Dimensions{Size: sz}
}
