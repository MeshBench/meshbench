package comp

import (
	"image"
	"math"
	"testing"

	"gioui.org/f32"
)

func zoomView() *MapView {
	return &MapView{Zoom: 1000, CentreLat: 56, CentreLon: -3}
}

// A wheel notch moves where the zoom is going, not where it is. Applying each
// notch on arrival is what made the map move in the size of a notch, however
// small the per-unit multiplier was - the steps were the input's, and no
// constant was going to smooth them.
func TestAWheelNotchAimsRatherThanJumps(t *testing.T) {
	m := zoomView()
	sz := image.Pt(800, 600)
	before := m.Zoom

	m.aimZoom(1.5, f32.Pt(400, 300), sz)
	if m.Zoom != before {
		t.Errorf("the camera moved on the event itself: %.1f -> %.1f", before, m.Zoom)
	}
	if m.zoomTarget <= before {
		t.Errorf("target = %.1f, want it beyond the current zoom %.1f", m.zoomTarget, before)
	}

	// It arrives over several frames, and each one is nearer than the last.
	prev := m.Zoom
	for i := 0; i < 200 && m.stepZoom(1.0/60, sz); i++ {
		if m.Zoom < prev {
			t.Fatalf("frame %d went backwards: %.2f -> %.2f", i, prev, m.Zoom)
		}
		prev = m.Zoom
	}
	if math.Abs(m.Zoom-m.zoomTarget) > 0.5 {
		t.Errorf("settled at %.2f, target %.2f", m.Zoom, m.zoomTarget)
	}
}

// Three notches in quick succession are one longer glide, not three jumps -
// which is the whole reason the target is multiplied rather than the camera.
func TestQuickNotchesCompose(t *testing.T) {
	m := zoomView()
	at := f32.Pt(400, 300)
	m.aimZoom(1.2, at, image.Pt(800, 600))
	m.aimZoom(1.2, at, image.Pt(800, 600))
	m.aimZoom(1.2, at, image.Pt(800, 600))
	want := 1000 * 1.2 * 1.2 * 1.2
	if math.Abs(m.zoomTarget-want) > 1 {
		t.Errorf("target = %.1f, want %.1f - notches must compound", m.zoomTarget, want)
	}
}

// The point under the cursor stays under the cursor for every frame of the
// glide, not merely once it lands.
func TestTheAnchorHoldsThroughoutTheGlide(t *testing.T) {
	m := zoomView()
	sz := image.Pt(800, 600)
	at := f32.Pt(600, 200) // deliberately off-centre
	wantLat, wantLon := m.unproject(at, sz)

	m.aimZoom(2.0, at, sz)
	for i := 0; i < 200 && m.stepZoom(1.0/60, sz); i++ {
		gotLat, gotLon := m.unproject(at, sz)
		if math.Abs(gotLat-wantLat) > 1e-6 || math.Abs(gotLon-wantLon) > 1e-6 {
			t.Fatalf("frame %d: the ground under the cursor moved to %.6f,%.6f from %.6f,%.6f",
				i, gotLat, gotLon, wantLat, wantLon)
		}
	}
}

// A frame time of zero (the first frame) or a huge one (the window was
// asleep) must not teleport the camera.
func TestAnOddFrameTimeDoesNotTeleport(t *testing.T) {
	sz := image.Pt(800, 600)
	for _, dt := range []float64{0, -1, 5} {
		m := zoomView()
		m.aimZoom(4, f32.Pt(400, 300), sz)
		m.stepZoom(dt, sz)
		if m.Zoom >= m.zoomTarget {
			t.Errorf("dt=%v arrived in one frame: %.1f of %.1f", dt, m.Zoom, m.zoomTarget)
		}
	}
}

// Zoom stays within the range the tile arithmetic can carry, however hard the
// wheel is spun.
func TestTheGlideStaysWithinRange(t *testing.T) {
	sz := image.Pt(800, 600)
	for _, f := range []float64{1e9, 1e-9} {
		m := zoomView()
		m.aimZoom(f, f32.Pt(400, 300), sz)
		for i := 0; i < 500 && m.stepZoom(1.0/60, sz); i++ {
		}
		if m.Zoom < 2 || m.Zoom > 4_000_000 {
			t.Errorf("factor %g left zoom at %.2f, outside the clamp", f, m.Zoom)
		}
	}
}
