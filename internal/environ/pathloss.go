// Pricing what stands on the ground, for any path.
//
// Extracted from the engine so the coverage rasters and the link budgets
// price a building with the same arithmetic - a roof that costs a packet
// 12 dB must cost the coverage map the same 12 dB, or the map lies about
// the network it claims to show.
package environ

import (
	"github.com/MeshBench/meshbench/internal/terrain"
)

// PathBuildingLossDB is what the buildings along one path cost it: knife-edge
// diffraction over each rooftop the ray must clear, plus one wall of material
// loss per building the ray actually passes through. txAslM and rxAslM are
// the antenna altitudes above sea level; totalM the path length.
func PathBuildingLossDB(p Provider, g Ground,
	aLat, aLon, txAslM, bLat, bLon, rxAslM, totalM, freqMHz float64) float64 {
	if p == nil || totalM <= 0 {
		return 0
	}
	obs := ObstructionsOnPath(p, g, aLat, aLon, bLat, bLon)
	if len(obs) == 0 {
		return 0
	}
	loss := 0.0
	for _, o := range obs {
		midFrac := (o.EnterFrac + o.ExitFrac) / 2
		rayM := txAslM + (rxAslM-txAslM)*midFrac
		// The rooftop as a knife edge at its position along the path - the
		// same ITU-R P.526 arithmetic terrain uses, so a building and a
		// ridge of equal height cost the same, as they should.
		d1 := totalM * midFrac
		d2 := totalM - d1
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		h := o.TopM - rayM
		v := terrain.FresnelParameter(h, d1, d2, freqMHz)
		loss += terrain.KnifeEdgeDB(v)
		// And the walls, when the ray goes through rather than over.
		if o.TopM > rayM {
			loss += MaterialLossDB(o.Material, freqMHz)
		}
	}
	return loss
}
