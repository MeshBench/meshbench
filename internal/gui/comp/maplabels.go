package comp

import (
	"image"
	"sort"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Labels that do not sit on top of each other.
//
// The old map drew every name where the node was and let them pile up, which
// at national scale turned the densest and most interesting part of the
// network into a grey smear. Thinning to every fourth name traded one problem
// for another: it hid names in empty countryside, where there was room, and
// still overlapped them in cities, where there was not.
//
// Placement is greedy over a priority order. Greedy is not optimal - label
// placement is NP-hard, and an optimal solver is not what a 16 millisecond
// frame is for - but it is stable, which matters more: the same network at the
// same camera places the same labels, so nothing flickers between frames.

// labeller places labels for one frame.
type labeller struct {
	taken []image.Rectangle
	// Max is how many labels will be placed before the rest are dropped, in
	// priority order. Not an arbitrary tidiness rule: each label is a shaping
	// and a draw, and at a full-window national scale the map was spending
	// 7ms a frame on names that a reader cannot take in anyway. Zero means the
	// default; a negative number means no limit, which is what a test that
	// wants every label asks for.
	Max int
}

// defaultMaxLabels is roughly what fits on a large window before the map is
// more text than map.
const defaultMaxLabels = 160

// candidate offsets, in the order they are tried. Right of the node first,
// because that is where a reader looks, then left, then above and below.
var labelSpots = [4]image.Point{
	{X: 8, Y: -7},
	{X: -8, Y: -7},
	{X: 0, Y: -20},
	{X: 0, Y: 6},
}

// place decides where each visible label goes.
//
// Returns the chosen top-left corner per point index. An index that is absent
// had nowhere to go, and is not drawn: a label that overlaps another is worse
// than no label, because it makes both unreadable rather than one absent.
func (l *labeller) place(pts []projected, sz image.Point, hover int,
	size func(int) image.Point) map[int]image.Point {

	l.taken = l.taken[:0]
	max := l.Max
	if max == 0 {
		max = defaultMaxLabels
	}
	out := make(map[int]image.Point, len(pts))
	for _, i := range labelOrder(pts, hover) {
		if max > 0 && len(l.taken) >= max {
			break
		}
		p := pts[i]
		if offscreen(p, sz) {
			continue
		}
		d := size(i)
		if d.X == 0 {
			continue
		}
		for _, spot := range labelSpots {
			at := image.Pt(int(p.x)+spot.X, int(p.y)+spot.Y)
			if spot.X < 0 {
				at.X -= d.X // right-aligned against the node
			}
			r := image.Rectangle{Min: at, Max: at.Add(d)}
			if !r.In(image.Rectangle{Max: sz}) || l.overlaps(r) {
				continue
			}
			l.taken = append(l.taken, r)
			out[i] = at
			break
		}
	}
	return out
}

func (l *labeller) overlaps(r image.Rectangle) bool {
	for _, o := range l.taken {
		if r.Overlaps(o) {
			return true
		}
	}
	return false
}

// labelOrder is who gets the space when two labels want it.
//
// Selection first, then whatever is under the pointer, then the kinds there
// are few of - a room server matters more than the four hundredth repeater -
// and then by index, which is stable across frames because the snapshot's node
// order is.
func labelOrder(pts []projected, hover int) []int {
	order := make([]int, len(pts))
	for i := range order {
		order[i] = i
	}
	rank := func(i int) int {
		switch {
		case pts[i].n.Selected:
			return 0
		case i == hover:
			return 1
		case kindOf(pts[i].n.Kind) != theme.SimpleRepeater:
			return 2
		}
		return 3
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := rank(order[a]), rank(order[b])
		if ra != rb {
			return ra < rb
		}
		return order[a] < order[b]
	})
	return order
}

// labelSizer measures label text once per name and remembers it.
//
// A name's size cannot change between frames, and measuring is a shaping call,
// so this is the difference between shaping every visible name every frame and
// shaping each new one once.
type labelSizer struct {
	size map[string]image.Point
}

func (s *labelSizer) measure(gtx layout.Context, t *theme.Theme, name string) image.Point {
	if s.size == nil {
		s.size = map[string]image.Point{}
	}
	if d, ok := s.size[name]; ok {
		return d
	}
	// Record rather than draw: the macro is discarded, and only the
	// dimensions are kept.
	m := op.Record(gtx.Ops)
	d := Text(t, t.Sz.Caption, t.P.Dim, name)(unbounded(gtx))
	m.Stop()
	s.size[name] = d.Size
	return d.Size
}

// unbounded is a context whose constraints do not force a size.
//
// A label laid out under the map's own constraints comes back the width of the
// map, because a minimum constraint is a demand rather than a permission and
// material.Label honours it. Measured that way every name is a rectangle the
// width of Scotland, nothing fits beside anything, and the map draws no labels
// at all - which is exactly what it did.
func unbounded(gtx layout.Context) layout.Context {
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = image.Pt(1<<14, 1<<14)
	return gtx
}
