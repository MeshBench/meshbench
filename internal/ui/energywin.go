package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/energy"
)

// energyState is the Energy panel: the winter question, for the selection.
type energyState struct {
	panelW     float32
	tiltDeg    float32
	batteryMAh float32

	forNode string
	result  *energy.YearResult
	err     string
	dutyTx  float64 // the duty the result was computed with, for the caption
}

// drawEnergyBody answers "does this node survive December".
//
// The draw comes from what the firmware actually did — the run's own airtime —
// which is the whole payoff of running real firmware: a chatty mesh drains a
// battery faster, and here that is measured rather than estimated.
func (a *App) drawEnergyBody() {
	s := &a.energy
	if s.panelW == 0 {
		s.panelW, s.tiltDeg, s.batteryMAh = 10, 55, 6800
	}

	if a.selected < 0 || a.selected >= len(a.Nodes) {
		imgui.TextDisabled("select a node on the map - the question is about a site")
		return
	}
	n := &a.Nodes[a.selected]
	if !n.Kind.Transmits() {
		imgui.TextDisabled("an SDR observer draws almost nothing; pick a repeater")
		return
	}

	imgui.Text(n.Name)
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("%.4f N  -  %.0f dBm", n.Position.Lat, n.TxPowerDBm))

	imgui.SetNextItemWidth(120)
	imgui.SliderFloat("panel W", &s.panelW, 0, 40)
	imgui.SetNextItemWidth(120)
	imgui.SliderFloat("tilt deg", &s.tiltDeg, 0, 90)
	imgui.SetNextItemWidth(120)
	imgui.SliderFloat("battery mAh", &s.batteryMAh, 1000, 30000)

	tx, rx, period := a.observedDuty(n.Name)
	if period > 0 {
		imgui.TextDisabled(fmt.Sprintf("draw from this run: %.0f ms of transmit in %.0f s",
			tx, period/1000))
	} else {
		imgui.TextDisabled("no run yet - assuming a quiet mesh; run real firmware and the\n" +
			"draw becomes what the firmware actually did")
	}

	if imgui.Button("simulate a year here") {
		a.runEnergyYear(n.Name, tx, rx, period)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Hourly for 365 days: solar geometry for this latitude, the DEM's\n" +
			"own horizon around this position (a panel behind a hill loses its\n" +
			"mornings), monthly UK cloud means, and the run's measured duty.")
	}

	if s.err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		imgui.TextWrapped(s.err)
		imgui.PopStyleColor()
	}
	if s.result == nil || s.forNode != n.Name {
		return
	}
	res := s.result

	imgui.SeparatorText("The year")
	worst := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, res.WorstDay-1)
	switch {
	case res.DeadDays > 0:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		imgui.TextWrapped(fmt.Sprintf("FAILS - dead on %d days of the year, first trouble around %s",
			res.DeadDays, worst.Format("2 January")))
		imgui.PopStyleColor()
	case res.WorstSoC < 0.3:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		imgui.TextWrapped(fmt.Sprintf("MARGINAL - %.0f%% at the worst (%s), and nothing here "+
			"allows for snow on the panel or a worse winter than average",
			res.WorstSoC*100, worst.Format("2 January")))
		imgui.PopStyleColor()
	default:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.45, 0.85, 0.5, 1))
		imgui.TextWrapped(fmt.Sprintf("survives the year - lowest charge %.0f%% on %s",
			res.WorstSoC*100, worst.Format("2 January")))
		imgui.PopStyleColor()
	}
	imgui.TextDisabled(fmt.Sprintf("autonomy: %.1f days from full with no sun at all",
		res.AutonomyDays))

	a.drawSoCSparkline(res)
	imgui.TextDisabled("daily minimum charge across the year; the dip is the answer")
}

// drawSoCSparkline is the year at a glance: 365 daily minima as one strip.
func (a *App) drawSoCSparkline(res *energy.YearResult) {
	avail := imgui.ContentRegionAvail()
	w := math.Max(240, float64(avail.X))
	const h = 64.0
	pos := imgui.CursorScreenPos()
	dl := imgui.WindowDrawList()
	x0, y0 := float64(pos.X), float64(pos.Y)
	dl.AddRectFilled(imgui.NewVec2(float32(x0), float32(y0)),
		imgui.NewVec2(float32(x0+w), float32(y0+h)), colour(0.09, 0.10, 0.13, 1))
	for i, d := range res.Days {
		x := float32(x0 + float64(i)/365*w)
		barH := float32(d.MinSoC * h)
		col := colour(0.45, 0.85, 0.5, 0.85)
		if d.Dead {
			col = colour(0.9, 0.35, 0.35, 1)
		} else if d.MinSoC < 0.3 {
			col = colour(0.95, 0.72, 0.25, 0.9)
		}
		dl.AddLineArgs(imgui.NewVec2(x, float32(y0+h)), imgui.NewVec2(x, float32(y0+h)-barH), col, 1)
	}
	imgui.Dummy(imgui.NewVec2(float32(w), float32(h)))
}

// observedDuty is what the run says this node actually did on the air.
func (a *App) observedDuty(name string) (txMs, rxMs, periodMs float64) {
	if a.eng == nil || a.eng.NowMs() == 0 {
		return 0, 0, 0
	}
	for _, row := range a.eng.Scoreboard() {
		if row.Name == name {
			period := float64(a.eng.NowMs())
			// A repeater listens whenever it is not transmitting.
			return row.AirtimeMs, period - row.AirtimeMs, period
		}
	}
	return 0, 0, 0
}

// runEnergyYear computes the budget, with the DEM's own horizon.
func (a *App) runEnergyYear(name string, txMs, rxMs, periodMs float64) {
	s := &a.energy
	i := a.nodeIndex(name)
	if i < 0 {
		return
	}
	n := a.Nodes[i]

	duty := energy.DutyFromAirtime(1, 0, 1000, true) // quiet-mesh default
	s.dutyTx = 0
	if periodMs > 0 {
		duty = energy.DutyFromAirtime(txMs, rxMs, periodMs, true)
		s.dutyTx = txMs / periodMs
	}
	site := energy.Site{
		Name: name, LatDeg: n.Position.Lat, LonDeg: n.Position.Lon,
		Battery: energy.Battery{
			Chemistry: energy.LiIon, CapacityMAh: float64(s.batteryMAh), Cells: 1, CutoffV: 3.1,
		},
		Panel: energy.Panel{
			PeakW: float64(s.panelW), TiltDeg: float64(s.tiltDeg), AzimuthDeg: 180,
			SoilingFactor: 0.8, ChargeEfficiency: 0.95,
		},
		Load:       energy.SX1262Load(),
		Duty:       duty,
		TxPowerDBm: n.TxPowerDBm,
		CloudByMonth: [12]float64{0.75, 0.72, 0.68, 0.62, 0.58, 0.58,
			0.60, 0.62, 0.65, 0.72, 0.78, 0.80},
		TempCByMonth: [12]float64{1, 1, 3, 5, 8, 11, 13, 13, 10, 7, 3, 1},
		Horizon:      a.terrainHorizon(n.Position.Lat, n.Position.Lon, n.HeightAGLm),
	}
	res, err := energy.SimulateYear(site)
	if err != nil {
		s.err, s.result = err.Error(), nil
		return
	}
	s.err, s.result, s.forNode = "", &res, name
}

// terrainHorizon samples the DEM around a position and returns the horizon's
// elevation angle by azimuth — the sun-path mask ADR-0011 calls the feature's
// real differentiator, and it is nearly free because the terrain is loaded.
func (a *App) terrainHorizon(lat, lon, aglM float64) func(float64) float64 {
	h0, ok := a.Terrain.ElevationM(lat, lon)
	if !ok {
		return nil
	}
	h0 += aglM
	const (
		sectors = 36   // every 10 degrees
		maxKm   = 10.0 // beyond this a hill subtends under a tenth of a degree per 17 m
		stepKm  = 0.25 // DEM resolution scale
	)
	mask := make([]float64, sectors)
	for sct := 0; sct < sectors; sct++ {
		az := float64(sct) * 360 / sectors * math.Pi / 180
		best := 0.0
		for d := stepKm; d <= maxKm; d += stepKm {
			dLat := d / 111.32 * math.Cos(az)
			dLon := d / (111.32 * math.Cos(lat*math.Pi/180)) * math.Sin(az)
			h, ok := a.Terrain.ElevationM(lat+dLat, lon+dLon)
			if !ok {
				continue
			}
			if ang := math.Atan2(h-h0, d*1000) * 180 / math.Pi; ang > best {
				best = ang
			}
		}
		mask[sct] = best
	}
	return func(azimuthDeg float64) float64 {
		s := int(math.Mod(azimuthDeg+360, 360) / (360 / sectors))
		if s < 0 || s >= sectors {
			return 0
		}
		return mask[s]
	}
}
