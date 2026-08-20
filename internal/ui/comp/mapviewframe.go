// Framing: where the camera goes when somebody asks to see everything.
//
// Split from mapview.go at the file limit. It is its own concern anyway - the
// map draws what it is told to draw, and these decide what that is: every
// node, one node, or the study area when there are no nodes at all.
package comp

import (
	"image"
	"math"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func (m *MapView) fit(s *state.Snapshot, sz image.Point) {
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0
	placed := 0
	for _, n := range s.Nodes {
		if n.Lat == 0 && n.Lon == 0 {
			continue
		}
		placed++
		minLat, maxLat = math.Min(minLat, n.Lat), math.Max(maxLat, n.Lat)
		minLon, maxLon = math.Min(minLon, n.Lon), math.Max(maxLon, n.Lon)
	}
	if placed == 0 {
		// Nothing to frame. Framing it anyway averaged the empty extents to
		// 0,0 and put a blank network in the Atlantic, a thousand miles from
		// anywhere anybody meant to place a repeater. The study area is the
		// next best answer; failing that the camera stays where it was.
		m.fitAreas(s, sz)
		return
	}
	m.CentreLat, m.CentreLon = (minLat+maxLat)/2, (minLon+maxLon)/2
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	spanX, spanY := (maxLon-minLon)*cos, maxLat-minLat
	if spanX <= 0 || spanY <= 0 {
		m.Zoom = 1000
		return
	}
	m.Zoom = math.Min(float64(sz.X-80)/spanX, float64(sz.Y-80)/spanY)
}

// Fit frames every node with a margin (10.9).
func (m *MapView) Fit(s *state.Snapshot, sz image.Point) {
	if s == nil || len(s.Nodes) == 0 {
		return
	}
	m.fit(s, sz)
}

// FocusOn centres the camera on a named node without changing the zoom, which
// is what "centre on this" means: the same view, somewhere else.
func (m *MapView) FocusOn(s *state.Snapshot, name string) bool {
	if s == nil {
		return false
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == name {
			m.CentreOn(s.Nodes[i].Lat, s.Nodes[i].Lon)
			return true
		}
	}
	return false
}

// nodeMatches is the map filter: name or kind, case-insensitively.
func nodeMatches(n *state.Node, want string) bool {
	return strings.Contains(strings.ToLower(n.Name), want) ||
		strings.Contains(strings.ToLower(n.Kind), want)
}

// fitAreas frames the study area, for a network with nothing in it yet.
//
// Somebody starting blank has usually just said where they are working -
// that is what the study area is - so the map opens there rather than
// wherever the last network happened to be.
func (m *MapView) fitAreas(s *state.Snapshot, sz image.Point) {
	if m.Zoom <= 0 {
		m.Zoom = 1000
	}
	// The largest outline, not the extent of every one of them.
	//
	// France's boundary takes in Guadeloupe, Réunion and New Caledonia, so
	// framing all of it frames the planet and mainland France is a smudge.
	// The biggest ring is the part somebody meant.
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0
	best := 0
	for _, a := range s.Areas {
		for _, ring := range a.Rings {
			if len(ring) <= best {
				continue
			}
			best = len(ring)
			minLat, maxLat = 90.0, -90.0
			minLon, maxLon = 180.0, -180.0
			for _, p := range ring {
				minLat, maxLat = math.Min(minLat, p.Lat), math.Max(maxLat, p.Lat)
				minLon, maxLon = math.Min(minLon, p.Lon), math.Max(maxLon, p.Lon)
			}
		}
	}
	if best == 0 {
		return // the camera keeps whatever it was looking at
	}
	m.CentreLat, m.CentreLon = (minLat+maxLat)/2, (minLon+maxLon)/2
	cos := math.Cos(m.CentreLat * math.Pi / 180)
	spanX, spanY := (maxLon-minLon)*cos, maxLat-minLat
	if spanX <= 0 || spanY <= 0 {
		return
	}
	m.Zoom = math.Min(float64(sz.X-80)/spanX, float64(sz.Y-80)/spanY)
}
