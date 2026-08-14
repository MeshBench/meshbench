// Package validate compares what the model predicts against what was actually
// heard.
//
// This is the only thing in the project that can tell you whether any of it is
// true. Every other component is checked against a published reference —
// Semtech's sensitivity figures, ITU-R P.526, RadioLib's airtime — and a model
// can agree with every textbook it was built from and still be wrong about a
// hillside in Perthshire.
//
// Two rules run through it.
//
// A node that did not report hearing a packet is *not* evidence that it could
// not hear it. It may not be an observer, it may have been transmitting, it may
// have been off. Treating silence as a negative observation is the single
// easiest way to manufacture a confident, wrong calibration, so silence is
// counted separately and never mixed into the residuals.
//
// And a residual is only meaningful if the position it was computed from is
// meaningful. A reception from a node placed at ±5 km cannot constrain a model
// to a decibel, so uncertainty gates what a sample is allowed to influence.
package validate

import (
	"fmt"
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/provider"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// Station is a node with everything needed to price a link to or from it.
type Station struct {
	Name          string
	Lat, Lon      float64
	UncertaintyKm float64
	HeightAGLm    float64
	TxPowerDBm    float64
	GainDBi       float64
	NoiseFigureDB float64
}

// Params describe the radio configuration the observations were made under.
type Params struct {
	FreqMHz      float64
	SF           int
	BandwidthHz  float64
	ProfileStepM float64

	// MaxUncertaintyKm excludes samples whose endpoints are too loosely placed
	// to say anything. Default 1 km: at 869 MHz over open ground, a kilometre of
	// position error is already several dB of path loss.
	MaxUncertaintyKm float64
}

// Residual is one comparison: what the model said against what was heard.
type Residual struct {
	From, To   string
	PacketID   string
	DistanceKm float64

	// PredictedSNRdB is what the model expects at the receiver; ObservedSNRdB is
	// what the receiver reported.
	PredictedSNRdB float64
	ObservedSNRdB  float64

	// ResidualDB is observed minus predicted. Positive means reality was better
	// than the model — the model was pessimistic on this path.
	ResidualDB float64

	PathLossDB float64
}

// Report is the summary that decides whether the model can be trusted.
type Report struct {
	Residuals []Residual

	// MeanDB and MedianDB are the bias. A negative mean means the model
	// predicts *more* signal than really arrived: it is optimistic, which is the
	// dangerous direction and the one docs/shortcomings.md expects.
	MeanDB   float64
	MedianDB float64
	StdDevDB float64
	RMSEdB   float64

	// P10DB and P90DB bracket the spread. The spread matters more than the bias:
	// a constant bias can be calibrated out, and scatter cannot.
	P10DB, P90DB float64

	// Used, and why the rest were not. Reported because a validation run that
	// silently discards 90% of its input looks exactly like one that did not.
	Used              int
	SkippedNoSNR      int
	SkippedNoPosition int
	SkippedUncertain  int
	SkippedNoTerrain  int

	// SilentReceivers counts observer nodes that did not report a packet that
	// others did. Never used as evidence — reported so the shape of the
	// evidence is visible.
	SilentReceivers int
}

// Compare evaluates a set of receptions against the model.
//
// Receptions are grouped by packet: every reception of one packet shares an
// origin, so each becomes one origin-to-receiver residual.
func Compare(rx []provider.Reception, stations map[string]Station, t coverage.Terrain, p Params) (Report, error) {
	if p.FreqMHz <= 0 {
		return Report{}, fmt.Errorf("validate: no frequency given")
	}
	if p.SF < 5 || p.SF > 12 {
		return Report{}, fmt.Errorf("validate: spreading factor %d is outside SF5-SF12", p.SF)
	}
	if p.BandwidthHz <= 0 {
		p.BandwidthHz = 125_000
	}
	if p.MaxUncertaintyKm <= 0 {
		p.MaxUncertaintyKm = 1.0
	}
	if p.ProfileStepM <= 0 {
		p.ProfileStepM = 30
	}

	var rep Report
	byPacket := map[string][]provider.Reception{}
	for _, r := range rx {
		if r.PacketID == "" {
			continue
		}
		byPacket[r.PacketID] = append(byPacket[r.PacketID], r)
	}

	for _, group := range byPacket {
		origin := originOf(group)
		if origin == "" {
			continue
		}
		src, ok := stations[origin]
		if !ok {
			rep.SkippedNoPosition += len(group)
			continue
		}

		heard := map[string]bool{}
		for _, r := range group {
			heard[r.Receiver] = true
		}
		// Everything that could have heard it and did not. Counted, never used.
		for name := range stations {
			if name != origin && !heard[name] {
				rep.SilentReceivers++
			}
		}

		for _, r := range group {
			if !r.HasSNR {
				rep.SkippedNoSNR++
				continue
			}
			dst, ok := stations[r.Receiver]
			if !ok {
				rep.SkippedNoPosition++
				continue
			}
			if src.UncertaintyKm > p.MaxUncertaintyKm || dst.UncertaintyKm > p.MaxUncertaintyKm {
				rep.SkippedUncertain++
				continue
			}

			res, ok := residual(src, dst, r, t, p)
			if !ok {
				rep.SkippedNoTerrain++
				continue
			}
			rep.Residuals = append(rep.Residuals, res)
		}
	}

	summarise(&rep)
	return rep, nil
}

// originOf finds the packet's origin from whichever reception recorded it.
// Sources disagree about whether they carry it at all, so the first one that
// does wins rather than requiring all of them to.
func originOf(group []provider.Reception) string {
	for _, r := range group {
		if r.Origin != "" {
			return r.Origin
		}
	}
	return ""
}

func residual(src, dst Station, r provider.Reception, t coverage.Terrain, p Params) (Residual, bool) {
	if _, ok := t.ElevationM(src.Lat, src.Lon); !ok {
		return Residual{}, false
	}
	if _, ok := t.ElevationM(dst.Lat, dst.Lon); !ok {
		return Residual{}, false
	}

	distKm := haversineKm(src.Lat, src.Lon, dst.Lat, dst.Lon)
	if distKm <= 0 {
		return Residual{}, false
	}

	profile, ok := sampleProfile(t, src, dst, distKm, p.ProfileStepM)
	if !ok {
		return Residual{}, false
	}
	loss := terrain.FSPLdB(distKm, p.FreqMHz) +
		terrain.MultiEdgeLossDB(profile, src.HeightAGLm, dst.HeightAGLm, p.FreqMHz)

	rxDBm := src.TxPowerDBm + src.GainDBi - loss + dst.GainDBi
	noiseDBm := dsp.NoiseFloorDBm(p.BandwidthHz, dst.NoiseFigureDB)
	predictedSNR := rxDBm - noiseDBm

	return Residual{
		From: src.Name, To: dst.Name, PacketID: r.PacketID,
		DistanceKm:     distKm,
		PredictedSNRdB: predictedSNR,
		ObservedSNRdB:  r.SNRdB,
		ResidualDB:     r.SNRdB - predictedSNR,
		PathLossDB:     loss,
	}, true
}

func sampleProfile(t coverage.Terrain, src, dst Station, distKm, stepM float64) ([]terrain.Point, bool) {
	n := int(distKm * 1000 / stepM)
	if n < 2 {
		n = 2
	}
	if n > 512 {
		n = 512
	}
	out := make([]terrain.Point, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		h, ok := t.ElevationM(src.Lat+(dst.Lat-src.Lat)*f, src.Lon+(dst.Lon-src.Lon)*f)
		if !ok {
			return nil, false
		}
		out[i] = terrain.Point{DistM: f * distKm * 1000, HeightM: h}
	}
	return out, true
}

func summarise(rep *Report) {
	rep.Used = len(rep.Residuals)
	if rep.Used == 0 {
		return
	}
	vals := make([]float64, rep.Used)
	var sum, sumsq float64
	for i, r := range rep.Residuals {
		vals[i] = r.ResidualDB
		sum += r.ResidualDB
		sumsq += r.ResidualDB * r.ResidualDB
	}
	rep.MeanDB = sum / float64(rep.Used)
	rep.RMSEdB = math.Sqrt(sumsq / float64(rep.Used))
	if rep.Used > 1 {
		var acc float64
		for _, v := range vals {
			d := v - rep.MeanDB
			acc += d * d
		}
		rep.StdDevDB = math.Sqrt(acc / float64(rep.Used-1))
	}

	sort.Float64s(vals)
	rep.MedianDB = percentile(vals, 0.5)
	rep.P10DB = percentile(vals, 0.10)
	rep.P90DB = percentile(vals, 0.90)
}

// percentile on already-sorted values, linearly interpolated.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

// Verdict states what the report means, in the terms that matter.
//
// Bias and scatter are different problems with different remedies: a constant
// bias can be calibrated out and scatter cannot, so a model with 1 dB of bias
// and 12 dB of spread is in much worse shape than one with 6 dB of bias and
// 3 dB of spread, even though the second looks worse at a glance.
func (r Report) Verdict() string {
	if r.Used == 0 {
		return "No usable observations. Nothing here says anything about the model."
	}
	direction := "pessimistic — reality was better than predicted"
	if r.MeanDB < 0 {
		direction = "OPTIMISTIC — the model predicted more signal than arrived, which is the dangerous direction"
	}
	return fmt.Sprintf(
		"%d observations. Bias %+.1f dB (%s), spread %.1f dB standard deviation, "+
			"10th to 90th percentile %+.1f to %+.1f dB.\n\n"+
			"Bias can be calibrated out; spread cannot. %d receptions were skipped "+
			"(%d no SNR, %d no position, %d too uncertain, %d no terrain), and %d "+
			"observer-silences were counted but not used as evidence — a node that did "+
			"not report a packet is not evidence that it could not hear one.",
		r.Used, r.MeanDB, direction, r.StdDevDB, r.P10DB, r.P90DB,
		r.SkippedNoSNR+r.SkippedNoPosition+r.SkippedUncertain+r.SkippedNoTerrain,
		r.SkippedNoSNR, r.SkippedNoPosition, r.SkippedUncertain, r.SkippedNoTerrain,
		r.SilentReceivers)
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const rEarth = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * rEarth * math.Asin(math.Min(1, math.Sqrt(a)))
}
