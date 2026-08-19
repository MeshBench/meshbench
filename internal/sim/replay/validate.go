package replay

import (
	"fmt"
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/world/provider"
)

// Predict is the model's answer for one directed link: the SNR the receiver
// would see for the origin's transmission, and whether it would decode.
// ok=false means the model cannot answer (no terrain, unknown node) and the
// pair is excluded rather than guessed.
type Predict func(origin, receiver string) (snrDB float64, decodes, ok bool)

// ResidualRow is one comparison of the model against one real reception —
// the table from ADR-0015, one row per (packet, receiver).
type ResidualRow struct {
	PacketID string
	Origin   string
	Receiver string

	Observed    bool // the real network heard it
	ObservedSNR float64
	HasSNR      bool

	PredictedSNR    float64
	PredictedDecode bool

	// ResidualDB is predicted minus observed, meaningful only when both sides
	// heard. Positive means the model was optimistic — the expected direction,
	// since the channel omits multipath, body loss and oscillator error.
	ResidualDB float64

	Verdict string
}

// Report is the validation result, with its own caveats attached: sample size
// and exclusions travel with the agreement figure, because "agrees within
// 4 dB" over eleven links from three observers is a hint, not a result.
type Report struct {
	Rows []ResidualRow

	// Samples is how many rows have both a prediction and an observed SNR —
	// the rows the bias and percentile are computed over.
	Samples     int
	MeanBiasDB  float64
	P90AbsDB    float64
	Optimistic  int // model decoded, reality heard nothing
	Pessimistic int // reality heard it, model said no

	// Excluded counts what was left out, by reason. Published rather than
	// dropped: a validation set with silent exclusions is worse than none.
	Excluded map[string]int
}

// Summary is the one-line agreement statement the tool can earn.
func (r Report) Summary() string {
	if r.Samples == 0 {
		return "no comparable observations - nothing was both predicted and measured"
	}
	return fmt.Sprintf("%d comparisons: bias %+.1f dB (positive = model optimistic), "+
		"90%% within %.1f dB; %d optimistic misses, %d pessimistic",
		r.Samples, r.MeanBiasDB, r.P90AbsDB, r.Optimistic, r.Pessimistic)
}

// Compare replays observed receptions against the model.
//
// onlineObservers are the nodes known to have been listening during the
// session — an observer absent from a packet's hearers is evidence of
// non-reception only if it was online at all. Survivorship is the dataset's
// deepest bias and this is the only guard available against it.
func Compare(rx []provider.Reception, predict Predict, onlineObservers []string) Report {
	rep := Report{Excluded: map[string]int{}}

	type group struct {
		origin  string
		heardBy map[string]provider.Reception
	}
	groups := map[string]*group{}
	var order []string
	for _, r := range rx {
		if r.PacketID == "" {
			rep.Excluded["no packet identity"]++
			continue
		}
		g, seen := groups[r.PacketID]
		if !seen {
			g = &group{heardBy: map[string]provider.Reception{}}
			groups[r.PacketID] = g
			order = append(order, r.PacketID)
		}
		if r.Origin != "" {
			g.origin = r.Origin
		}
		g.heardBy[r.Receiver] = r
	}

	var absResiduals []float64
	for _, id := range order {
		g := groups[id]
		if g.origin == "" {
			rep.Excluded["no origin recorded"] += len(g.heardBy)
			continue
		}
		for recv, r := range g.heardBy {
			if recv == g.origin {
				continue
			}
			snr, decodes, ok := predict(g.origin, recv)
			if !ok {
				rep.Excluded["model cannot answer (unknown node or no terrain)"]++
				continue
			}
			row := ResidualRow{
				PacketID: id, Origin: g.origin, Receiver: recv,
				Observed: true, PredictedSNR: snr, PredictedDecode: decodes,
			}
			if !r.HasSNR {
				rep.Excluded["reception carried no SNR"]++
				row.Verdict = "heard, no level reported"
				rep.Rows = append(rep.Rows, row)
				continue
			}
			row.HasSNR, row.ObservedSNR = true, r.SNRdB
			row.ResidualDB = snr - r.SNRdB
			rep.Samples++
			rep.MeanBiasDB += row.ResidualDB
			absResiduals = append(absResiduals, math.Abs(row.ResidualDB))
			switch {
			case !decodes:
				row.Verdict = "MODEL PESSIMISTIC - reality heard this"
				rep.Pessimistic++
			case math.Abs(row.ResidualDB) <= 3:
				row.Verdict = fmt.Sprintf("agrees, %+.1f dB", row.ResidualDB)
			default:
				row.Verdict = fmt.Sprintf("%+.1f dB off", row.ResidualDB)
			}
			rep.Rows = append(rep.Rows, row)
		}

		// What was never heard was never recorded — except by an observer that
		// was demonstrably listening. Those silences are the model's hardest
		// test: a predicted decode nobody made is the model being optimistic.
		for _, obs := range onlineObservers {
			if obs == g.origin {
				continue
			}
			if _, heard := g.heardBy[obs]; heard {
				continue
			}
			snr, decodes, ok := predict(g.origin, obs)
			if !ok || !decodes {
				continue // consistent: neither model nor reality delivered
			}
			// A grazing prediction missing is expected; a confident one is not.
			if snr < 6 {
				continue
			}
			rep.Rows = append(rep.Rows, ResidualRow{
				PacketID: id, Origin: g.origin, Receiver: obs,
				PredictedSNR: snr, PredictedDecode: true,
				Verdict: "MODEL OPTIMISTIC - predicted a comfortable decode, nothing heard",
			})
			rep.Optimistic++
		}
	}

	if rep.Samples > 0 {
		rep.MeanBiasDB /= float64(rep.Samples)
		sort.Float64s(absResiduals)
		rep.P90AbsDB = absResiduals[int(math.Ceil(float64(len(absResiduals))*0.9))-1]
	}
	return rep
}

// Calibration is an excess path loss term with evidence behind it.
type Calibration struct {
	ExcessLossDB float64
	Samples      int
}

// Calibrate fits the one-parameter correction ADR-0015 asks for: the mean
// bias, applied as extra path loss. A positive bias (model optimistic) becomes
// a positive excess loss. It refuses small samples rather than dignifying
// them: a constant fitted to six receptions is a fudge factor with a citation.
func Calibrate(r Report) (Calibration, error) {
	const minSamples = 30
	if r.Samples < minSamples {
		return Calibration{}, fmt.Errorf(
			"replay: %d comparable observations; calibration needs at least %d "+
				"or the constant is noise wearing a lab coat", r.Samples, minSamples)
	}
	return Calibration{ExcessLossDB: r.MeanBiasDB, Samples: r.Samples}, nil
}
