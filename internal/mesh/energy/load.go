package energy

import "math"

// Load is what a node draws, by radio state.
//
// Currents are from the SX1262 and nRF52840 datasheets rather than measured
// boards, so they are the optimistic end: a real board adds a regulator, an
// LED nobody removed, and a GPS that someone left enabled.
type Load struct {
	// SleepUA is deep sleep for the whole board. A well-built nRF52840 node is
	// a few microamps; a board with a linear regulator is hundreds.
	SleepUA float64

	// IdleMA is the MCU awake with the radio in standby.
	IdleMA float64

	// RxMA is receiving. A LoRa node in a mesh spends nearly all its time here,
	// so this — not transmit — is usually what sets battery life.
	RxMA float64

	// TxMA is transmitting, by power level. The SX1262's supply current is
	// strongly non-linear in output power, so a single figure is wrong by a
	// factor of three across the range.
	TxMA func(dBm float64) float64
}

// SX1262Load is a typical nRF52840 + SX1262 repeater.
func SX1262Load() Load {
	return Load{
		SleepUA: 20,
		IdleMA:  3.5,
		RxMA:    5.5,
		TxMA:    SX1262TxCurrentMA,
	}
}

// SX1262TxCurrentMA is supply current against output power.
//
// Interpolated from the datasheet's tabulated figures. Straight-line
// interpolation between them rather than a fitted curve, because the datasheet
// points are the only thing actually measured and a smooth fit would invent
// precision between them.
func SX1262TxCurrentMA(dBm float64) float64 {
	points := []struct{ dBm, mA float64 }{
		{-9, 15}, {0, 18}, {10, 27}, {14, 38}, {17, 58}, {20, 87}, {22, 118},
	}
	if dBm <= points[0].dBm {
		return points[0].mA
	}
	last := points[len(points)-1]
	if dBm >= last.dBm {
		return last.mA
	}
	for i := 1; i < len(points); i++ {
		if dBm <= points[i].dBm {
			a, b := points[i-1], points[i]
			f := (dBm - a.dBm) / (b.dBm - a.dBm)
			return a.mA + (b.mA-a.mA)*f
		}
	}
	return last.mA
}

// Duty is how a node spends a period of time.
//
// Fractions of the interval, and they must sum to at most 1 — whatever is left
// over is sleep. Expressed this way because it is what the simulation can
// actually measure: airtime transmitted, airtime received, and the rest.
type Duty struct {
	TxFraction float64
	RxFraction float64
	// IdleFraction is awake but not on the radio. Nodes that never sleep set
	// this to whatever is left rather than leaving it to sleep, and that single
	// choice is usually the difference between a month and a year.
	IdleFraction float64
}

// AverageMA is the mean current for a duty cycle at a transmit power.
func (l Load) AverageMA(d Duty, txDBm float64) float64 {
	tx := clamp(d.TxFraction, 0, 1)
	rx := clamp(d.RxFraction, 0, 1)
	idle := clamp(d.IdleFraction, 0, 1)
	sleep := math.Max(0, 1-tx-rx-idle)

	txMA := 0.0
	if l.TxMA != nil {
		txMA = l.TxMA(txDBm)
	}
	return tx*txMA + rx*l.RxMA + idle*l.IdleMA + sleep*l.SleepUA/1000
}

// DutyFromAirtime turns measured airtime into a duty cycle.
//
// This is the seam that makes the energy model honest: the transmit fraction
// comes from what the node actually sent, as the channel timed it, rather than
// from an assumed message rate. A node that is being flooded by its neighbours
// draws more, and that shows up here without anyone configuring it.
func DutyFromAirtime(txMillis, rxMillis, periodMillis float64, alwaysOn bool) Duty {
	if periodMillis <= 0 {
		return Duty{}
	}
	d := Duty{
		TxFraction: clamp(txMillis/periodMillis, 0, 1),
		RxFraction: clamp(rxMillis/periodMillis, 0, 1),
	}
	if alwaysOn {
		// A repeater listens continuously: everything not spent transmitting is
		// spent receiving, not sleeping.
		d.RxFraction = math.Max(0, 1-d.TxFraction)
	}
	return d
}
