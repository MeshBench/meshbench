// A study area of your own, rather than one the gazetteer knows a name for.
//
// boundary.set searches Nominatim, which means the area has to be somewhere
// with an administrative name and this machine has to be online. Plenty of real
// study areas are neither: a catchment, a valley, the bit of a council area
// north of the river, a polygon somebody drew in QGIS this morning. Those
// arrive as GeoJSON, the parser has been in scenario the whole time, and
// nothing outside the process could reach it.
package boundary

import (
	"fmt"
	"os"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// maxBoundaryBytes bounds what will be read from a path.
//
// Ordnance Survey's whole-country polygons run to tens of megabytes and a
// study area does not need them at that resolution; more to the point, a path
// typed by mistake should fail rather than read a DVD image into memory.
const maxBoundaryBytes = 64 << 20

func registerBoundaryLoad(st *state.Store, s *session.Sim) {
	st.HandleSpec("boundary.load", state.Spec{
		What: "take a study area from GeoJSON rather than from the gazetteer, " +
			"which is the only way to study a catchment, a valley or a polygon " +
			"somebody drew this morning, and the only way to set one offline",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Primary: true,
				What: "a file to read the GeoJSON from; a file over 64 MB is " +
					"refused, and giving neither this nor geojson is refused, " +
					"as is giving both"},
			{Name: "geojson", Type: state.ParamString,
				What: "the document itself, for a caller that generated the " +
					"polygon and should not have to write it to disk first"},
			{Name: "name", Type: state.ParamString,
				What: "what to call the area; for a single polygon it wins " +
					"outright, for several it only fills in the ones the file " +
					"left unnamed, and absent it falls back to the file's own " +
					"name and then to a number"},
			{Name: "name_field", Type: state.ParamString,
				What: "the feature property to read each name from; absent it " +
					"is \"name\", which is what almost every file calls it"},
		},
		Returns: []string{"loaded", "areas", "polygons"},
		Answers: "`polygons` is what the document held and `loaded` names only " +
			"those actually added, so it is shorter when an area of that name " +
			"was already in the study and empty when all of them were. `areas` " +
			"is the size of the whole study area afterwards. A GeoJSON " +
			"coordinate is longitude then latitude, which is the opposite way " +
			"round to everything else here.",
		Example: &state.Example{
			Params: map[string]any{
				"geojson": `{"type":"Polygon","coordinates":` +
					`[[[-3.5,56.0],[-3.2,56.0],[-3.2,56.3],[-3.5,56.3],[-3.5,56.0]]]}`,
				"name": "Lomond hills",
			},
			What:     "study a polygon the gazetteer has no name for",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		path, _ := session.StringField(p, "path")
		text, _ := session.NamedField(p, "geojson")
		if path == "" && text == "" {
			return nil, session.BadParams("boundary.load needs a path or a geojson document")
		}
		if path != "" && text != "" {
			return nil, session.BadParams("boundary.load takes a path or a geojson document, not both")
		}

		data := []byte(text)
		if path != "" {
			st, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if st.Size() > maxBoundaryBytes {
				return nil, session.BadParams("%s is %d MB; a study area does not need that resolution",
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
		nameField, _ := session.NamedField(p, "name_field")
		if nameField == "" {
			nameField = "name"
		}
		bounds, err := scenario.ParseGeoJSON(data, nameField)
		if err != nil {
			return nil, session.BadParams("%s", err)
		}

		// Every area needs a name, because the name is how one is taken back
		// out again: a nameless polygon joins the study as "" and boundary.remove
		// cannot address it.
		//
		// A name the caller gave wins outright for a single polygon - they are
		// saying what to call this area - and otherwise fills in the blanks the
		// file left, falling back to the file's own name and then to a number.
		chosen, _ := session.NamedField(p, "name")
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
			s.SetAreas(append(s.Areas(), b))
			added = append(added, b.Name)
		}
		w.Say(fmt.Sprintf("study area now includes %s - %d in all",
			strings.Join(added, ", "), len(w.Areas)))
		return map[string]any{
			"loaded": added, "areas": len(w.Areas), "polygons": len(bounds),
		}, nil
	})
}

// The snapshot carries how many, which answers a card and nothing else: from
// outside the window "which areas am I studying" was unanswerable, and so was
// "did that accept actually take".
func registerBoundaryList(st *state.Store) {
	st.HandleSpec("boundary.list", state.Spec{
		What: "say which areas the study is made of, which is how a caller " +
			"outside the window finds out whether an accept or a load took",
		Returns: []string{"areas", "names"},
		Answers: "`areas` is a row per area with its name and how many rings " +
			"and points it is drawn from, and `names` the same names on their " +
			"own, ready for boundary.remove. The geometry itself is not " +
			"returned: a national boundary is megabytes of coordinates. Both " +
			"are empty when no study area has been set, which is not an error.",
		Example: &state.Example{
			Params: map[string]any{}, What: "check what the study area holds",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
