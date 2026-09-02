// Pricing what stands on the ground, for any path.
//
// Extracted from the engine so the coverage rasters and the link budgets
// price a building with the same arithmetic - a roof that costs a packet
// 12 dB must cost the coverage map the same 12 dB, or the map lies about
// the network it claims to show. Everything here is reached through
// priceObstructions, and the aperture below is part of the price rather
// than part of any one caller's search, so a walk that finds more crossings
// than another still returns the same decibels.
package environ

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// multiScreenDB is how fast rows of buildings behind the first one add up.
//
// Walfisch and Bertoni (1988) solved the field reaching a street after N
// rows of uniform screens and found it settles as N^-0.9, so the rows behind
// the leading one cost 18 log10(N) dB however many of them there are. COST
// 231's multi-screen diffraction term carries the same exponent as its
// k_d log10(d) distance dependence, and that model - COST 231
// Walfisch-Ikegami - is stated valid over 800 to 2000 MHz for base antennas
// of 4 to 50 m, mobiles of 1 to 3 m and paths of 0.02 to 5 km. A 869 MHz
// mesh over UK housing sits inside its frequency and both its height ranges;
// long links run outside its distance range, which is why only the exponent
// is borrowed and the leading screen keeps its own ITU-R P.526 knife edge
// rather than COST 231's fitted 54 dB constant. A building and a ridge of
// equal height still cost the same, which is the property that made the
// knife edge the right tool for a rooftop in the first place.
//
// The exponent is also what suits this dataset. Microsoft's ML footprints
// publish no heights in the United Kingdom, so every building here stands at
// envgen's 6 m default at confidence 0.3. A term driven by how many rows the
// ray crosses rests on data that was actually surveyed; a term driven by
// rooftop height would rest on a constant. Height still decides whether a
// screen shadows at all, so the model reads as "are the antennas below
// roof level", which is the most a uniform 6 m can honestly answer.
const multiScreenDB = 18

// apertureM is how far from each end of a path a rooftop is priced.
//
// Fresnel arithmetic is the justification, and it is the same one the raster
// was already relying on: v scales with sqrt(d/(d1*d2)), so a rooftop's few
// metres mid-way across tens of kilometres prices at a decibel or two, while
// the same roof beside an antenna prices at ten or twenty. Making it a rule
// of the price rather than of one caller's search is what stops the engine
// and the coverage raster answering differently: the raster walks only the
// ends because it cannot afford more, and now the engine, which walks
// everything, charges for exactly the same set.
const apertureM = 3000

// tallM is where that argument stops holding. "A rooftop's few metres" is an
// argument about houses, and a structure this tall mid-path is not a rooftop
// but an obstacle: 20 m above the ray at the midpoint of a 25 km hop still
// reaches v = 0.6 and eleven decibels. So the price charges these wherever
// they stand, and
// the index keeps them separately so the walks that skip the middle of a path
// can afford to look for them. In practice they are rare, and in the United
// Kingdom footprint data, which publishes no heights at all, there are none.
const tallM = 20

// insideDBPerM is what a metre of building interior costs a signal already
// through the wall. 3GPP TR 38.901's outdoor-to-indoor model charges exactly
// this for its indoor depth, and it is the interior rather than the fabric:
// a room is mostly air, so the per-metre figure is nothing like the wall's.
const insideDBPerM = 0.5

// grazingDB is ITU-R P.526's knife-edge loss at v = 0, where the ray just
// reaches the rooftops. Below that the ray is clear of them and no settled
// field builds up behind, so a screen joins the multi-screen count in
// proportion to how far past grazing it is. Fading it in rather than
// switching it on keeps the loss continuous in geometry, and a step here
// would draw an edge across a coverage raster that no town put there.
var grazingDB = terrain.KnifeEdgeDB(0)

// PathBuildingLossDB is what the buildings along one path cost it: the
// leading rooftop as a knife edge, the rows behind it as a settled field,
// and the walls as the alternative route through rather than over. txAslM
// and rxAslM are the antenna altitudes above sea level; totalM the path
// length.
func PathBuildingLossDB(p Provider, g Ground,
	aLat, aLon, txAslM, bLat, bLon, rxAslM, totalM, freqMHz float64) float64 {
	if p == nil || totalM <= 0 {
		return 0
	}
	obs := ObstructionsOnPath(p, g, aLat, aLon, bLat, bLon)
	return priceObstructions(obs, txAslM, rxAslM, totalM, freqMHz)
}

// priceObstructions is the one place a set of crossings becomes decibels.
func priceObstructions(obs []Obstruction, txAslM, rxAslM, totalM, freqMHz float64) float64 {
	if len(obs) == 0 || totalM <= 0 {
		return 0
	}
	var principal, screens, walls float64
	for _, o := range obs {
		midFrac := (o.EnterFrac + o.ExitFrac) / 2
		d1 := totalM * midFrac
		d2 := totalM - d1
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		if d1 > apertureM && d2 > apertureM && o.HeightM < tallM {
			continue
		}
		rayM := txAslM + (rxAslM-txAslM)*midFrac
		h := o.TopM - rayM
		edge := terrain.KnifeEdgeDB(terrain.FresnelParameter(h, d1, d2, freqMHz))
		if edge <= 0 {
			continue
		}
		principal = math.Max(principal, edge)
		screens += math.Min(1, edge/grazingDB)
		if h > 0 {
			// Through rather than over means both walls, going in and
			// coming out, plus what the inside costs. Depth is what
			// separates a house from a tower block: without it one building
			// could never cost more than two of its own walls however far
			// the ray had to travel inside it, and the parallel combination
			// below would cap a concrete slab at twenty decibels.
			walls += 2*MaterialLossDB(o.Material, freqMHz) +
				insideDBPerM*(o.ExitFrac-o.EnterFrac)*totalM
		}
	}
	if principal <= 0 {
		return 0
	}
	overRoof := principal + multiScreenDB*math.Max(0, math.Log10(screens))
	if walls <= 0 {
		return overRoof
	}
	// Over the roofs and through the walls are two routes the field takes at
	// once, so they combine in power the way ITU-R P.452 combines the
	// mechanisms it models side by side. Summing them instead would charge a
	// signal for a route it did not take.
	return -10 * math.Log10(math.Pow(10, -overRoof/10)+math.Pow(10, -walls/10))
}
