package comp

import (
	"image"
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/basemap"
)

// What the map actually asks for, all the way out.
//
// The path is viewport -> bounds -> zoom -> tiles, and only the last step
// clamps. This walks the whole zoom range the input allows and checks the
// answer stays the size of a screen, because the failure was a frame that
// asked for sixteen million tiles and never finished drawing.
func TestNoZoomAsksForMoreTilesThanFitOnScreen(t *testing.T) {
	const (
		// clampZoom's own floor and ceiling, in pixels per degree.
		minZoom = 2.0
		maxZoom = 4_000_000.0
	)
	// What a frame may ask for. A 2560x1440 screen holds about 84 tiles of
	// 256 pixels; the worst case over the whole zoom range is 240, right at
	// the bottom where the screen shows nearly the whole world at a zoom
	// chosen to fill it. The rest of the headroom is for ZoomFor's rounding,
	// which can land half a level either way and so up to double per axis.
	//
	// The number matters less than its size: the bug this guards produced
	// 15,895,926.
	const maxPerFrame = 512

	sz := image.Pt(2560, 1440)
	layer := basemap.Layer{MaxZoom: 18}

	for _, lat := range []float64{0, 56.47, -45, 84, -84} {
		for z := minZoom; z <= maxZoom; z *= 1.07 {
			zoom := basemap.ZoomFor(metresPerPixel(z), lat, layer)
			south, north, west, east := viewportBounds(sz, lat, -4.2, z)
			got := len(basemap.TilesFor(south, north, west, east, zoom))
			if got > maxPerFrame {
				t.Fatalf("at %g px/deg and latitude %g: %d tiles for one frame",
					z, lat, got)
			}
			if got == 0 {
				t.Fatalf("at %g px/deg and latitude %g: no tiles at all", z, lat)
			}
		}
	}
}

// The tiles fetched must be the resolution the screen is showing.
//
// This is the invariant the cosine broke. Folding it into the scale made the
// screen look finer than it was, so ZoomFor went log2(1/cos) levels too deep -
// one extra level in Scotland, three at latitude 84 - and each level is four
// times the tiles, fetched and decoded and drawn, for a picture no sharper
// than the screen can show. The ratio below was 10:1 at latitude 84.
func TestTheTileZoomMatchesTheScreenScale(t *testing.T) {
	layer := basemap.Layer{MaxZoom: 18}
	for _, lat := range []float64{0, 30, 56.47, 70, 84, -84} {
		for _, px := range []float64{8, 64, 512, 4096, 32768, 262144} {
			mpp := metresPerPixel(px)
			zoom := basemap.ZoomFor(mpp, lat, layer)
			if zoom >= layer.MaxZoom || zoom <= 1 {
				continue // clamped rather than chosen, so it proves nothing
			}
			// Web Mercator's ground resolution at that zoom and latitude.
			res := 156543.033928 * math.Cos(lat*math.Pi/180) / math.Exp2(float64(zoom))
			// Rounding in log space allows a factor of root two either way,
			// and nothing beyond it.
			if ratio := res / mpp; ratio < 0.7 || ratio > 1.42 {
				t.Errorf("at latitude %g and %g px/deg: tile zoom %d gives %.1f m/px "+
					"for a screen showing %.1f m/px (%.2fx)", lat, px, zoom, res, mpp, ratio)
			}
		}
	}
}

// The bounds themselves are deliberately not clamped - see viewportBounds. If
// somebody "fixes" that here, the tile grid's clamp becomes the only thing
// standing between a wide viewport and a hang, and this test says why it is
// allowed to look wrong.
func TestTheViewportMayCoverMoreThanThePlanet(t *testing.T) {
	south, north, _, _ := viewportBounds(image.Pt(1600, 900), 56, -4, 2)
	if north <= 90 && south >= -90 {
		t.Skip("the minimum zoom no longer covers more than the planet")
	}
	if len(basemap.TilesFor(south, north, -180, 180, 1)) > 4 {
		t.Error("the tile grid did not clamp a viewport larger than the world")
	}
}

// A zero or negative scale is division by zero, and a NaN centre is what came
// back out of it.
func TestADegenerateZoomDoesNotProduceNaN(t *testing.T) {
	for _, z := range []float64{0, -1} {
		s, n, w, e := viewportBounds(image.Pt(800, 600), 56, -4, z)
		for _, v := range []float64{s, n, w, e} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("zoom %g gave bounds %g %g %g %g", z, s, n, w, e)
			}
		}
	}
}
