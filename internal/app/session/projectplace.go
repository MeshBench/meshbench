// Turning a place name into somewhere to look.
//
// Both of these serve project.new's optional place: a blank network with no
// place is a map in the middle of the Atlantic, so the name becomes the study
// area and the camera is pointed at it. Kept out of verbs.go because neither
// is a verb, and that file is a list of them.
package session

import (
	"math"
	"strings"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// centreOf is the middle of a set of outlines, for pointing a camera at a
// place that has no nodes in it yet.
// centreOf is the middle of the largest outline in a set, for pointing a
// camera at a place that has no nodes in it yet.
//
// The largest, not the extent of all of them: France's boundary takes in
// Guadeloupe, French Guiana and Réunion, and the middle of that extent is
// open ocean off west Africa. The biggest ring is the part somebody means.
func centreOf(bs []scenario.Boundary) (lat, lon float64, ok bool) {
	best := 0
	for _, b := range bs {
		for _, ring := range b.Rings {
			if len(ring) <= best {
				continue
			}
			minLat, maxLat := 90.0, -90.0
			minLon, maxLon := 180.0, -180.0
			for _, p := range ring {
				minLat, maxLat = math.Min(minLat, p.Lat), math.Max(maxLat, p.Lat)
				minLon, maxLon = math.Min(minLon, p.Lon), math.Max(maxLon, p.Lon)
			}
			best = len(ring)
			lat, lon, ok = (minLat+maxLat)/2, (minLon+maxLon)/2, true
		}
	}
	return lat, lon, ok
}

// knowsArea reports whether the last search already found this place, so a
// name picked from a list is not looked up a second time.
func (s *Sim) knowsArea(name string) bool {
	for _, f := range s.foundAreas {
		if strings.EqualFold(f.Name, name) || strings.EqualFold(f.DisplayName, name) {
			return true
		}
	}
	return false
}
