package dsp

import "math"

// PreambleSymbols is the preamble length MeshCore configures, which is not
// RadioLib's default of 8.
//
// From RadioLibWrappers.h: `sf <= 8 ? 32 : 16`. It matters more than it looks —
// at SF7 a 32-symbol preamble is about 33 ms, which is a large fraction of a
// short packet's airtime and therefore of the duty-cycle budget the firmware
// polices itself with.
func PreambleSymbols(sf int) int {
	if sf <= 8 {
		return 32
	}
	return 16
}

// AirtimeMillis is the LoRa time on air for a payload of n bytes, in
// milliseconds, computed the way the firmware computes it.
//
// This is not "an airtime formula" — it is *RadioLib's* `getTimeOnAir()`,
// truncated to milliseconds, because that is literally what MeshCore's
// RadioLibWrapper::getEstAirtimeFor() returns. The firmware's CSMA timing,
// duty-cycle budget and send-timeout are all built on this number. If our
// channel occupies the air for a different length of time than the firmware
// believes it did, the two desynchronise silently and every collision result
// after that is fiction.
//
// cr is the coding rate denominator offset, 1..4 for 4/5..4/8. crc reflects
// whether the hardware CRC is enabled; explicitHeader reflects the header mode.
func AirtimeMillis(sf int, bandwidthHz float64, cr int, n int, crc, explicitHeader bool) float64 {
	// Symbol length in milliseconds — the unit RadioLib works in, and the unit
	// the low-data-rate threshold below is expressed in.
	symbolMs := float64(uint64(1)<<uint(sf)) / (bandwidthHz / 1000.0)

	// SF5 and SF6 use a different preamble arrangement on the SX126x.
	sfCoeff1, sfCoeff2 := 4.25, 8.0
	if sf == 5 || sf == 6 {
		sfCoeff1, sfCoeff2 = 6.25, 0.0
	}

	// Low data rate optimisation, which the chip enables itself once a symbol
	// reaches 16 ms — SF11 and SF12 at 125 kHz. It costs two bits per symbol.
	sfDivisor := 4 * sf
	if symbolMs >= 16.0 {
		sfDivisor = 4 * (sf - 2)
	}

	headerSymbols := 0
	if explicitHeader {
		headerSymbols = 20
	}
	crcBits := 0
	if crc {
		crcBits = 16
	}

	bits := 8*n + crcBits - 4*sf + int(sfCoeff2) + headerSymbols
	if bits < 0 {
		bits = 0
	}
	coded := (bits + sfDivisor - 1) / sfDivisor

	// The lone +8 is the sync word and header symbols. It belongs to the frame,
	// not to the payload — adding it in both places is an easy 8-symbol error
	// that at SF12 is 262 ms of airtime that does not exist.
	total := float64(PreambleSymbols(sf)) + sfCoeff1 + 8 + float64(coded*(cr+4))
	// RadioLib returns microseconds and MeshCore divides by 1000 with integer
	// truncation, so the firmware sees a whole number of milliseconds. Matching
	// that truncation is the point.
	return math.Trunc(symbolMs * total)
}
