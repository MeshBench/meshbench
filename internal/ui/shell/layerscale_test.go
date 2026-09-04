package shell

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/unit"
)

// A drag that crosses onto a screen at another scale keeps following the
// pointer.
//
// Reported: dragging a window leftwards across the boundary between a screen
// at 150% and one at 100% made it bounce until it was wholly over. Gio takes
// the largest scale of the outputs a surface is on, so the instant the window
// touched the scaled screen its pixels-per-dp doubled. The bar reports the
// point it was grabbed at in the pixels of the frame the press happened in, so
// from that moment the grab was measured in one unit and the pointer in
// another: the difference between them doubled, the window leapt, the leap
// changed which screens it was touching, the scale went back, and it leapt the
// other way.
//
// The distances here are the arithmetic of that. The press is at 100 pixels
// with one pixel to the dp, so the grabbed point is 100 dp into the surface.
// The scale then doubles, and the pointer - having moved 40 dp - reports 280
// pixels, because the same place in the surface is now twice as many pixels
// along. What the window owes is 40 dp. Measuring both in the surface's own
// logical pixels gives exactly that; multiplying the raw difference by the new
// metric gives 90, and that leap is the bug.
func TestLayerChromeDragSurvivesAScaleChange(t *testing.T) {
	h := newChromeHarness()
	h.pxPerDp = 1
	h.frame(image.Pt(800, 600))
	h.frame(image.Pt(800, 600))

	// Take hold and move, both on the unscaled screen. The grabbed point is
	// 100 into the surface and the window owes 40.
	h.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(100, 12),
		Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(140, 12),
		Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	if moved := h.chrome.spot.Left - unit.Dp(24); moved != unit.Dp(40) {
		t.Fatalf("the window moved %v before any scale change, want 40dp", moved)
	}
	crossed := h.chrome.spot

	// Now the window's edge reaches the scaled screen and its pixels double
	// underneath the drag. The pointer has not moved: the window caught up, so
	// the same place in the surface is the grabbed point again - 100 dp in,
	// which is 200 of the new pixels. Nothing is owed, so nothing should move.
	h.pxPerDp = 2
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(200, 24),
		Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	if leapt := h.chrome.spot.Left - crossed.Left; leapt != 0 {
		t.Errorf("the window leapt %v when the scale changed under it, want none"+
			" - the grab and the pointer are being measured in different units", leapt)
	}
	// Horizontally only. The router reports a position in the bar's own space,
	// and the bar's origin sits a few pixels down that the harness does not
	// scale, so a vertical coordinate chosen here is not the same place in the
	// surface under both metrics - which would be measuring the arithmetic in
	// this file rather than the code. Across is clean, and it is the axis the
	// fault appeared on.

	// And it still follows: thirty dp further left is sixty of the new pixels.
	h.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(140, 24),
		Buttons: pointer.ButtonPrimary})
	h.frame(image.Pt(800, 600))
	if moved := h.chrome.spot.Left - crossed.Left; moved != unit.Dp(-30) {
		t.Errorf("the window moved %v after the scale change, want -30dp", moved)
	}
}
