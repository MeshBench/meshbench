// What each node did over a run, totalled.
package engine

import "sort"

// Scoreboard is the per-node summary, ordered worst-value first.
//
// Unique deliveries against redundant relays is the number HopReach found
// mattered most, and it is the one a duty-cycle figure hides: a repeater can be
// busy, legal, and reaching nobody who had not already heard the packet.
type Score struct {
	Name           string
	Sent           int
	Heard          int
	AirtimeMs      float64
	DutyCyclePct   float64
	UniqueDelivery int
	RedundantRelay int
}

func (e *Engine) Scoreboard() []Score {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]Score, 0, len(e.nodes))
	for _, n := range e.nodes {
		s := Score{
			Name: n.Spec.Name, Sent: n.Sent, Heard: n.Heard,
			AirtimeMs:      n.AirtimeMs,
			UniqueDelivery: n.UniqueDelivery, RedundantRelay: n.RedundantRelay,
		}
		if e.nowMs > 0 {
			s.DutyCyclePct = 100 * n.AirtimeMs / float64(e.nowMs)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AirtimeMs > out[j].AirtimeMs })
	return out
}
