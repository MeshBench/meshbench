// A repeater site's year on solar, for the selected node (6.19).
//
// Through internal/energy, which runs the year hourly rather than daily: the
// thing being tested is whether the pack gets through the night, and a daily
// energy balance cannot see a night.
//
// The duty cycle comes from the run rather than from a form. A site sized
// against a guessed duty is a site sized against a guess, and the simulator is
// sitting right there having measured the real one.
package main

import (
	"github.com/A13xB0/meshcoresim/internal/energy"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// A typical UK hilltop repeater, stated here so that changing it is a decision
// rather than a discovery. These are the defaults the panel starts from; the
// point of the panel is to change them and watch the worst day move.
var (
	defaultBattery = energy.Battery{
		Chemistry: energy.LiFePO4, CapacityMAh: 20000, Cells: 4,
		// The regulator's dropout, not the cell's floor: a node dies when its
		// 3.3 V rail collapses, not when the cell is empty.
		CutoffV: 10.0,
	}
	defaultPanel = energy.Panel{
		PeakW: 50, TiltDeg: 50, AzimuthDeg: 180, SoilingFactor: 0.8,
	}
	defaultLoad = energy.Load{SleepUA: 20, IdleMA: 12, RxMA: 14}
	// Scotland, monthly mean cloud cover and temperature. Not climate data -
	// that is a separate problem - but stated numbers rather than an implied
	// clear sky, which would make every site look like it works.
	ukCloud = [12]float64{0.75, 0.72, 0.68, 0.62, 0.58, 0.60, 0.62, 0.63, 0.65, 0.70, 0.76, 0.78}
	ukTempC = [12]float64{4, 4, 6, 8, 11, 14, 16, 16, 13, 10, 6, 4}
)

// energyFor simulates a year at one node, using the duty cycle the run
// measured for it.
func energyFor(n scenario.Node, dutyPct float64) (*state.Energy, error) {
	tx := dutyPct / 100
	if tx < 0 {
		tx = 0
	}
	site := energy.Site{
		Name: n.Name, LatDeg: n.Position.Lat, LonDeg: n.Position.Lon,
		Battery: defaultBattery, Panel: defaultPanel, Load: defaultLoad,
		// Whatever is not transmitting is listening: a repeater's receiver is
		// what sets its battery life, not its transmitter, and assuming it
		// sleeps between packets would flatter the site by a factor.
		Duty:         energy.Duty{TxFraction: tx, RxFraction: 1 - tx},
		TxPowerDBm:   n.TxPowerDBm,
		CloudByMonth: ukCloud,
		TempCByMonth: ukTempC,
	}
	res, err := energy.SimulateYear(site)
	if err != nil {
		return nil, err
	}
	out := &state.Energy{
		Node: n.Name, WorstSoC: res.WorstSoC, WorstDay: res.WorstDay,
		DeadDays: res.DeadDays, AutonomyDays: res.AutonomyDays,
		DutyPct: dutyPct,
		SoC:     make([]float64, 0, len(res.Days)),
	}
	for _, d := range res.Days {
		out.SoC = append(out.SoC, d.MinSoC)
	}
	return out, nil
}
