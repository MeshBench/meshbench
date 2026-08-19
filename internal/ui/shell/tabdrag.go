// Dragging a tab from one region to another.
//
// Tabs without dragging are half a dock: they say a region can hold several
// panels and give no way to decide which region a panel is in. A press that
// moves picks the tab up, the region under the pointer lights, and letting go
// puts the panel there.
//
// The drop test needs each region's rectangle, and Gio hands a widget its
// size but never its position. They are accumulated here instead, from the
// sizes each region reported as it drew: within a row the columns follow one
// another, and the rows follow one another down the body, so the sizes are
// enough to place every one of them. Last frame's geometry decides this
// frame's drop, which is exact by the time anybody has dragged anywhere.
package shell

import (
	"image"

	"gioui.org/f32"
)

// tabDrag is a tab that has been picked up.
type tabDrag struct {
	Name string
	From regionRef
	// At is where the pointer is, in window coordinates.
	At f32.Point
	// Over is the region it would drop into, or noRegion.
	Over regionRef
	// moved says the press has travelled far enough to be a drag rather
	// than a click that has not been released yet.
	moved bool
}

// dragThresholdDp is how far a press travels before it is a drag. Below it a
// slightly unsteady click is still a click, which matters most for the people
// this interface has to work for.
const dragThresholdDp = 4

// startTabDrag records a press on a tab. Nothing has been picked up yet.
func (sh *Shell) startTabDrag(name string, from regionRef, at f32.Point) {
	sh.drag = &tabDrag{Name: name, From: from, At: at}
}

// moveTabDrag follows the pointer and works out what is under it.
func (sh *Shell) moveTabDrag(at f32.Point, threshold float32) {
	if sh.drag == nil {
		return
	}
	d := sh.drag
	if !d.moved {
		dx, dy := at.X-d.At.X, at.Y-d.At.Y
		if dx*dx+dy*dy < threshold*threshold {
			return
		}
		d.moved = true
	}
	d.At = at
	d.Over = sh.regionUnder(at)
}

// dropTabDrag puts the panel where it was let go, and reports whether the
// arrangement changed.
func (sh *Shell) dropTabDrag() bool {
	d := sh.drag
	sh.drag = nil
	if d == nil || !d.moved {
		return false
	}
	if !d.Over.valid() || d.Over == d.From {
		return false
	}
	target := sh.regionAt(sh.View, d.Over)
	if target == nil {
		return false
	}
	// Out of the old region and into the new one, in one step: a panel is in
	// one place, and a move that leaves a copy behind is a move that has to
	// be undone twice.
	sh.Undock(d.Name)
	target.Tabs = append(target.Tabs, d.Name)
	target.Active = len(target.Tabs) - 1
	sh.focus = d.Over
	return true
}

// cancelTabDrag drops the tab where it came from, for a cancelled gesture.
func (sh *Shell) cancelTabDrag() { sh.drag = nil }

// dragging reports whether a tab is being carried right now.
func (sh *Shell) dragging() bool { return sh.drag != nil && sh.drag.moved }

// regionUnder is which region contains a point, in window coordinates.
func (sh *Shell) regionUnder(at f32.Point) regionRef {
	p := image.Pt(int(at.X), int(at.Y))
	for ref, r := range sh.regionRect {
		if p.In(r) {
			return ref
		}
	}
	return noRegion
}

// noteRegionSize records what a region measured, for the accumulation.
func (sh *Shell) noteRegionSize(ref regionRef, sz image.Point) {
	if sh.regionSize == nil {
		sh.regionSize = map[regionRef]image.Point{}
	}
	sh.regionSize[ref] = sz
}

// placeRegions turns the measured sizes into rectangles in the window's own
// coordinates - the ones a pointer event speaks.
//
// The gaps between regions - the splitters between columns and rows, the
// rules between rail entries - are whatever the measured sizes do not
// account for, so they need no bookkeeping of their own: the columns of a
// row are laid left to right and share its width, and the rows share the
// body's height.
func (sh *Shell) placeRegions(body image.Point, railDp int) {
	if sh.regionRect == nil {
		sh.regionRect = map[regionRef]image.Rectangle{}
	}
	clear(sh.regionRect)
	a := sh.arrangement()

	y := sh.bodyTop
	for i := range a.Rows {
		x, rowH := 0, 0
		for j := range a.Rows[i].Cols {
			ref := regionRef{Row: i, Col: j}
			sz := sh.regionSize[ref]
			if sz.X <= 0 || sz.Y <= 0 {
				continue
			}
			sh.regionRect[ref] = image.Rect(x, y, x+sz.X, y+sz.Y)
			// The next column starts after this one plus whatever sits
			// between them; the splitter is the only thing that does, and a
			// few pixels of slack cannot change which region a pointer is in.
			x += sz.X
			if sz.Y > rowH {
				rowH = sz.Y
			}
		}
		y += rowH
	}
	// The rail is a stack down the right, whatever the rows left of it did.
	if len(a.Rail) == 0 {
		return
	}
	railX := body.X - railDp
	if railX < 0 {
		railX = 0
	}
	ry := sh.bodyTop
	for j := range a.Rail {
		ref := regionRef{Row: -1, Col: j}
		sz := sh.regionSize[ref]
		if sz.X <= 0 || sz.Y <= 0 {
			continue
		}
		sh.regionRect[ref] = image.Rect(railX, ry, body.X, ry+sz.Y)
		ry += sz.Y
	}
}
