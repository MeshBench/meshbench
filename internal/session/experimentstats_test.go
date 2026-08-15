package session

import (
	"math"
	"testing"
)

func near(t *testing.T, got, want, tol float64, what string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %v want %v (±%v)", what, got, want, tol)
	}
}

// Interval arithmetic against a hand-worked example: 10, 12, 14, 16 has mean
// 13, sample standard deviation √(20/3) ≈ 2.582, and at df=3 the 95% t
// critical value is 3.182 - a half-width of about 4.108.
func TestIntervalArithmeticAgainstAHandWorkedExample(t *testing.T) {
	mean, lo, hi, has := armInterval([]float64{10, 12, 14, 16})
	if !has {
		t.Fatal("four samples should produce an interval")
	}
	near(t, mean, 13, 1e-9, "mean")
	near(t, lo, 13-4.108, 0.01, "lo")
	near(t, hi, 13+4.108, 0.01, "hi")
}

// n=2 still produces an interval - a wide one, because df=1 carries the
// largest t critical value there is - but not a zero-width one.
func TestNEqualsTwoProducesAWideInterval(t *testing.T) {
	mean, lo, hi, has := armInterval([]float64{10, 20})
	if !has {
		t.Fatal("two samples should produce an interval")
	}
	near(t, mean, 15, 1e-9, "mean")
	// sd = √50 ≈ 7.0711; half = 12.706 * 7.0711 / √2 ≈ 63.53
	near(t, hi-mean, 63.53, 0.1, "half-width")
	near(t, mean-lo, 63.53, 0.1, "half-width")
}

// n=1 reports no interval rather than a zero-width one: a single run has
// nothing to build a spread from, and a zero-width interval reads as
// certainty it cannot support.
func TestNEqualsOneReportsNoInterval(t *testing.T) {
	mean, lo, hi, has := armInterval([]float64{42})
	if has {
		t.Fatal("one sample reported an interval")
	}
	if mean != 42 || lo != 42 || hi != 42 {
		t.Fatalf("a single sample should still report its own value: mean=%v lo=%v hi=%v", mean, lo, hi)
	}
	if _, _, _, has := armInterval(nil); has {
		t.Fatal("zero samples reported an interval")
	}
}

// Non-overlapping intervals, and only non-overlapping intervals, are marked
// significant. Everything else - including overlap, and either side lacking
// an interval - is "not yet".
func TestSignificanceIsNonOverlapOnly(t *testing.T) {
	results := []ExpResult{
		{Arm: "baseline", RX: 3559}, {Arm: "baseline", RX: 3565},
		{Arm: "clearly-different", RX: 3319}, {Arm: "clearly-different", RX: 3345},
		{Arm: "overlapping", RX: 3400}, {Arm: "overlapping", RX: 3700},
		{Arm: "one-seed", RX: 3400},
	}
	stats := armStatsFor(results, "baseline")
	byArm := map[string]ArmStats{}
	for _, s := range stats {
		byArm[s.Arm] = s
	}
	if v := byArm["baseline"].Verdict; v != "" {
		t.Errorf("the baseline arm itself should carry no verdict, got %q", v)
	}
	if v := byArm["clearly-different"].Verdict; v != "significant" {
		t.Errorf("non-overlapping intervals: got %q want significant", v)
	}
	if v := byArm["overlapping"].Verdict; v != "not yet" {
		t.Errorf("overlapping intervals: got %q want \"not yet\"", v)
	}
	if v := byArm["one-seed"].Verdict; v != "not yet" {
		t.Errorf("one seed against the baseline: got %q want \"not yet\" (no interval to compare)", v)
	}
	if !byArm["clearly-different"].HasDelta {
		t.Error("a non-baseline arm should carry a delta")
	}
	if byArm["baseline"].HasDelta {
		t.Error("the baseline arm should not carry a delta against itself")
	}
}

// Errored cells do not enter the statistics: a cell that measured nothing
// must not pull an arm's mean toward zero.
func TestErroredCellsAreExcludedFromStats(t *testing.T) {
	results := []ExpResult{
		{Arm: "a", RX: 100}, {Arm: "a", RX: 100},
		{Arm: "a", RX: 0, Err: "no firmware attached"},
	}
	stats := armStatsFor(results, "a")
	if len(stats) != 1 {
		t.Fatalf("expected one arm, got %d", len(stats))
	}
	if stats[0].Runs != 2 {
		t.Fatalf("errored run counted: Runs=%d want 2", stats[0].Runs)
	}
	if stats[0].RXMean != 100 {
		t.Fatalf("errored run pulled the mean: got %v want 100", stats[0].RXMean)
	}
}

// The seeds-needed estimate is checked against what a hand-worked extension
// actually achieves: the returned n's own half-width must be at or below the
// target, and one fewer must not be.
func TestSeedsNeededMatchesAHandWorkedExtension(t *testing.T) {
	sd := 7.0711
	target := 5.0
	n := seedsNeeded(sd, target)
	if n < 2 {
		t.Fatalf("seedsNeeded returned %d, too small to have an interval at all", n)
	}
	half := tCritical(n-1) * sd / math.Sqrt(float64(n))
	if half > target+1e-9 {
		t.Fatalf("n=%d does not reach the target: half=%v target=%v", n, half, target)
	}
	if n > 2 {
		prevHalf := tCritical(n-2) * sd / math.Sqrt(float64(n-1))
		if prevHalf <= target {
			t.Fatalf("n=%d was not the smallest that reaches the target (n-1 already did)", n)
		}
	}
}

func TestSeedsNeededIsZeroWithoutSpreadOrTarget(t *testing.T) {
	if n := seedsNeeded(0, 5); n != 0 {
		t.Errorf("zero standard deviation: got %d want 0", n)
	}
	if n := seedsNeeded(5, 0); n != 0 {
		t.Errorf("zero target: got %d want 0", n)
	}
}
