package comp

import (
	"math"
	"testing"
)

// A slippy zoom level is not pixels per degree, and the camera takes the
// second.
//
// -look 56.33,-3.32,14 rendered Greenland: fourteen pixels per degree is most
// of the planet across a window, so every level a person tried came out at
// roughly world scale and the flag looked like it was ignoring its third
// field. It was not - it was reading it in units nobody types.
func TestASlippyLevelIsNotPixelsPerDegree(t *testing.T) {
	// The world is 256 pixels wide at level 0, so a degree is a fraction of a
	// pixel; by the high teens a degree is tens of thousands.
	for _, c := range []struct {
		level   float64
		wantPPD float64
	}{
		{8, 256 * 256 / 360.0},
		{14, 256 * 16384 / 360.0},
	} {
		if got := ZoomForLevel(c.level); math.Abs(got-c.wantPPD) > 1 {
			t.Errorf("level %v is %.1f px per degree, want %.1f",
				c.level, got, c.wantPPD)
		}
	}
	// Six levels apart is a factor of sixty-four, which is the thing the
	// reported bug did not do: two views one step of the scale bar apart.
	if r := ZoomForLevel(14) / ZoomForLevel(8); math.Abs(r-64) > 0.5 {
		t.Errorf("eight to fourteen is a factor of %.1f, want 64", r)
	}
}

// The two directions agree, so a camera reported back to somebody is in the
// units they set it in.
func TestZoomAndLevelRoundTrip(t *testing.T) {
	for _, level := range []float64{2, 6, 9, 13, 16, 19} {
		if got := LevelForZoom(ZoomForLevel(level)); math.Abs(got-level) > 0.001 {
			t.Errorf("level %v came back as %v", level, got)
		}
	}
}

// Levels outside what the camera can hold are pulled to what it can, rather
// than producing a scale the projection divides by.
func TestAnImpossibleLevelIsClamped(t *testing.T) {
	if z := ZoomForLevel(-5); z < 2 {
		t.Errorf("a negative level gave %v, under the camera's floor", z)
	}
	if z := ZoomForLevel(40); z > 4_000_000 {
		t.Errorf("an enormous level gave %v, over the camera's ceiling", z)
	}
}

// What a level actually frames, which is the thing the report was about.
//
// The complaint was not arithmetic: it was that -look 56.33,-3.32,16, meant to
// be a street in Perthshire, rendered Greenland and Iceland. So the assertion
// is in degrees across the window, because that is what somebody sees.
func TestALevelFramesWhatSomebodyExpects(t *testing.T) {
	const windowPx = 1000
	for _, c := range []struct {
		level          float64
		minDeg, maxDeg float64
		what           string
	}{
		{6, 15, 90, "a continent"},
		{8, 3, 20, "a region"},
		{14, 0.02, 0.3, "a town"},
		{16, 0.005, 0.08, "a street"},
	} {
		span := windowPx / ZoomForLevel(c.level)
		if span < c.minDeg || span > c.maxDeg {
			t.Errorf("level %v spans %.4f degrees across %dpx, want %v "+
				"(%v to %v)", c.level, span, windowPx, c.what, c.minDeg, c.maxDeg)
		}
	}
	// And the old behaviour, so the regression has a name: the raw number as
	// pixels per degree put the whole planet on screen at every level anybody
	// tried.
	for _, level := range []float64{8, 14, 16} {
		if span := windowPx / level; span < 60 {
			t.Errorf("level %v read as pixels per degree spans %.0f degrees, "+
				"which would not have looked like the whole world", level, span)
		}
	}
}
