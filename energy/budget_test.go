package energy_test

import (
	"math"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/energy"
)

// A Scottish hilltop: the case this package exists to answer.
func perthshire() energy.Site {
	return energy.Site{
		Name:   "Ben Vrackie repeater",
		LatDeg: 56.72, LonDeg: -3.72,
		Battery: energy.Battery{
			Chemistry: energy.LiIon, CapacityMAh: 6800, Cells: 1, CutoffV: 3.1,
		},
		Panel: energy.Panel{
			PeakW: 10, TiltDeg: 55, AzimuthDeg: 180,
			SoilingFactor: 0.8, ChargeEfficiency: 0.95,
		},
		Load:       energy.SX1262Load(),
		Duty:       energy.Duty{TxFraction: 0.001, RxFraction: 0.999},
		TxPowerDBm: 22,
		CloudByMonth: [12]float64{0.75, 0.72, 0.68, 0.62, 0.58, 0.58,
			0.60, 0.62, 0.65, 0.72, 0.78, 0.80},
		TempCByMonth: [12]float64{1, 1, 3, 5, 8, 11, 13, 13, 10, 7, 3, 1},
	}
}

// Midwinter at 57° north is the whole problem. If a site survives the year it
// survives late December, so the annual minimum must land there — and if this
// test ever says otherwise, the solar geometry is wrong, not the site.
func TestWorstDayIsMidwinter(t *testing.T) {
	r, err := energy.SimulateYear(perthshire())
	if err != nil {
		t.Fatal(err)
	}
	if r.WorstDay > 60 && r.WorstDay < 300 {
		t.Errorf("worst day of the year is day %d — not midwinter, so the sun model is wrong", r.WorstDay)
	}
	t.Logf("worst SoC %.2f on day %d, %d dead days, autonomy %.1f days",
		r.WorstSoC, r.WorstDay, r.DeadDays, r.AutonomyDays)
}

// The sun must be up in June and down at midnight, at a plausible elevation.
// Getting the equation of time or the hour angle wrong produces a site that is
// sunny at 3 a.m., and every energy figure downstream is then fiction.
func TestSolarGeometry(t *testing.T) {
	const lat, lon = 56.72, -3.72
	cases := []struct {
		name          string
		day           int
		hour          float64
		wantElevation float64
		tol           float64
	}{
		// Summer solstice local noon: 90 - 56.72 + 23.44 = 56.7 degrees.
		{"midsummer noon", 172, 12.25, 56.7, 3},
		// Winter solstice noon: 90 - 56.72 - 23.44 = 9.8 degrees. Low enough
		// that a flat panel is nearly useless, which is the design point.
		{"midwinter noon", 355, 12.25, 9.8, 3},
		{"midsummer midnight", 172, 0.0, -10, 6},
	}
	for _, c := range cases {
		got := energy.SunAt(lat, lon, c.day, c.hour).ElevationDeg
		if math.Abs(got-c.wantElevation) > c.tol {
			t.Errorf("%s: sun at %.1f degrees, want about %.1f", c.name, got, c.wantElevation)
		}
	}
}

// Overcast is not darkness. A winter sky still delivers roughly a tenth of the
// clear-sky resource, all diffuse, and a solar node lives on exactly that.
func TestOvercastStillHarvests(t *testing.T) {
	p := perthshire().Panel
	sun := energy.SunAt(56.72, -3.72, 355, 12.25)

	clear := p.HarvestW(sun, 0)
	overcast := p.HarvestW(sun, 1)
	if overcast <= 0 {
		t.Fatal("an overcast winter day harvests nothing at all — diffuse is missing")
	}
	if overcast >= clear {
		t.Errorf("overcast %.2f W is not below clear-sky %.2f W", overcast, clear)
	}
	if r := overcast / clear; r > 0.4 {
		t.Errorf("overcast is %.0f%% of clear sky, which is far too generous", r*100)
	}
}

// Night must harvest nothing. Obvious, and exactly the kind of thing a sign
// error in the incidence cosine gets wrong while every other test still passes.
func TestNightHarvestsNothing(t *testing.T) {
	p := perthshire().Panel
	if w := p.HarvestW(energy.SunAt(56.72, -3.72, 355, 0.5), 0.5); w != 0 {
		t.Errorf("harvested %.3f W at midnight in December", w)
	}
}

// Receive, not transmit, sets battery life in a mesh. A node listening
// continuously and transmitting 0.1% of the time spends nearly all its energy
// on the receiver — which is why "reduce transmit power to save battery" is
// usually the wrong advice.
func TestReceiveDominatesTheBudget(t *testing.T) {
	l := energy.SX1262Load()
	duty := energy.Duty{TxFraction: 0.001, RxFraction: 0.999}

	at22 := l.AverageMA(duty, 22)
	at14 := l.AverageMA(duty, 14)
	if change := (at22 - at14) / at22; change > 0.05 {
		t.Errorf("dropping 22 dBm to 14 dBm changed average current by %.1f%%; "+
			"receive should dominate", change*100)
	}

	// And sleeping is transformative in a way transmit power is not.
	sleepy := l.AverageMA(energy.Duty{TxFraction: 0.001, RxFraction: 0.05}, 22)
	if sleepy > at22/10 {
		t.Errorf("a 5%% duty node draws %.2f mA against %.2f mA always-on; expected far less",
			sleepy, at22)
	}
}

func TestColdCostsCapacity(t *testing.T) {
	b := energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.1}
	warm := b.CapacityAt(20, 5)
	cold := b.CapacityAt(-10, 5)
	if cold >= warm {
		t.Fatalf("cold capacity %.0f mAh is not below warm %.0f mAh", cold, warm)
	}
	if r := cold / warm; r < 0.4 || r > 0.85 {
		t.Errorf("at -10 C the pack keeps %.0f%% of capacity, which is not a real curve", r*100)
	}
}

// The plateau is the point of a Li-ion curve. A pack at 80% and one at 30% read
// almost the same voltage, which is why voltage-based fuel gauges say "fine"
// until they very suddenly do not.
func TestLiIonHasAPlateau(t *testing.T) {
	b := energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0}
	if d := b.VoltageAt(0.8) - b.VoltageAt(0.3); d > 0.15 {
		t.Errorf("voltage fell %.3f V across the plateau; that is not a Li-ion curve", d)
	}
	if b.VoltageAt(0.05) >= b.VoltageAt(0.3) {
		t.Error("voltage did not collapse at the bottom")
	}
	if !b.Dead(0.0) || b.Dead(1.0) {
		t.Error("cutoff detection is the wrong way round")
	}
}

// Transmit current is strongly non-linear in output power, so a single figure
// would be wrong by a factor of three across the range.
func TestTxCurrentIsNonLinear(t *testing.T) {
	lo := energy.SX1262TxCurrentMA(14)
	hi := energy.SX1262TxCurrentMA(22)
	if hi <= lo*2 {
		t.Errorf("22 dBm draws %.0f mA against %.0f mA at 14 dBm — too flat", hi, lo)
	}
	// Clamped outside the datasheet range rather than extrapolated into
	// nonsense.
	if energy.SX1262TxCurrentMA(100) != hi || energy.SX1262TxCurrentMA(-100) != energy.SX1262TxCurrentMA(-9) {
		t.Error("current is extrapolated beyond the datasheet points")
	}
}

// A repeater listens continuously; whatever it is not transmitting, it is
// receiving. Letting the leftover fall to sleep would overstate battery life by
// an order of magnitude.
func TestAlwaysOnDoesNotSleep(t *testing.T) {
	d := energy.DutyFromAirtime(100, 0, 10_000, true)
	if d.RxFraction < 0.98 {
		t.Errorf("an always-on node receives %.3f of the time", d.RxFraction)
	}
	sleepy := energy.DutyFromAirtime(100, 200, 10_000, false)
	if sleepy.RxFraction > 0.03 {
		t.Errorf("a duty-cycled node receives %.3f of the time", sleepy.RxFraction)
	}
}

func TestSimulateYearRejectsAnImpossibleSite(t *testing.T) {
	s := perthshire()
	s.Battery.CapacityMAh = 0
	if _, err := energy.SimulateYear(s); err == nil {
		t.Fatal("a site with no battery was accepted")
	}
}

// The model has to be able to say no, and to say why. A panel bolted flat to
// the top of a pole is the commonest mistake on a real installation, and at 57
// degrees north in December it is close to useless: the sun never gets more
// than about 10 degrees up, so a horizontal panel sees almost none of it.
//
// The comfortable site above passes trivially, which tests nothing. This one is
// deliberately marginal.
func TestFlatPanelFailsWhereTiltedOneSurvives(t *testing.T) {
	marginal := perthshire()
	marginal.Battery.CapacityMAh = 2200
	marginal.Panel.PeakW = 1.5

	tilted := marginal
	tilted.Panel.TiltDeg = 55

	flat := marginal
	flat.Panel.TiltDeg = 0

	rt, err := energy.SimulateYear(tilted)
	if err != nil {
		t.Fatal(err)
	}
	rf, err := energy.SimulateYear(flat)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("tilted 55 deg: worst SoC %.2f, %d dead days", rt.WorstSoC, rt.DeadDays)
	t.Logf("flat:          worst SoC %.2f, %d dead days", rf.WorstSoC, rf.DeadDays)

	if rf.WorstSoC >= rt.WorstSoC {
		t.Errorf("a flat panel did no worse than a tilted one at 57 degrees north — "+
			"tilted %.3f, flat %.3f", rt.WorstSoC, rf.WorstSoC)
	}
	if rf.DeadDays == 0 && rf.WorstSoC > 0.5 {
		t.Errorf("a 1.5 W flat panel carried this site through a Scottish winter "+
			"with %.0f%% to spare; the harvest model is too generous", rf.WorstSoC*100)
	}
}

// A hill to the south-east costs a UK site its winter mornings, and the
// budget must show it: same site, horizon added, strictly worse worst-case.
func TestTerrainHorizonCostsHarvest(t *testing.T) {
	site := perthshire()
	clear, err := energy.SimulateYear(site)
	if err != nil {
		t.Fatal(err)
	}
	site.Horizon = func(az float64) float64 {
		if az > 90 && az < 200 {
			return 25 // a serious hill across the southern sky
		}
		return 0
	}
	blocked, err := energy.SimulateYear(site)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.WorstSoC >= clear.WorstSoC {
		t.Fatalf("hill did not cost anything: clear %.2f, blocked %.2f",
			clear.WorstSoC, blocked.WorstSoC)
	}
}
