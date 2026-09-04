// Reading a sweep back: what each arm did, whether the difference between
// them means anything, and the sentence to put on the front of it.
//
// The honesty rules live here. A sweep that has not run enough seeds to
// separate its arms from their own run-to-run spread says so rather than
// reporting the larger number, because one run is not evidence.
package experiment

import (
	"fmt"
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// armLabels is what was defined, said back. A count of arms is not enough to
// tell a cross that worked from one that quietly produced the wrong six.
func (e *experiment) armLabels() []string {
	out := make([]string, 0, len(e.Arms))
	for _, a := range e.Arms {
		out = append(out, a.Label)
	}
	return out
}

func (e *experiment) describe() map[string]any {
	return map[string]any{
		"arms": len(e.Arms), "seeds": len(e.Seeds), "senders": len(e.Senders),
		"runs": e.runsTotal(), "run_for_ms": e.RunForMs, "send_at_ms": e.SendAtMs,
		"spread_ms": e.SpreadMs, "bytes": e.Bytes,
		"scope":      session.CanonicalScope(e.Scope),
		"arm_labels": e.armLabels(),
	}
}

// notAResultYet is the honesty check: what would make these numbers not mean
// what they appear to.
func (e *experiment) notAResultYet() string {
	switch {
	case len(e.results) == 0:
		return "nothing has run yet"
	// Ahead of every other objection, because the others are about how much a
	// difference is worth and this one is about whether there is a difference
	// to weigh. Seeds bound the noise from a draw; nothing here bounds the
	// noise from a clock the run does not own, so two cells that differ may
	// have differed for no reason the matrix records.
	case e.notReproducible != "":
		return "the arms are not comparable: " + e.notReproducible
	case len(e.Seeds) < 2:
		return "one seed: this is one draw, not a spread"
	case len(e.Arms) < 2:
		return "one arm: there is nothing to compare it against"
	}
	for _, r := range e.results {
		if r.Err != "" {
			return "at least one run failed: " + r.Err
		}
	}
	// Identical runs across seeds are not a spread. That is not a fault here:
	// with one originator and rx_delay_base at its shipped zero, every
	// repeater relays exactly once and the count is a property of the
	// topology - the project's own study measured the same 93 transmissions
	// on each of eight seeds. What follows from it is that the seed cannot
	// bound the noise, so a difference between arms has nothing to be called
	// larger than, and saying so is the whole job of this function.
	for _, sum := range e.summarise() {
		if n, _ := sum["runs"].(int); n < 2 {
			continue
		}
		if sp, _ := sum["rx_spread"].(float64); sp == 0 {
			// What was observed, and not why.
			//
			// This used to assert a cause - "which this firmware does by design
			// with one originator" - and that cause was wrong the first time it
			// was read in anger: the run had two originators, and three of the
			// four arms varied normally. A message that explains is worth more
			// than one that reports, but only while the explanation holds, and
			// a confident wrong reason sends the reader looking in the wrong
			// place. What is certain is the consequence.
			return fmt.Sprintf(
				"every seed of %v returned the same numbers, so the seed gives no "+
					"noise floor here and a difference between arms has nothing to "+
					"be called larger than. Add senders or seeds, or quote the "+
					"deltas as unbounded.", sum["arm"])
		}
	}

	// And the comparison that decides whether any of it is a result: the seeds
	// disagreeing among themselves by more than the arms disagree with each
	// other means the difference on the table is inside the noise, whatever
	// its sign.
	if spread, between, ok := e.spreadAgainstBetween(); ok && spread >= between {
		return fmt.Sprintf(
			"the seeds disagree by more than the arms do on receptions "+
				"(±%.1f%% within an arm, %.1f%% between them) - add repeats",
			spread*100, between*100)
	}
	return ""
}

// perSecondFor sums one arm's histograms across its seeds.
//
// Summed rather than averaged, and every arm drawn on one scale afterwards:
// what is being compared is the shape, and a per-panel scale would flatten the
// arm that delivered least into looking identical to the one that delivered
// most.
func (e *experiment) perSecondFor(arm string) []int {
	var out []int
	for _, r := range e.results {
		if r.Arm != arm {
			continue
		}
		for i, v := range r.PerSecond {
			if i >= len(out) {
				out = append(out, make([]int, i-len(out)+1)...)
			}
			out[i] += v
		}
	}
	return out
}

// runRows is every cell of the matrix and where it has got to.
//
// Results are positional - arm outer, seed inner, in the order they were
// defined - so the cell after the last result is the one running now.
func (e *experiment) runRows() []state.RunRow {
	var out []state.RunRow
	done := len(e.results)
	i := 0
	for _, arm := range e.Arms {
		for _, seed := range e.Seeds {
			row := state.RunRow{Arm: arm.Label, Seed: seed, State: "queued"}
			switch {
			case i < done:
				r := e.results[i]
				row.State = "done"
				row.Result = fmt.Sprintf("%d tx  %d rx  %d delivered",
					r.TX, r.RX, r.Delivered)
				if r.Err != "" {
					row.State, row.Result = "failed", r.Err
				}
			case i == done && e.running:
				row.State = "running"
			}
			out = append(out, row)
			i++
		}
	}
	return out
}

// verdict is what the sweep found, in one line, once it has finished.
//
// It exists because a table of numbers is not an answer, and because the
// answer is usually "nothing changed" - which somebody reading six columns of
// small percentages will not conclude on their own, and will instead read as a
// small change.
func (e *experiment) verdict() string {
	sums := e.summarise()
	if len(sums) < 2 {
		return ""
	}
	base := sums[0]
	spread, _, _ := e.spreadAgainstBetween()

	// Every metric here is one where less is better, except delivery.
	metrics := []struct {
		key, said string
	}{
		{"rx", "receptions"},
		{"tx", "transmissions"},
		{"collisions", "collisions"},
		{"airtime_ms", "airtime"},
		{"delivered", "unique deliveries"},
		{"redundant", "redundant relays"},
	}
	biggest, what, which := 0.0, "", ""
	for _, s := range sums[1:] {
		// An arm nothing measured has no numbers to compare, and its absent
		// ones read as zero: without this the verdict announced the arm that
		// failed as the biggest change the sweep found.
		if n, _ := s["runs"].(int); n == 0 {
			continue
		}
		for _, m := range metrics {
			ref, _ := base[m.key].(float64)
			got, _ := s[m.key].(float64)
			if ref == 0 {
				continue
			}
			d := (got - ref) / ref
			if math.Abs(d) > math.Abs(biggest) {
				arm, _ := s["arm"].(string)
				biggest, what, which = d, m.said, arm
			}
		}
	}
	switch {
	case what == "" || math.Abs(biggest) < 0.005:
		return "No difference. Every arm produced the same numbers."
	case math.Abs(biggest) <= spread:
		return fmt.Sprintf(
			"No difference worth reporting: the largest change, %s on %s by %+.1f%%, "+
				"is inside the ±%.1f%% the seeds vary by on their own.",
			what, which, biggest*100, spread*100)
	default:
		return fmt.Sprintf("%s changed %s by %+.1f%%, against a seed spread of ±%.1f%%.",
			which, what, biggest*100, spread*100)
	}
}

// spreadAgainstBetween is the worst arm's seed spread, and how far apart the
// arms' own means are. Both as fractions, so they can be compared.
func (e *experiment) spreadAgainstBetween() (spread, between float64, ok bool) {
	sums := e.summarise()
	if len(sums) < 2 {
		return 0, 0, false
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	measured := 0
	for _, s := range sums {
		// Only arms that measured something. An arm that did not run has no
		// receptions rather than none of them, and counting its absent zero as
		// the low end made the arms look further apart than they are.
		if n, _ := s["runs"].(int); n == 0 {
			continue
		}
		measured++
		rx, _ := s["rx"].(float64)
		lo, hi = math.Min(lo, rx), math.Max(hi, rx)
		sp, _ := s["rx_spread"].(float64)
		spread = math.Max(spread, sp)
	}
	if hi <= 0 || measured < 2 {
		return 0, 0, false
	}
	// Normalised by the largest mean rather than by the smallest or the
	// average: it is the conservative choice, and makes the between-arm figure
	// harder to clear rather than easier.
	return spread, (hi - lo) / hi, true
}

func (e *experiment) summarise() []map[string]any {
	by := map[string][]Result{}
	for _, r := range e.results {
		by[r.Arm] = append(by[r.Arm], r)
	}
	names := make([]string, 0, len(by))
	for k := range by {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, summariseArm(name, by[name]))
	}
	return out
}

// summariseArm averages one arm over the seeds that produced a measurement.
//
// A cell that failed carries zeros in every counter, because it never got as
// far as counting anything. Averaged in with the rest those zeros are not a
// missing number but a wrong one: one failed seed of three pulled an arm's
// receptions down by a third, and the matrix drew that as a regression the
// firmware had caused. An arm where every seed failed came out at zero
// throughout, which reads as the worst result the sweep has ever produced
// rather than as an arm that did not run.
func summariseArm(name string, rs []Result) map[string]any {
	var tx, rx, del, red, coll, air float64
	// at2 is the share of this arm's decodes that 2 dB of receiver would
	// have cost. Reported per arm because it is the one figure here that a
	// flood's redundancy cannot hide: it is counted on the deliveries
	// themselves rather than summed out of them.
	var at2, at2n float64
	ran := make([]Result, 0, len(rs))
	failed := 0
	for _, r := range rs {
		if r.Err != "" {
			failed++
			continue
		}
		ran = append(ran, r)
		tx += float64(r.TX)
		rx += float64(r.RX)
		del += float64(r.Delivered)
		red += float64(r.Redundant)
		coll += float64(r.Collided)
		air += r.AirtimeMs
		if len(r.AtRisk) > 1 {
			at2 += r.AtRisk[1]
			at2n++
		}
	}
	row := map[string]any{"arm": name, "runs": len(ran), "failed": failed}
	// Absent rather than zero when nothing measured it, here and for
	// at_risk_2db below: "no cell measured this" and "this cell measured
	// nothing" are different answers, and drawing the second for the first is
	// the quiet lie the whole of this file exists to stop.
	if len(ran) == 0 {
		return row
	}
	n := float64(len(ran))
	row["tx"], row["rx"], row["delivered"] = tx/n, rx/n, del/n
	row["redundant"], row["collisions"] = red/n, coll/n
	row["airtime_ms"], row["rx_spread"] = air/n, spreadOf(ran)
	if at2n > 0 {
		row["at_risk_2db"] = at2 / at2n
	}
	return row
}

// spreadOf is the range of receptions across seeds, which is what says whether
// a difference between arms is bigger than the noise within one.
// spreadOf is how much the seeds of one arm disagree, as a fraction of that
// arm's mean: half the range, so it reads as the ± either side of the middle.
//
// A fraction rather than a count of receptions, because it exists to be
// compared against the difference between arms, and that comparison is
// meaningless in absolute terms - twelve receptions is noise on a national
// flood and the whole result on a valley.
func spreadOf(rs []Result) float64 {
	if len(rs) < 2 {
		return 0
	}
	lo, hi, sum := float64(rs[0].RX), float64(rs[0].RX), 0.0
	for _, r := range rs {
		v := float64(r.RX)
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
		sum += v
	}
	mean := sum / float64(len(rs))
	if mean == 0 {
		return 0
	}
	return (hi - lo) / 2 / mean
}
