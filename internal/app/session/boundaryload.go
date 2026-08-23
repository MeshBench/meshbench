// A study area of your own, rather than one the gazetteer knows a name for.
//
// boundary.set searches Nominatim, which means the area has to be somewhere
// with an administrative name and this machine has to be online. Plenty of real
// study areas are neither: a catchment, a valley, the bit of a council area
// north of the river, a polygon somebody drew in QGIS this morning. Those
// arrive as GeoJSON, the parser has been in scenario the whole time, and
// nothing outside the process could reach it.
package session

import (
	"fmt"
	"os"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// maxBoundaryBytes bounds what will be read from a path.
//
// Ordnance Survey's whole-country polygons run to tens of megabytes and a
// study area does not need them at that resolution; more to the point, a path
// typed by mistake should fail rather than read a DVD image into memory.
const maxBoundaryBytes = 64 << 20

func registerBoundaryLoad(st *state.Store, s *Sim) {
	// boundary.load: take a study area from GeoJSON.
	//
	// A path or the document itself, because both callers are real: a script
	// has a file, and a program that generated the polygon has a string and
	// should not have to write it to disk to be understood.
	st.Handle("boundary.load", func(w *state.World, p any) (any, error) {
		path, _ := stringField(p, "path")
		text, _ := namedField(p, "geojson")
		if path == "" && text == "" {
			return nil, badParams("boundary.load needs a path or a geojson document")
		}
		if path != "" && text != "" {
			return nil, badParams("boundary.load takes a path or a geojson document, not both")
		}

		data := []byte(text)
		if path != "" {
			st, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if st.Size() > maxBoundaryBytes {
				return nil, badParams("%s is %d MB; a study area does not need that resolution",
					path, st.Size()>>20)
			}
			data, err = os.ReadFile(path) //nolint:gosec // the caller is naming their own study area
			if err != nil {
				return nil, err
			}
		}

		// nameField names the property to take each area's name from. Left
		// out it is guessed, because the property is called "name" in almost
		// every file and asking for it every time is a question with one
		// answer.
		nameField, _ := namedField(p, "name_field")
		if nameField == "" {
			nameField = "name"
		}
		bounds, err := scenario.ParseGeoJSON(data, nameField)
		if err != nil {
			return nil, badParams("%s", err)
		}

		// Every area needs a name, because the name is how one is taken back
		// out again: a nameless polygon joins the study as "" and boundary.remove
		// cannot address it.
		//
		// A name the caller gave wins outright for a single polygon - they are
		// saying what to call this area - and otherwise fills in the blanks the
		// file left, falling back to the file's own name and then to a number.
		chosen, _ := namedField(p, "name")
		fallback := chosen
		if fallback == "" && path != "" {
			fallback = defaultAreaName(path)
		}
		for i := range bounds {
			switch {
			case len(bounds) == 1 && chosen != "":
				bounds[i].Name = chosen
			case bounds[i].Name == "" && fallback != "":
				bounds[i].Name = fallback
			case bounds[i].Name == "":
				bounds[i].Name = fmt.Sprintf("area %d", i+1)
			}
		}

		added := make([]string, 0, len(bounds))
		for _, b := range bounds {
			if hasArea(w.Areas, b.Name) {
				continue
			}
			w.Areas = append(w.Areas, areaFrom(b))
			s.areas = append(s.areas, b)
			added = append(added, b.Name)
		}
		w.Say(fmt.Sprintf("study area now includes %s - %d in all",
			strings.Join(added, ", "), len(w.Areas)))
		return map[string]any{
			"loaded": added, "areas": len(w.Areas), "polygons": len(bounds),
		}, nil
	})
}

// boundary.list: what the study area is made of.
//
// The snapshot carries how many, which answers a card and nothing else: from
// outside the window "which areas am I studying" was unanswerable, and so was
// "did that accept actually take". Names and ring counts, not the geometry -
// a national boundary is megabytes of coordinates and nobody asking this
// wanted them.
func registerBoundaryList(st *state.Store) {
	st.Handle("boundary.list", func(w *state.World, _ any) (any, error) {
		out := make([]map[string]any, 0, len(w.Areas))
		names := make([]string, 0, len(w.Areas))
		for _, a := range w.Areas {
			points := 0
			for _, r := range a.Rings {
				points += len(r)
			}
			out = append(out, map[string]any{
				"name": a.Name, "rings": len(a.Rings), "points": points,
			})
			names = append(names, a.Name)
		}
		return map[string]any{"areas": out, "names": names}, nil
	})
}

// defaultAreaName is a file's basename, which is what somebody who named the
// file "tay-catchment.geojson" already told us the area is called.
func defaultAreaName(path string) string {
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return strings.TrimSuffix(base, ".geojson")
}

func hasArea(areas []state.Area, name string) bool {
	for _, a := range areas {
		if strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

func areaFrom(b scenario.Boundary) state.Area {
	area := state.Area{Name: b.Name}
	for _, r := range b.Rings {
		ring := make([]state.Point, 0, len(r))
		for _, pt := range r {
			ring = append(ring, state.Point{Lat: pt.Lat, Lon: pt.Lon})
		}
		area.Rings = append(area.Rings, ring)
	}
	return area
}
