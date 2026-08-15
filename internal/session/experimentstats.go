// Confidence, not just spread.
//
// The bench already computed a spread - the range of receptions across
// seeds - and left the reader to decide whether a difference between arms
// was bigger than it. A study once ran four arms at two seeds each; one arm
// spread 5% across its seeds, wider than three of the differences being
// discussed, and the interface said so in a single warning line that had to
// be interpreted rather than read.
//
// This computes an interval instead, marks what it does and does not
// support against the baseline arm, and says how many more seeds would
// narrow it - because "is this difference real?" is a question a spread
// cannot answer and an interval can.
package session

import (
	"fmt"
	"math"
	"sort"
)

// ArmStats is one arm's headline-metric (receptions) interval, and what it
// says about a claim against the baseline arm.
type ArmStats struct {
	Arm  string
	Runs int
	// RXMean, RXLo, RXHi are the 95% Student's t interval on receptions.
	// HasInterval is false at Runs<2: a single run has no spread to build an
	// interval from, and a zero-width one would read as certainty a single
	// draw cannot support.
	RXMean, RXLo, RXHi float64
	HasInterval        bool
	// DeltaPct is against the baseline arm; HasDelta is false for the
	// baseline arm itself, which is not being compared against anything.
	DeltaPct float64
	HasDelta bool
	// Verdict is "significant", "not yet", or "" for the baseline arm.
	// Significance is non-overlapping intervals only - anything else,
	// including either side lacking an interval at all, is "not yet".
	Verdict string
}

// tCrit95 is the two-tailed 95% Student's t critical value, indexed by
// degrees of freedom. A table rather than a computed quantile: this project
// carries no statistics dependency for one number, and the acceptance
// criterion is that the arithmetic matches a hand-worked example, which a
// published table can promise and an approximation cannot.
var tCrit95 = []float64{
	0, // df=0 is never looked up
	12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
	2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
	2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042,
}

// tCritical is the table above for small samples, and the normal
// approximation once more seeds have made the difference immaterial.
func tCritical(df int) float64 {
	switch {
	case df <= 0:
		return 0
	case df < len(tCrit95):
		return tCrit95[df]
	default:
		return 1.960
	}
}

// meanStdev is the sample mean and the unbiased sample standard deviation.
// Stdev is zero, not NaN, below two samples - there is nothing to divide by
// zero over, only nothing yet to say about spread.
func meanStdev(xs []float64) (mean, sd float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= n
	if len(xs) < 2 {
		return mean, 0
	}
	var ss float64
	for _, x := range xs {
		d := x - mean
		ss += d * d
	}
	return mean, math.Sqrt(ss / (n - 1))
}

// armInterval is the 95% Student's t confidence interval for one arm's
// values. has is false at n<2.
func armInterval(xs []float64) (mean, lo, hi float64, has bool) {
	mean, sd := meanStdev(xs)
	if len(xs) < 2 {
		return mean, mean, mean, false
	}
	half := tCritical(len(xs)-1) * sd / math.Sqrt(float64(len(xs)))
	return mean, mean - half, mean + half, true
}

// overlap reports whether two closed intervals share a point.
func overlap(lo1, hi1, lo2, hi2 float64) bool {
	return lo1 <= hi2 && lo2 <= hi1
}

// armStatsFor computes the rx interval and baseline comparison for every arm
// present in results. baseline is compared against nothing; every other arm
// is compared against it.
//
// Reported on receptions only, deliberately: receptions, deliveries and
// collisions move together, and marking several metrics "significant" from
// the same two seeds invites reading them as independent confirmation when
// they are one measurement three ways.
func armStatsFor(results []ExpResult, baseline string) []ArmStats {
	by := map[string][]float64{}
	var order []string
	for _, r := range results {
		if r.Err != "" {
			continue
		}
		if _, seen := by[r.Arm]; !seen {
			order = append(order, r.Arm)
		}
		by[r.Arm] = append(by[r.Arm], float64(r.RX))
	}
	sort.Strings(order)

	baseMean, baseLo, baseHi, baseHas := armInterval(by[baseline])
	_, baseKnown := by[baseline]

	out := make([]ArmStats, 0, len(order))
	for _, name := range order {
		mean, lo, hi, has := armInterval(by[name])
		st := ArmStats{Arm: name, Runs: len(by[name]),
			RXMean: mean, RXLo: lo, RXHi: hi, HasInterval: has}
		if name != baseline && baseKnown {
			st.HasDelta = true
			if baseMean != 0 {
				st.DeltaPct = (mean - baseMean) / baseMean * 100
			}
			switch {
			case !has || !baseHas:
				st.Verdict = "not yet"
			case !overlap(lo, hi, baseLo, baseHi):
				st.Verdict = "significant"
			default:
				st.Verdict = "not yet"
			}
		}
		out = append(out, st)
	}
	return out
}

// seedsNeeded estimates the total sample size that would narrow a Student's
// t interval to at most targetHalf, given the standard deviation already
// observed. Iterated rather than solved from the normal-approximation
// formula: t widens as degrees of freedom fall, and solving the z-only
// formula understates what small n costs, which is exactly the case this
// estimate exists for.
func seedsNeeded(sd, targetHalf float64) int {
	if sd <= 0 || targetHalf <= 0 {
		return 0
	}
	for n := 2; n <= 200; n++ {
		half := tCritical(n-1) * sd / math.Sqrt(float64(n))
		if half <= targetHalf {
			return n
		}
	}
	return 200
}

// widestArm is the arm whose interval is loosest relative to its own mean -
// the one a claim about it cannot yet support, and the one "narrow it"
// spends its seeds on.
func widestArm(stats []ArmStats) (name string, relHalf float64, ok bool) {
	best := -1.0
	for _, st := range stats {
		if !st.HasInterval || st.RXMean == 0 {
			continue
		}
		half := (st.RXHi - st.RXMean) / st.RXMean
		if half > best {
			best, name, ok = half, st.Arm, true
		}
	}
	return name, best, ok
}

// avgWallMs is the mean wall-clock cost of one cell among results that
// finished, for turning "N more seeds" into a time estimate a human can
// weigh against waiting for the whole thing.
func avgWallMs(results []ExpResult) float64 {
	var sum, n float64
	for _, r := range results {
		if r.Err != "" || r.WallMs <= 0 {
			continue
		}
		sum += r.WallMs
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

// narrowCaption is the sentence about the loosest arm: what it spreads, how
// many more seeds would halve that, and about how long that takes. The
// second return is the seed count to hand straight to experiment.extend.
//
// Targets half the current relative half-width - "let's tighten the worst
// one" rather than a fixed percentage nothing about the data justified.
func narrowCaption(results []ExpResult, stats []ArmStats) (string, int) {
	name, relHalf, ok := widestArm(stats)
	if !ok {
		return "", 0
	}
	var xs []float64
	for _, r := range results {
		if r.Err == "" && r.Arm == name {
			xs = append(xs, float64(r.RX))
		}
	}
	mean, sd := meanStdev(xs)
	if sd == 0 || mean == 0 {
		return "", 0
	}
	targetHalf := (relHalf * mean) / 2
	total := seedsNeeded(sd, targetHalf)
	extra := total - len(xs)
	if extra <= 0 {
		return "", 0
	}
	minutes := avgWallMs(results) * float64(extra) * float64(armCount(results)) / 60000
	targetPct, nowPct := targetHalf/mean*100, relHalf*100
	plural := "s"
	if extra == 1 {
		plural = ""
	}
	caption := fmt.Sprintf("%s spreads ±%.1f%% across %d seeds. run %d more seed%s",
		name, nowPct, len(xs), extra, plural)
	if minutes > 0 {
		caption += fmt.Sprintf(" ≈ %.0f min", minutes)
	}
	caption += fmt.Sprintf(" · would bound it to ±%.1f%%", targetPct)
	return caption, extra
}

// armCount is how many distinct arms results carries, so "N more seeds"
// against a K-arm matrix can say what it actually costs: K new cells per
// seed, not one.
func armCount(results []ExpResult) int {
	seen := map[string]bool{}
	for _, r := range results {
		seen[r.Arm] = true
	}
	if len(seen) == 0 {
		return 1
	}
	return len(seen)
}
