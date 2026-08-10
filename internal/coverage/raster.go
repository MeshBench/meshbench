// Package coverage turns link budgets into maps.
//
// The unit of answer is a *pair* of margins, never one. A raster that says
// "covered" without saying which direction is wrong even when the arithmetic is
// right: a 22 dBm repeater on a mast reaching a 14 dBm handheld at 1.5 m is not
// the same link back again, and the case worth showing an operator is precisely
// the one where they can hear it and it cannot hear them.
package coverage

import (
	"fmt"
	"math"

	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// Terrain supplies ground elevation. An interface because the raster does not
// care whether the heights came from a downloaded tile, a cache or a test.
type Terrain interface {
	// ElevationM returns metres above sea level, and whether the point is
	// covered by data at all. A raster over a coastline asks for points that no
	// tile covers, and inventing zero for them would draw sea-level coverage
	// across the Atlantic.
	ElevationM(lat, lon float64) (float64, bool)
}

// Endpoint is one end of a link.
type Endpoint struct {
	Name       string
	Lat, Lon   float64
	HeightAGLm float64
	TxPowerDBm float64
	// AntennaGainDBi in the direction of the far end, and the feedline loss
	// already deducted. Directional gain is evaluated per cell by the caller,
	// because a Yagi's gain towards one cell is not its gain towards another.
	GainTowardsDBi func(bearingDeg, elevationDeg float64) float64
	// SensitivityDBm is what this end needs to decode.
	SensitivityDBm float64
}

// Cell is one raster cell's verdict.
type Cell struct {
	// OutboundMarginDB is the margin at the remote end for the fixed station's
	// transmission; InboundMarginDB is the margin back at the fixed station.
	// Positive is workable.
	OutboundMarginDB float64
	InboundMarginDB  float64

	// PathLossDB is the total, including diffraction. Kept so a cell can
	// explain itself without recomputing.
	PathLossDB float64

	// NoData marks a cell the terrain could not answer for. Distinct from "no
	// coverage" — one is ignorance and the other is a result.
	NoData bool
}

// Workable reports whether the link closes in both directions.
func (c Cell) Workable() bool {
	return !c.NoData && c.OutboundMarginDB >= 0 && c.InboundMarginDB >= 0
}

// OneWay reports the case worth drawing differently from every other: the link
// closes one way and not the other.
func (c Cell) OneWay() bool {
	if c.NoData {
		return false
	}
	return (c.OutboundMarginDB >= 0) != (c.InboundMarginDB >= 0)
}

// Raster is a grid of verdicts in a lat/lon box.
type Raster struct {
	South, North, West, East float64
	Width, Height            int
	Cells                    []Cell
	FreqMHz                  float64
}

// At returns the cell at a pixel. Row 0 is the *north* edge, matching how the
// grid is drawn rather than how latitude increases.
func (r *Raster) At(x, y int) Cell { return r.Cells[y*r.Width+x] }

// LatLonAt is the geographic centre of a pixel.
func (r *Raster) LatLonAt(x, y int) (lat, lon float64) {
	lat = r.North - (float64(y)+0.5)*(r.North-r.South)/float64(r.Height)
	lon = r.West + (float64(x)+0.5)*(r.East-r.West)/float64(r.Width)
	return lat, lon
}

// Options control a computation.
type Options struct {
	// RemoteHeightAGLm is how high the imagined station at each cell is. This
	// is per raster, not global: a coverage map for handhelds at 1.5 m and one
	// for a vehicle whip at 2.5 m are different maps, and the difference is
	// large enough that mixing them is a real error.
	RemoteHeightAGLm float64
	RemoteTxPowerDBm float64
	RemoteGainDBi    float64
	// RemoteSensitivityDBm is what the imagined station needs to decode.
	RemoteSensitivityDBm float64

	// ProfileStepM is the terrain sampling interval along each path. Too coarse
	// and a ridge is stepped over entirely; 30 m matches the usual DEM.
	ProfileStepM float64
}

// Compute evaluates a raster.
//
// One profile per cell, both directions from the same profile — the terrain
// between two points is the same whichever way the signal travels, so the
// expensive part is shared and only the budgets differ.
func Compute(fixed Endpoint, t Terrain, r *Raster, o Options) error {
	if r.Width <= 0 || r.Height <= 0 {
		return fmt.Errorf("coverage: raster is %dx%d", r.Width, r.Height)
	}
	if o.ProfileStepM <= 0 {
		o.ProfileStepM = 30
	}
	fixedGround, ok := t.ElevationM(fixed.Lat, fixed.Lon)
	if !ok {
		return fmt.Errorf("coverage: no terrain at %s (%.5f, %.5f)", fixed.Name, fixed.Lat, fixed.Lon)
	}

	r.Cells = make([]Cell, r.Width*r.Height)
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			lat, lon := r.LatLonAt(x, y)
			r.Cells[y*r.Width+x] = evaluate(fixed, fixedGround, lat, lon, t, r.FreqMHz, o)
		}
	}
	return nil
}

func evaluate(fixed Endpoint, fixedGround, lat, lon float64, t Terrain, freqMHz float64, o Options) Cell {
	remoteGround, ok := t.ElevationM(lat, lon)
	if !ok {
		return Cell{NoData: true}
	}

	distKm := haversineKm(fixed.Lat, fixed.Lon, lat, lon)
	if distKm <= 0 {
		// The cell containing the station itself. Zero distance has no defined
		// path loss, and reporting a huge margin there would put a bright spot
		// at every station regardless of whether it reaches anywhere.
		return Cell{OutboundMarginDB: 0, InboundMarginDB: 0}
	}

	profile, okProfile := sampleProfile(t, fixed.Lat, fixed.Lon, lat, lon, distKm, o.ProfileStepM)
	if !okProfile {
		return Cell{NoData: true}
	}

	// MultiEdgeLossDB takes heights *above ground* and adds the profile's own
	// endpoint elevations itself. Passing absolute altitudes counts the ground
	// twice, which lifts both ends by the terrain height and quietly buys the
	// path clearance it does not have.
	loss := terrain.FSPLdB(distKm, freqMHz) +
		terrain.MultiEdgeLossDB(profile, fixed.HeightAGLm, o.RemoteHeightAGLm, freqMHz)

	return cellFromLoss(fixed, fixedGround, remoteGround, lat, lon, distKm, loss, o)
}

// cellFromLoss turns one path loss into a cell: gains, powers and margins,
// both directions. Shared by the tile-walking path and the grid/GPU path, so
// the two cannot disagree about anything except where the loss came from.
func cellFromLoss(fixed Endpoint, fixedGround, remoteGround, lat, lon, distKm, loss float64, o Options) Cell {
	txAlt := fixedGround + fixed.HeightAGLm
	rxAlt := remoteGround + o.RemoteHeightAGLm
	bearing := bearingDeg(fixed.Lat, fixed.Lon, lat, lon)
	elevation := math.Atan2(rxAlt-txAlt, distKm*1000) * 180 / math.Pi

	fixedGain := 0.0
	if fixed.GainTowardsDBi != nil {
		fixedGain = fixed.GainTowardsDBi(bearing, elevation)
	}

	// Both directions from one profile. The asymmetry lives entirely in the
	// powers, gains and sensitivities — the terrain is the same either way.
	outboundRx := fixed.TxPowerDBm + fixedGain - loss + o.RemoteGainDBi
	inboundRx := o.RemoteTxPowerDBm + o.RemoteGainDBi - loss + fixedGain

	return Cell{
		OutboundMarginDB: outboundRx - o.RemoteSensitivityDBm,
		InboundMarginDB:  inboundRx - fixed.SensitivityDBm,
		PathLossDB:       loss,
	}
}

// ComputeFromLosses fills a raster from a precomputed loss field — the GPU
// path. The losses come from CoverageGridLoss (or its CPU twin); this applies
// the same gains and margins the tile path applies.
func ComputeFromLosses(fixed Endpoint, g HeightGrid, losses []float32, r *Raster, o Options) error {
	if len(losses) != r.Width*r.Height {
		return fmt.Errorf("coverage: %d losses for a %dx%d raster", len(losses), r.Width, r.Height)
	}
	fixedGround, ok := g.At(fixed.Lat, fixed.Lon)
	if !ok {
		return fmt.Errorf("coverage: no terrain at %s (%.5f, %.5f)", fixed.Name, fixed.Lat, fixed.Lon)
	}
	r.Cells = make([]Cell, r.Width*r.Height)
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			i := y*r.Width + x
			loss := float64(losses[i])
			if loss > 1e30 {
				r.Cells[i] = Cell{NoData: true}
				continue
			}
			lat, lon := r.LatLonAt(x, y)
			distKm := haversineKm(fixed.Lat, fixed.Lon, lat, lon)
			if distKm <= 0 {
				r.Cells[i] = Cell{}
				continue
			}
			remoteGround, ok := g.At(lat, lon)
			if !ok {
				r.Cells[i] = Cell{NoData: true}
				continue
			}
			r.Cells[i] = cellFromLoss(fixed, fixedGround, remoteGround, lat, lon, distKm, loss, o)
		}
	}
	return nil
}

// sampleProfile walks the great circle between two points.
func sampleProfile(t Terrain, lat1, lon1, lat2, lon2, distKm, stepM float64) ([]terrain.Point, bool) {
	n := int(distKm * 1000 / stepM)
	if n < 2 {
		n = 2
	}
	// A very long path at a fine step is a lot of samples for one cell, and a
	// raster has thousands of cells. Capping the count coarsens long paths
	// rather than refusing them, which is the right trade: a 100 km path does
	// not need 30 m resolution to find the ridge that blocks it.
	const maxSamples = 512
	if n > maxSamples {
		n = maxSamples
	}

	profile := make([]terrain.Point, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		lat := lat1 + (lat2-lat1)*f
		lon := lon1 + (lon2-lon1)*f
		h, ok := t.ElevationM(lat, lon)
		if !ok {
			return nil, false
		}
		profile[i] = terrain.Point{DistM: f * distKm * 1000, HeightM: h}
	}
	return profile, true
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	y := math.Sin((lon2-lon1)*rad) * math.Cos(lat2*rad)
	x := math.Cos(lat1*rad)*math.Sin(lat2*rad) -
		math.Sin(lat1*rad)*math.Cos(lat2*rad)*math.Cos((lon2-lon1)*rad)
	b := math.Atan2(y, x) / rad
	if b < 0 {
		b += 360
	}
	return b
}
