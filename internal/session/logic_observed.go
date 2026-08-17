package session

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provider"
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
			HopCount: r.HopCount, HasSNR: r.HasSNR, SNRdB: r.SNRdB,
			PacketID: r.PacketID,
		})
	}
	return out, nil
}

// residualsOf compares predicted margins against observed signal-to-noise.
//
// Only direct receptions - hop count zero or one - because a packet that
// arrived over three relays says nothing about the link between its origin and
// whoever finally heard it, and counting it would compare a path to a hop.
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
	matched, unmatched := 0, 0
	for _, o := range obs {
		if !o.HasSNR || o.HopCount > 1 {
			continue
		}
		a, ok1 := index[o.Origin]
		b, ok2 := index[o.Receiver]
		if !ok1 || !ok2 {
			unmatched++
			continue
		}
		m, ok := margin[[2]int{a, b}]
		if !ok {
			unmatched++
			continue
		}
		// The observed margin is how far the signal was above what the
		// demodulator needed, which is the same quantity the model predicts.
		observed := o.SNRdB - requiredSNRFor(s, a)
		diffs = append(diffs, m-observed)
		matched++
	}
	if matched == 0 {
		return &state.Residuals{Matched: 0, Unmatched: unmatched}
	}
	sort.Float64s(diffs)
	return &state.Residuals{
		Matched: matched, Unmatched: unmatched,
		MedianDB: diffs[len(diffs)/2],
		IQRdB:    math.Abs(quantile(diffs, 0.75) - quantile(diffs, 0.25)),
	}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(q * float64(len(sorted)-1))
	return sorted[i]
}

func requiredSNRFor(s *Sim, i int) float64 {
	if i < 0 || i >= len(s.nodes) {
		return -15
	}
	switch s.nodes[i].Radio.SpreadFactor {
	case 7:
		return -7.5
	case 8:
		return -10
	case 9:
		return -12.5
	case 11:
		return -17.5
	case 12:
		return -20
	}
	return -15
}
