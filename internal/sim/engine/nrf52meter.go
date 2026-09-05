// The nRF52840's converter, as far as a battery reading is concerned.
//
// Its own file because the two parts have nothing in common but the question.
// An ESP32-S3 reading goes through a calibration curve baked into the part's
// fuses, and getting the true voltage back means inverting that exact curve;
// an nRF52 reading is a straight ratio of a reference the firmware picks, so
// the arithmetic here is one multiplication and the interesting part is which
// input the pin is.
package engine

import (
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
)

// nrf52Meter is what this board's cell puts on its converter at a full charge.
//
// FullScaleMV is declared as the cell voltage that fills the converter, so the
// raw is a straight proportion of it - and that one rule covers all four boards
// here despite their arithmetic being written three different ways. A Heltec
// scales a 12-bit reading against the internal 3.0 V reference by 4.9, a RAK
// multiplies by 6160.53 and divides by 4096, a XIAO multiplies by 9 and divides
// by 4.096. Each is linear, and each declared its own full scale, so encoding
// the raw against that returns the firmware's own answer to the cell voltage it
// started from.
//
// Renode's converter has no per-channel value: our model answers every channel
// with the same sample. The channel is still worked out and still reported,
// because it is a fact about the board and the row that shows it should be
// right on the day the model grows a second input.
func nrf52Meter(board hw.Board) (meterReading, bool) {
	p := board.Hardware
	if p == nil {
		return meterReading{}, false
	}
	for _, part := range p.PartsOfKind(hw.Meter) {
		if part.Pin == hw.PinNone || part.FullScaleMV <= 0 {
			continue
		}
		ch, ok := nrf52Input(part.Pin)
		if !ok {
			continue
		}
		cellMV := board.Battery.VoltageAt(1) * 1000
		raw := cellMV * nrf52FullScaleCounts / float64(part.FullScaleMV)
		if raw < 0 {
			raw = 0
		}
		if raw > nrf52FullScaleCounts {
			raw = nrf52FullScaleCounts
		}
		return meterReading{channel: ch, raw: uint16(raw + 0.5)}, true
	}
	return meterReading{}, false
}

// nrf52FullScaleCounts is the largest a 12-bit conversion reads. Every MeshCore
// nRF52 board here calls analogReadResolution(12) before reading its cell, and
// each board's FullScaleMV was derived against this same number, so the two
// have to agree or every reading is out by a part in four thousand.
const nrf52FullScaleCounts = 4095.0

// nrf52Input is which analogue input a pin is on an nRF52840: AIN0 to AIN7 are
// P0.02 to P0.05 and P0.28 to P0.31, and nothing else on the chip is one.
//
// A pin outside that set is reported as no input rather than as input zero,
// which would put a board's cell on somebody else's pin. Two of the four boards
// state their own answer in a comment - the RAK4631's variant says "AIN3 =
// P0.05" and the XIAO's says "AIN7 = P0.31" - and both agree with this table.
func nrf52Input(pin int) (int, bool) {
	switch {
	case pin >= 2 && pin <= 5:
		return pin - 2, true
	case pin >= 28 && pin <= 31:
		return pin - 24, true
	}
	return 0, false
}
