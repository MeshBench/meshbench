// The material taxonomy, its inference, and what a wall costs a signal.
package environ

import "strings"

// The MeshBench taxonomy, owned here rather than tied to OSM's tags - the
// plan's rule, so a differently-tagged dataset maps into the same physics.
const (
	MatUnknown     = "UNKNOWN"
	MatBrick       = "BRICK"
	MatConcrete    = "CONCRETE"
	MatStone       = "STONE"
	MatTimber      = "TIMBER"
	MatMetal       = "METAL"
	MatGlass       = "GLASS"
	MatLightweight = "LIGHTWEIGHT"
	MatMixed       = "MIXED"
)

// InferMaterial follows the plan's priority order: explicit tags first, then
// the building's classification, then a regional default, then UNKNOWN -
// with the source and confidence carried so the interface can distinguish
// "this building is brick" from "buildings like this are usually brick".
func InferMaterial(osmMaterial, buildingType, region string) (material, source string, confidence float64) {
	if m := canonicalMaterial(osmMaterial); m != MatUnknown {
		return m, "osm", 1.0
	}
	switch strings.ToLower(buildingType) {
	case "industrial", "warehouse":
		return MatMetal, "type", 0.55
	case "commercial", "retail", "office":
		return MatConcrete, "type", 0.5
	case "church", "cathedral", "castle", "ruins":
		return MatStone, "type", 0.8
	case "residential", "house", "detached", "semidetached_house", "terrace", "apartments":
		// The UK default; a regional profile can overrule when one exists.
		if strings.EqualFold(region, "uk") || region == "" {
			return MatBrick, "regional", 0.6
		}
		return MatMixed, "regional", 0.4
	case "greenhouse":
		return MatGlass, "type", 0.9
	case "shed", "hut", "cabin":
		return MatTimber, "type", 0.6
	}
	return MatUnknown, "", 0
}

func canonicalMaterial(osm string) string {
	switch strings.ToLower(strings.TrimSpace(osm)) {
	case "brick":
		return MatBrick
	case "concrete", "reinforced_concrete":
		return MatConcrete
	case "stone", "sandstone", "granite", "masonry":
		return MatStone
	case "wood", "timber", "timber_framing":
		return MatTimber
	case "metal", "steel", "corrugated_metal", "tin":
		return MatMetal
	case "glass":
		return MatGlass
	case "plaster", "vinyl", "plastic":
		return MatLightweight
	}
	return MatUnknown
}

// MaterialLossDB is what one exterior wall costs at a frequency - the RF
// engine's question, deliberately never stored in the dataset so one
// environment serves every band. Figures follow ITU-R P.2040's building
// entry loss measurements, interpolated crudely between its 1-2 GHz anchors
// and the sub-GHz behaviour the mesh actually uses; W5's hardware suite is
// what will tighten them.
func MaterialLossDB(material string, freqMHz float64) float64 {
	// Sub-GHz base figures per wall; higher bands pay more.
	base := map[string]float64{
		MatBrick:       6,
		MatConcrete:    10,
		MatStone:       9,
		MatTimber:      3,
		MatMetal:       20,
		MatGlass:       2,
		MatLightweight: 2,
		MatMixed:       6,
		MatUnknown:     6,
	}[material]
	if base == 0 {
		base = 6
	}
	if freqMHz > 1500 {
		base *= 1.5
	}
	return base
}
