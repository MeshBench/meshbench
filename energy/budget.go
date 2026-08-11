package energy

import (
	"fmt"
	"math"
)

// Site is everything about a node that decides whether it survives the winter.
type Site struct {
	Name       string
	LatDeg     float64
	LonDeg     float64
	Battery    Battery
	Panel      Panel // zero PeakW means no solar
	Load       Load
	Duty       Duty
	TxPowerDBm float64

	// Weather, per month, as mean cloud cover in [0,1] and mean temperature.
	// Twelve entries. Real climate data belongs here eventually (MSIM-20's
	// sibling problem); until then a scenario supplies its own.
	CloudByMonth [12]float64
	TempCByMonth [12]float64

	// Horizon, if set, is the terrain's elevation angle at an azimuth, in
	// degrees. A panel behind a hill loses hours of morning sun, and that is
	// exactly the site error this exists to catch. When the sun is above 0
	// but below the horizon, only diffuse skylight reaches the panel.
	Horizon func(azimuthDeg float64) float64
}

// Day is one day of the simulated year.
type Day struct {
	DayOfYear   int
	HarvestWh   float64
	ConsumedWh  float64
	EndSoC      float64
	MinSoC      float64
	Dead        bool
	SunHoursEq  float64 // full-sun-equivalent hours, the number panel sizing uses
	MeanCurrmMA float64
}

// YearResult is a whole year at this site.
type YearResult struct {
	Days []Day

	// WorstSoC and WorstDay are the low point, which for a UK solar node is
	// almost always late December — and is the only number that decides whether
	// the site works.
	WorstSoC float64
	WorstDay int

	// DeadDays counts days on which the node stopped. Non-zero means the site
	// does not work, regardless of how good the annual average looks.
	DeadDays int

	// AutonomyDays is how long it lasts from full with no sun at all, at the
	// darkest month's temperature. The number to quote when someone asks "what
	// if it snows over".
	AutonomyDays float64
}

// SimulateYear runs a site through a year at hourly resolution.
//
// Hourly rather than daily because the thing being tested is whether the pack
// gets through the night, and a daily energy balance cannot see a night. It
// starts from a full battery on 1 January, which is deliberately the hardest
// starting assumption to be wrong about in the optimistic direction: a UK site
// that survives from a full pack in January survives.
func SimulateYear(s Site) (YearResult, error) {
	if s.Battery.CapacityMAh <= 0 {
		return YearResult{}, fmt.Errorf("energy: %s: battery capacity is %.0f mAh", s.Name, s.Battery.CapacityMAh)
	}
	if s.Battery.Cells < 1 {
		return YearResult{}, fmt.Errorf("energy: %s: battery has %d cells", s.Name, s.Battery.Cells)
	}

	res := YearResult{WorstSoC: 1}
	soc := 1.0

	for day := 1; day <= 365; day++ {
		month := monthOf(day)
		cloud := s.CloudByMonth[month]
		tempC := s.TempCByMonth[month]

		avgMA := s.Load.AverageMA(s.Duty, s.TxPowerDBm)
		capacityMAh := s.Battery.CapacityAt(tempC, avgMA)
		packV := s.Battery.VoltageAt(soc)
		capacityWh := capacityMAh / 1000 * s.Battery.VoltageAt(0.5)

		d := Day{DayOfYear: day, MinSoC: soc, MeanCurrmMA: avgMA}

		for hour := 0.0; hour < 24; hour++ {
			sun := SunAt(s.LatDeg, s.LonDeg, day, hour+0.5)
			harvestW := s.Panel.HarvestW(sun, cloud)
			if s.Horizon != nil && sun.ElevationDeg > 0 &&
				sun.ElevationDeg < s.Horizon(sun.AzimuthDeg) {
				// Behind the hill: the direct beam is gone, the sky is not.
				// Full-overcast harvest is the closest honest stand-in for
				// diffuse-only light.
				harvestW = s.Panel.HarvestW(sun, 1)
			}
			drawW := avgMA / 1000 * packV

			d.HarvestWh += harvestW
			d.ConsumedWh += drawW
			if sun.ElevationDeg > 0 {
				d.SunHoursEq += s.Panel.HarvestW(sun, cloud) / math.Max(s.Panel.PeakW, 1e-9)
			}

			if capacityWh > 0 {
				soc = clamp(soc+(harvestW-drawW)/capacityWh, 0, 1)
			}
			packV = s.Battery.VoltageAt(soc)
			if soc < d.MinSoC {
				d.MinSoC = soc
			}
			if s.Battery.Dead(soc) {
				d.Dead = true
			}
		}

		d.EndSoC = soc
		if d.Dead {
			res.DeadDays++
		}
		if d.MinSoC < res.WorstSoC {
			res.WorstSoC, res.WorstDay = d.MinSoC, day
		}
		res.Days = append(res.Days, d)
	}

	res.AutonomyDays = autonomy(s)
	return res, nil
}

// autonomy is days from full with no harvest, at the coldest month.
func autonomy(s Site) float64 {
	coldest := 0
	for m := 1; m < 12; m++ {
		if s.TempCByMonth[m] < s.TempCByMonth[coldest] {
			coldest = m
		}
	}
	avgMA := s.Load.AverageMA(s.Duty, s.TxPowerDBm)
	if avgMA <= 0 {
		return math.Inf(1)
	}
	capacityMAh := s.Battery.CapacityAt(s.TempCByMonth[coldest], avgMA)

	// Usable capacity stops at the cutoff voltage, not at zero charge. A pack
	// whose regulator drops out at 60% state of charge has 60% less autonomy
	// than its capacity suggests, and that is a real and common outcome.
	usable := 1.0
	for soc := 0.0; soc <= 1.0; soc += 0.01 {
		if !s.Battery.Dead(soc) {
			usable = 1 - soc
			break
		}
	}
	return capacityMAh * usable / avgMA / 24
}

// monthOf is the zero-based month containing a day of the year. Non-leap.
func monthOf(dayOfYear int) int {
	cum := [...]int{31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334, 365}
	for m, end := range cum {
		if dayOfYear <= end {
			return m
		}
	}
	return 11
}
