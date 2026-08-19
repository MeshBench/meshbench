// Package energy models what keeps a node alive: battery, load and solar.
//
// A repeater on a hill fails for two reasons. One is RF, which the rest of the
// simulator is about. The other is that it ran out of charge in December, and
// no amount of link-budget work finds that. This package is the second reason.
package energy

import "math"

// Chemistry is the cell type, which sets both the discharge curve and how
// badly the cold hurts.
type Chemistry string

const (
	// LiIon covers 18650 and pouch cells: the usual choice, and the one that
	// must not be charged below freezing.
	LiIon Chemistry = "li-ion"
	// LiFePO4 is flatter, tolerates cold charging, and holds a lower voltage.
	LiFePO4 Chemistry = "lifepo4"
	// Alkaline is for a node someone posts out and forgets. Its voltage sags
	// continuously, which matters because the regulator gives up before the
	// chemistry does.
	Alkaline Chemistry = "alkaline"
)

// Battery is a pack.
type Battery struct {
	Chemistry Chemistry

	// CapacityMAh at 20 °C and a gentle discharge. Real capacity is lower in
	// the cold and lower again at high current, which Capacity() applies.
	CapacityMAh float64

	// Cells in series. Sets the pack voltage; parallel strings belong in
	// CapacityMAh.
	Cells int

	// CutoffV is the pack voltage at which the node stops. Usually the
	// regulator's dropout rather than the cell's own floor — a node dies when
	// its 3.3 V rail collapses, not when the cell is empty.
	CutoffV float64
}

// nominalV, fullV and emptyV per cell.
func (b Battery) cellVoltages() (nominal, full, empty float64) {
	switch b.Chemistry {
	case LiFePO4:
		return 3.2, 3.65, 2.5
	case Alkaline:
		return 1.5, 1.6, 0.9
	default: // LiIon
		return 3.7, 4.2, 3.0
	}
}

// VoltageAt is the pack voltage at a state of charge in [0,1].
//
// Piecewise rather than linear because the shape is the whole point: Li-ion
// spends most of its life near nominal and then falls off a cliff, which is why
// a voltage-based fuel gauge reads "fine" until it very suddenly does not.
func (b Battery) VoltageAt(soc float64) float64 {
	soc = clamp(soc, 0, 1)
	nominal, full, empty := b.cellVoltages()
	cells := float64(b.Cells)
	if cells < 1 {
		cells = 1
	}

	var v float64
	switch {
	case b.Chemistry == Alkaline:
		// A steady sag from the start; no plateau to speak of.
		v = empty + (full-empty)*math.Pow(soc, 0.75)
	case soc > 0.9:
		v = nominal + (full-nominal)*(soc-0.9)/0.1
	case soc > 0.15:
		// The plateau. Slightly sloped, so a reading is not perfectly ambiguous.
		v = nominal + (nominal-empty)*0.12*(soc-0.15)/0.75
	default:
		v = empty + (nominal-empty)*(soc/0.15)
	}
	return v * cells
}

// CapacityAt is usable capacity in mAh at a cell temperature and average
// discharge current.
//
// Both corrections matter for the case this package exists for. A pack on a
// Scottish hilltop in January is at -5 °C, and the specification sheet figure
// is not what it will deliver.
func (b Battery) CapacityAt(temperatureC, currentMA float64) float64 {
	cap := b.CapacityMAh * temperatureDerate(b.Chemistry, temperatureC)

	// Peukert-ish correction for high discharge. A mesh node's average current
	// is small relative to pack capacity, so this is usually a percent or two —
	// included because it is free and its absence would be a silent optimism.
	if cap > 0 && currentMA > 0 {
		hourRate := currentMA / cap
		if hourRate > 0.2 {
			cap *= math.Pow(0.2/hourRate, 0.05)
		}
	}
	return cap
}

// temperatureDerate is the fraction of rated capacity available.
//
// Figures are the usual manufacturer curves: Li-ion loses roughly a third of
// its capacity by -20 °C, LiFePO4 rather more, alkaline much more. Above 20 °C
// there is no gain worth modelling — the small increase is offset by
// accelerated ageing this does not track.
func temperatureDerate(c Chemistry, tC float64) float64 {
	if tC >= 20 {
		return 1.0
	}
	var lossPerDegree float64
	switch c {
	case LiFePO4:
		lossPerDegree = 0.018
	case Alkaline:
		lossPerDegree = 0.025
	default:
		lossPerDegree = 0.013
	}
	return clamp(1-(20-tC)*lossPerDegree, 0.15, 1.0)
}

// Dead reports whether the node has stopped.
//
// Empty counts as dead regardless of the cutoff. A pack configured with a
// cutoff at or below the cell's own floor would otherwise never be reported
// dead — it would sit at zero charge, still nominally running, and the year
// would come out survivable.
func (b Battery) Dead(soc float64) bool {
	return soc <= 0 || b.VoltageAt(soc) < b.CutoffV
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
