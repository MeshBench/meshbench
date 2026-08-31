// Package coverage turns link budgets into maps.
//
// The unit of answer is a *pair* of margins, never one. A raster that says
// "covered" without saying which direction is wrong even when the arithmetic is
// right: a 22 dBm repeater on a mast reaching a 14 dBm handheld at 1.5 m is not
// the same link back again, and the case worth showing an operator is precisely
// the one where they can hear it and it cannot hear them.
//
// Both of those margins are a best case, and each cell says by how much: a
// station imported at +/-5 km is priced where it was reported and carries the
// decibels that placement could cost, so the verdicts are taken at the
// pessimistic end. A cell that is only served when a guess turns out to be
// right is not served.
package coverage

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
)

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

	// UncertaintyKm is the radius this end's position is good to, carried from
	// whatever imported it. The imagined station at each cell has none of its
	// own: it is the point being asked about, and it is exactly there.
	UncertaintyKm float64
}

// Cell is one raster cell's verdict.
type Cell struct {
	// OutboundMarginDB is the margin at the remote end for the fixed station's
	// transmission; InboundMarginDB is the margin back at the fixed station.
	// Positive is workable. Both are the best case: the fixed station exactly
	// where it was reported to be.
	OutboundMarginDB float64
	InboundMarginDB  float64

	// PositionSlackDB is how much of either margin the fixed station's position
	// uncertainty could take away. One figure serves both directions because it
	// is a property of the path and not of an end - and it is subtracted from
	// two different margins, so the two directions still answer separately,
	// which is the only way a cell is allowed to answer.
	PositionSlackDB float64

	// PathLossDB is the total, including diffraction. Kept so a cell can
	// explain itself without recomputing.
	PathLossDB float64

	// NoData marks a cell the terrain could not answer for. Distinct from "no
	// coverage" — one is ignorance and the other is a result.
	NoData bool
}

// OutboundWorstCaseDB and InboundWorstCaseDB are the two margins with the
// position uncertainty going the wrong way. Two methods rather than one,
// because the pair is the answer: an uncertain link that fails outbound and an
// uncertain link that fails inbound need different work done to them.
func (c Cell) OutboundWorstCaseDB() float64 { return c.OutboundMarginDB - c.PositionSlackDB }

// InboundWorstCaseDB is the inbound half of that pair.
func (c Cell) InboundWorstCaseDB() float64 { return c.InboundMarginDB - c.PositionSlackDB }

// WorstCaseDB is the weaker direction at the pessimistic end of its band: the
// one number to sort or colour a cell by, where one number is all there is
// room for.
func (c Cell) WorstCaseDB() float64 {
	return math.Min(c.OutboundWorstCaseDB(), c.InboundWorstCaseDB())
}

// Workable reports whether the link closes in both directions even with the
// positions as wrong as they are allowed to be.
//
// Pessimistic on purpose. A cell that is only served when an imported position
// happens to be exact is not served: it is a guess with a colour on it.
func (c Cell) Workable() bool {
	return !c.NoData && c.OutboundWorstCaseDB() >= 0 && c.InboundWorstCaseDB() >= 0
}

// WorkableIfExact is the same question with the positions taken at face value.
// It can only ever be the more generous of the two, and where it disagrees with
// Workable the cell is worth exactly as much as the survey behind it.
func (c Cell) WorkableIfExact() bool {
	return !c.NoData && c.OutboundMarginDB >= 0 && c.InboundMarginDB >= 0
}

// OneWay reports the case worth drawing differently from every other: the link
// closes one way and not the other.
func (c Cell) OneWay() bool {
	if c.NoData {
		return false
	}
	return (c.OutboundWorstCaseDB() >= 0) != (c.InboundWorstCaseDB() >= 0)
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
func Compute(fixed Endpoint, t propagation.Terrain, r *Raster, o Options) error {
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

func evaluate(fixed Endpoint, fixedGround, lat, lon float64, t propagation.Terrain, freqMHz float64, o Options) Cell {
	remoteGround, ok := t.ElevationM(lat, lon)
	if !ok {
		return Cell{NoData: true}
	}

	distKm := geo.DistanceKm(fixed.Lat, fixed.Lon, lat, lon)
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
	bearing := geo.BearingDeg(fixed.Lat, fixed.Lon, lat, lon)
	elevation := geo.ElevationDeg(txAlt, rxAlt, distKm)

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
		// Nothing from the remote end: the cell is the question, so it is
		// exactly where it is being asked about.
		PositionSlackDB: linkbudget.PositionSlackDB(distKm, fixed.UncertaintyKm, 0),
		PathLossDB:      loss,
	}
}

// sampleProfile walks the great circle between two points.
func sampleProfile(t propagation.Terrain, lat1, lon1, lat2, lon2, distKm, stepM float64) ([]terrain.Point, bool) {
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

// LossBetween is the path loss between two points, over the terrain profile
// between them.
//
// Exported because a raster is not the only thing that needs a path loss: a
// route search asks the same question about a mast that does not exist yet.
// Written here, next to evaluate, and using the same profile and the same two
// terms, so that a planned link and a drawn one cannot disagree about the
// propagation while agreeing about everything else.
//
// Reports false where the terrain has no data, which is not the same as a
// large loss: one is ignorance and the other is a result.
func LossBetween(t propagation.Terrain, aLat, aLon, aHeightAGLm, bLat, bLon, bHeightAGLm,
	freqMHz, profileStepM float64) (float64, bool) {

	if _, ok := t.ElevationM(aLat, aLon); !ok {
		return 0, false
	}
	if _, ok := t.ElevationM(bLat, bLon); !ok {
		return 0, false
	}
	distKm := geo.DistanceKm(aLat, aLon, bLat, bLon)
	if distKm <= 0 {
		return 0, false
	}
	if profileStepM <= 0 {
		profileStepM = 120
	}
	profile, ok := sampleProfile(t, aLat, aLon, bLat, bLon, distKm, profileStepM)
	if !ok {
		return 0, false
	}
	// Heights above ground, not absolute altitudes: MultiEdgeLossDB adds the
	// profile's own endpoint elevations itself, and passing absolute values
	// counts the ground twice and buys clearance the path does not have.
	return terrain.FSPLdB(distKm, freqMHz) +
		terrain.MultiEdgeLossDB(profile, aHeightAGLm, bHeightAGLm, freqMHz), true
}
