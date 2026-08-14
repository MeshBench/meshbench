package ui_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/ui"
)

func view() ui.MapView {
	return ui.MapView{CentreLat: 56.7, CentreLon: -3.9, MetresPerPixel: 50, Width: 800, Height: 600}
}

// Clicking is the inverse of drawing. A projection that does not round-trip
// puts a placed node somewhere nobody clicked, and the error is small enough at
// first to look like imprecision rather than a bug.
func TestProjectionRoundTrips(t *testing.T) {
	v := view()
	for _, p := range [][2]float64{{0, 0}, {400, 300}, {799, 599}, {123, 456}} {
		lat, lon := v.ScreenToLatLon(p[0], p[1])
		x, y := v.LatLonToScreen(lat, lon)
		if math.Abs(x-p[0]) > 1e-6 || math.Abs(y-p[1]) > 1e-6 {
			t.Errorf("(%.0f,%.0f) -> (%.6f,%.6f) -> (%.6f,%.6f)", p[0], p[1], lat, lon, x, y)
		}
	}
}

// North must be up and east must be right. Getting the latitude sign wrong
// mirrors the map vertically, and a mirrored map of unfamiliar terrain looks
// entirely plausible.
func TestNorthIsUpAndEastIsRight(t *testing.T) {
	v := view()
	cx, cy := v.LatLonToScreen(v.CentreLat, v.CentreLon)
	nx, ny := v.LatLonToScreen(v.CentreLat+0.05, v.CentreLon)
	ex, ey := v.LatLonToScreen(v.CentreLat, v.CentreLon+0.05)

	if ny >= cy {
		t.Errorf("a point 0.05 degrees north drew at y=%.1f, below centre y=%.1f", ny, cy)
	}
	if ex <= cx {
		t.Errorf("a point 0.05 degrees east drew at x=%.1f, left of centre x=%.1f", ex, cx)
	}
	// North must not shift east, and east must not shift north.
	if math.Abs(nx-cx) > 1e-6 || math.Abs(ey-cy) > 1e-6 {
		t.Errorf("the axes are not orthogonal: north moved x by %.6f, east moved y by %.6f",
			nx-cx, ey-cy)
	}
}

// Zooming about the cursor keeps the ground under it. Zooming about the centre
// makes the map slide away from whatever the user was looking at, which is the
// difference between a map you can work with and one you fight.
func TestZoomKeepsTheGroundUnderTheCursor(t *testing.T) {
	v := view()
	const px, py = 620.0, 140.0
	beforeLat, beforeLon := v.ScreenToLatLon(px, py)

	v.ZoomAt(px, py, 2.0)
	afterLat, afterLon := v.ScreenToLatLon(px, py)

	if math.Abs(afterLat-beforeLat) > 1e-9 || math.Abs(afterLon-beforeLon) > 1e-9 {
		t.Errorf("the ground under the cursor moved: (%.8f,%.8f) -> (%.8f,%.8f)",
			beforeLat, beforeLon, afterLat, afterLon)
	}
	if v.MetresPerPixel >= 50 {
		t.Errorf("zooming in did not increase the scale: %.2f m/px", v.MetresPerPixel)
	}
}

func TestPanMovesTheRightWay(t *testing.T) {
	v := view()
	lat0, lon0 := v.CentreLat, v.CentreLon
	// Dragging the map to the right moves the view west.
	v.PanPixels(100, 0)
	if v.CentreLon >= lon0 {
		t.Errorf("dragging right moved the centre east: %.5f -> %.5f", lon0, v.CentreLon)
	}
	v = view()
	// Dragging down moves the view north.
	v.PanPixels(0, 100)
	if v.CentreLat <= lat0 {
		t.Errorf("dragging down moved the centre south: %.5f -> %.5f", lat0, v.CentreLat)
	}
}

// Markers overlap at low zoom. Picking the first match in list order selects a
// node the user cannot see instead of the one drawn on top.
func TestNodeAtPicksTheNearestNotTheFirst(t *testing.T) {
	v := view()
	farLat, farLon := v.ScreenToLatLon(412, 300)
	nearLat, nearLon := v.ScreenToLatLon(400, 300)
	nodes := []scenario.Node{
		{Name: "far", Position: scenario.LatLon{Lat: farLat, Lon: farLon}},
		{Name: "near", Position: scenario.LatLon{Lat: nearLat, Lon: nearLon}},
	}

	if got := v.NodeAt(nodes, 400, 300, 20); got != 1 {
		t.Errorf("picked index %d; the nearer node is index 1", got)
	}
	if got := v.NodeAt(nodes, 700, 300, 20); got != -1 {
		t.Errorf("a click in empty space selected node %d", got)
	}
}

// Fitting must show every node, with room, or the outermost markers sit half
// off the edge where they are easy to miss.
func TestFitFramesEveryNode(t *testing.T) {
	nodes := []scenario.Node{
		{Position: scenario.LatLon{Lat: 56.75, Lon: -3.74}},
		{Position: scenario.LatLon{Lat: 56.39, Lon: -3.43}},
		{Position: scenario.LatLon{Lat: 56.56, Lon: -3.59}},
	}
	var v ui.MapView
	v.FitTo(nodes, 800, 600)

	for i, n := range nodes {
		x, y := v.LatLonToScreen(n.Position.Lat, n.Position.Lon)
		if x < 10 || x > 790 || y < 10 || y > 590 {
			t.Errorf("node %d drew at (%.0f,%.0f), outside or against the edge", i, x, y)
		}
	}
	south, north, west, east := v.Bounds()
	if north <= south || east <= west {
		t.Errorf("bounds are inverted: %.4f..%.4f, %.4f..%.4f", south, north, west, east)
	}
}
