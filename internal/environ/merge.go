// Merging two footprint sources into one, the way the environment plan
// says: a primary detection source (Microsoft's ML footprints) provides
// existence and height, OpenStreetMap enriches with what it explicitly
// knows - type, levels, material - and explicit information overrides
// inferred. One building on the ground must come out as one building.
package environ

import (
	"bytes"
	"encoding/json"
	"io"
)

// MergeStats says what the merge did, so the operator can judge how much of
// the area OSM actually knows.
type MergeStats struct {
	// Primary and Enrich are the input counts; Matched of the primary
	// buildings found an OSM twin, EnrichOnly stood in OSM alone.
	Primary, Enrich, Matched, EnrichOnly int
}

// mergeBucketDeg buckets centroids for matching: ~200 m, comfortably wider
// than the centroid drift between two tracings of the same roof.
const mergeBucketDeg = 0.002

type mergeFeature struct {
	raw      feature
	ring     [][2]float64 // lon, lat as GeoJSON carries them
	centroid [2]float64
	used     bool
}

// MergeGeoJSON reads two newline-delimited GeoJSON streams and returns one:
// every primary building, enriched with the tags of the OSM building whose
// centroid falls inside its footprint, plus every OSM building the primary
// source missed. The enriched feature keeps OSM's geometry and properties -
// a surveyed outline beats a detected one - and inherits the primary height
// only where OSM states neither a height nor a level count.
func MergeGeoJSON(primary, enrich io.Reader) (io.Reader, MergeStats, error) {
	var stats MergeStats
	pf, err := readMergeFeatures(primary)
	if err != nil {
		return nil, stats, err
	}
	ef, err := readMergeFeatures(enrich)
	if err != nil {
		return nil, stats, err
	}
	stats.Primary, stats.Enrich = len(pf), len(ef)

	grid := map[[2]int][]*mergeFeature{}
	key := func(c [2]float64) [2]int {
		return [2]int{int(c[0] / mergeBucketDeg), int(c[1] / mergeBucketDeg)}
	}
	for i := range ef {
		k := key(ef[i].centroid)
		grid[k] = append(grid[k], &ef[i])
	}

	var out bytes.Buffer
	emit := func(ft feature) {
		line, err := json.Marshal(ft)
		if err != nil {
			return
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	for i := range pf {
		p := &pf[i]
		var twin *mergeFeature
		k := key(p.centroid)
		for dx := -1; dx <= 1 && twin == nil; dx++ {
			for dy := -1; dy <= 1 && twin == nil; dy++ {
				for _, e := range grid[[2]int{k[0] + dx, k[1] + dy}] {
					if !e.used && pointInRing(e.centroid, p.ring) {
						twin = e
						break
					}
				}
			}
		}
		if twin == nil {
			emit(p.raw)
			continue
		}
		twin.used = true
		stats.Matched++
		merged := twin.raw
		if merged.Properties == nil {
			merged.Properties = map[string]any{}
		}
		// Explicit overrides inferred: only where OSM says nothing about
		// height does the detection's height survive.
		if numProp(merged.Properties, "height") <= 0 &&
			numProp(merged.Properties, "building:levels") <= 0 {
			if h := numProp(p.raw.Properties, "height"); h > 0 {
				merged.Properties["height"] = h
			}
		}
		emit(merged)
	}
	for i := range ef {
		if !ef[i].used {
			stats.EnrichOnly++
			emit(ef[i].raw)
		}
	}
	return bytes.NewReader(out.Bytes()), stats, nil
}

// readMergeFeatures parses a newline-delimited stream into matchable
// features; anything that is not a polygon is dropped, exactly as the
// ingester would drop it later.
func readMergeFeatures(r io.Reader) ([]mergeFeature, error) {
	var out []mergeFeature
	dec := json.NewDecoder(r)
	for {
		var ft feature
		if err := dec.Decode(&ft); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if ft.Geometry.Type != "Polygon" {
			continue
		}
		var rings [][][2]float64
		if json.Unmarshal(ft.Geometry.Coordinates, &rings) != nil ||
			len(rings) == 0 || len(rings[0]) < 3 {
			continue
		}
		mf := mergeFeature{raw: ft, ring: rings[0]}
		for _, v := range rings[0] {
			mf.centroid[0] += v[0]
			mf.centroid[1] += v[1]
		}
		mf.centroid[0] /= float64(len(rings[0]))
		mf.centroid[1] /= float64(len(rings[0]))
		out = append(out, mf)
	}
	return out, nil
}

// pointInRing is a ray cast, in the ring's own lon/lat plane.
func pointInRing(pt [2]float64, ring [][2]float64) bool {
	in := false
	j := len(ring) - 1
	for i := 0; i < len(ring); i++ {
		xi, yi := ring[i][0], ring[i][1]
		xj, yj := ring[j][0], ring[j][1]
		if (yi > pt[1]) != (yj > pt[1]) &&
			pt[0] < (xj-xi)*(pt[1]-yi)/(yj-yi)+xi {
			in = !in
		}
		j = i
	}
	return in
}
