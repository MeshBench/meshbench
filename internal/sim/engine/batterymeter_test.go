package engine

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The emulated ESP32-S3 carries ADC calibration in eFuse (BLK_VERSION_MAJOR=1),
// so the firmware maps a raw reading through ESP-IDF's non-linear V1 curve.
// batteryMeter must encode the raw as the inverse of that curve, so these tests
// pin the curve to the firmware's own arithmetic and check the round-trip.

// TestADC1Atten3CurveRoundTrip is the property the whole fix rests on: a raw
// encoded for a voltage reads back as that voltage through the same curve.
func TestADC1Atten3CurveRoundTrip(t *testing.T) {
	// Across the range a single LiIon cell behind a halving divider presents:
	// ~1.5 V (empty) to ~2.1 V (full) at the pin.
	for mv := 1400.0; mv <= 2200.0; mv += 50 {
		raw := adc1Atten3RawForVoltage(mv)
		back := adc1Atten3Voltage(float64(raw))
		if math.Abs(back-mv) > 1.0 {
			t.Errorf("round-trip at %.0f mV: raw=%d reads back %.1f mV (off by %.1f)",
				mv, raw, back, back-mv)
		}
	}
}

// TestADC1Atten3CurveMonotonic guards the bisection: the curve must rise with
// the raw code, or the inverse would not converge.
func TestADC1Atten3CurveMonotonic(t *testing.T) {
	prev := adc1Atten3Voltage(0)
	for raw := 1; raw <= 4095; raw++ {
		v := adc1Atten3Voltage(float64(raw))
		if v <= prev {
			t.Fatalf("curve not monotonic at raw=%d: %.3f <= %.3f", raw, v, prev)
		}
		prev = v
	}
}

// TestBatteryMeterReportsTrueVoltage is the no-regression check: the raw
// batteryMeter hands the converter, read back through the calibration curve and
// scaled by the board's own halving divider, is the true cell voltage. The
// T-Deck documents that divider (3300 mV at the pin, 6600 mV at the cell), so
// the firmware reports the full-charge voltage rather than the ~15% high the
// bare eFuse burn would have produced.
func TestBatteryMeterReportsTrueVoltage(t *testing.T) {
	board, err := scenario.BoardByName("LilyGo_TDeck")
	if err != nil {
		t.Fatalf("looking up the T-Deck: %v", err)
	}
	meter, ok := batteryMeter(board)
	if !ok {
		t.Fatal("the T-Deck declares a battery meter; batteryMeter returned none")
	}

	// The divider the board declares: cell volts that fill the converter over
	// pin volts that do.
	var fullScaleMV float64
	for _, part := range board.Hardware.PartsOfKind(scenario.Meter) {
		if part.FullScaleMV > 0 {
			fullScaleMV = float64(part.FullScaleMV)
		}
	}
	divider := fullScaleMV / adc1Atten3FullScaleMV

	pinMV := adc1Atten3Voltage(float64(meter.raw))
	reportedMV := pinMV * divider
	trueMV := board.Battery.VoltageAt(1) * 1000

	if math.Abs(reportedMV-trueMV) > 15.0 {
		t.Errorf("T-Deck full charge: converter raw=%d reads %.0f mV at the pin, "+
			"reported %.0f mV, want the true %.0f mV", meter.raw, pinMV, reportedMV, trueMV)
	}
}
