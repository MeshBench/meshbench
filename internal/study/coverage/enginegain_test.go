package coverage

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// flatGround is terrain at one height, so the only thing setting the look
// angle is how high the two antennas are.
type flatGround struct{ h float64 }

func (f flatGround) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

// The map and the packets have to agree about the same link.
//
// A raster has always evaluated an antenna in the true direction of the cell
// it is pricing; the engine charged peak gain regardless. So a repeater on a
// hill was drawn one way and simulated another, and the difference on a steep
// path was several decibels - enough to put a node inside the coverage and
// outside the run.
func TestTheEngineAndARasterCellAgreeOnGain(t *testing.T) {
	const (
		lat, lon      = 56.700, -3.900
		remoteLat     = 56.740
		remoteLon     = -3.820
		groundM       = 100.0
		fixedHeightM  = 45.0
		remoteHeightM = 2.0
		peakDBi       = 9.0
		feedlineDB    = 1.5
		boresightDeg  = 20.0
		downtiltDeg   = 2.0
	)
	mounted := antenna.Mounted{
		Pattern:      antenna.Collinear{GainDBiPeak: peakDBi},
		BearingDeg:   boresightDeg,
		DowntiltDeg:  downtiltDeg,
		Polarisation: antenna.Vertical,
		FeedlineDB:   feedlineDB,
	}

	e := engine.New(flatGround{groundM}, engine.Config{StepMs: 10})
	spec := func(name string, la, lo, h float64) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: la, Lon: lo}, HeightAGLm: h,
			Radio: scenario.RadioConfig{
				CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
			},
			TxPowerDBm: 22, NoiseFigureDB: 6, Antenna: mounted,
		}
	}
	e.Add(spec("fixed", lat, lon, fixedHeightM), nil)
	e.Add(spec("remote", remoteLat, remoteLon, remoteHeightM), nil)
	fromEngine, _ := e.LinkGainsDBiForTest(0, 1)

	// The same gain out of a raster cell, isolated: with every power, every
	// far-end gain and every sensitivity at zero and no path loss, the
	// outbound margin is the fixed station's own gain and nothing else.
	distKm := geo.DistanceKm(lat, lon, remoteLat, remoteLon)
	cell := cellFromLoss(Endpoint{
		Name: "fixed", Lat: lat, Lon: lon, HeightAGLm: fixedHeightM,
		GainTowardsDBi: mounted.GainTowardsDBi,
	}, groundM, groundM, remoteLat, remoteLon, distKm, 0, Options{
		RemoteHeightAGLm: remoteHeightM,
	})

	if math.Abs(fromEngine-cell.OutboundMarginDB) > 1e-9 {
		t.Fatalf("the engine charges %.4f dBi and the raster %.4f dBi for the "+
			"same antenna on the same path", fromEngine, cell.OutboundMarginDB)
	}
	if math.Abs(fromEngine-(peakDBi-feedlineDB)) < 0.1 {
		t.Fatalf("both came out at the peak (%.4f dBi), so the two agreeing "+
			"proves nothing about direction", fromEngine)
	}
}
