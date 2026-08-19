package session

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

// PullObserved fetches recent receptions from a deployment.
func PullObserved(ctx context.Context, url string, since time.Duration) ([]state.Observed, error) {
	if url == "" {
		return nil, fmt.Errorf("no deployment URL")
	}
	cs := &provider.CoreScope{BaseURL: url}
	recs, err := cs.Receptions(ctx, time.Now().Add(-since))
	if err != nil {
		return nil, err
	}
	out := make([]state.Observed, 0, len(recs))
	for _, r := range recs {
		out = append(out, state.Observed{
			At: r.At, Receiver: r.Receiver, Origin: r.Origin,
			Transmitter: r.Transmitter,
			HopCount:    r.HopCount, HasSNR: r.HasSNR, SNRdB: r.SNRdB,
			PacketID: r.PacketID,
		})
	}
	return out, nil
}

// residualsOf compares predicted margins against observed signal-to-noise.
//
// The pair is transmitter to observer - the link the SNR was measured on -
// which makes every hop count usable: a packet three relays deep is direct
// evidence about its final relay's link to whoever heard it, and nothing at
// all about its origin's. Filtering to "hop count zero or one" and pairing
// origin with observer threw away the deep-path majority and mispaired the
// rest, which is one half of how a calibration run matched nothing.
func (s *Sim) residualsOf(obs []state.Observed, links []state.Link,
	nodes []state.Node) *state.Residuals {

	index := map[string]int{}
	for i := range nodes {
		index[nodes[i].Name] = i
	}
	margin := map[[2]int]float64{}
	for _, l := range links {
		if l.Known {
			margin[[2]int{l.A, l.B}] = l.MarginDB
			margin[[2]int{l.B, l.A}] = l.MarginDB
		}
	}
	var diffs []float64
	res := &state.Residuals{}
	for _, o := range obs {
		if !o.HasSNR {
			continue
		}
		tx := o.Transmitter
		if tx == "" {
			// An origin-only observation is a direct link only when nothing
			// relayed it.
			if o.HopCount > 0 {
				continue
			}
			tx = o.Origin
		}
		a, ok1 := index[tx]
		b, ok2 := index[o.Receiver]
		if !ok1 || !ok2 {
			res.Unmatched++
			res.OffScenario++
			continue
		}
		m, ok := margin[[2]int{a, b}]
		if !ok {
			res.Unmatched++
			res.NoLink++
			continue
		}
		// Predicted and observed have to be the same quantity before they are
		// subtracted. The observation came off a modem whose estimator
		// saturates at +15 dB, so the prediction is clamped the same way -
		// otherwise every strong link manufactures tens of decibels of
		// "residual" out of the receiver's register width, and the excess
		// loss fitted to the median inherits it.
		required := requiredSNRFor(s, a)
		predicted := dsp.ReportSNRdB(m + required)
		diffs = append(diffs, predicted-o.SNRdB)
		res.Matched++
	}
	if res.Matched == 0 {
		return res
	}
	sort.Float64s(diffs)
	res.MedianDB = diffs[len(diffs)/2]
	res.IQRdB = math.Abs(quantile(diffs, 0.75) - quantile(diffs, 0.25))
	return res
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q * float64(len(sorted)-1))
	return sorted[i]
}

// requiredSNRFor reads the demodulator floor from the one table the whole
// project shares. A hand-rolled copy of it lived here once, silently missing
// SF10 - dsp.RequiredSNRdB is measured against Semtech's figures by test, and
// a second copy is a second place for it to be wrong.
func requiredSNRFor(s *Sim, i int) float64 {
	if i >= 0 && i < len(s.nodes) {
		if v, ok := dsp.RequiredSNRdB[s.nodes[i].Radio.SpreadFactor]; ok {
			return v
		}
	}
	return -15
}
