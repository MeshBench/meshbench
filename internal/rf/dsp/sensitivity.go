package dsp

import (
	"runtime"
	"sync"
)

// RequiredSNRdB holds Semtech's published demodulator SNR floor for the SX1262,
// per spreading factor. These are the figures MSIM-2's acceptance test measures
// against: if the simulated modem does not land close to them, the chain is
// wrong however convincing a waterfall looks.
//
// Note what they are: packet-level sensitivity with forward error correction, at
// a stated packet error rate. A raw uncoded symbol-error measurement is a
// related but not identical quantity, so compare the *spacing* between spreading
// factors as well as the absolute values — the 2.5 dB per SF step is the part
// that falls straight out of the physics.
var RequiredSNRdB = map[int]float64{
	7:  -7.5,
	8:  -10.0,
	9:  -12.5,
	10: -15.0,
	11: -17.5,
	12: -20.0,
}

// ReportableSNRCeilingDB is the highest signal-to-noise ratio an SX126x can
// tell anyone about, and therefore the highest a MeshCore node can ever report.
//
// The estimator saturates. A signal 20 dB above the noise floor and one 60 dB
// above it come back as the same number, because the quantity is read out of a
// register with a finite span - so this is a property of the receiver, not of
// the air, and no amount of transmit power moves it.
//
// The figure is measured, not assumed: 1,992 receptions carrying SNR from the
// ScotMesh network have a hard wall here, a median of +5.0 dB, a 90th
// percentile of +13.0 dB, and not one reception above +15.0 dB.
//
// There is deliberately no matching floor. The real distribution stops near
// -13 dB, but that is the demodulator giving up rather than the estimator
// saturating - packets below it are not reported because they are not
// received - and clamping there would put a floor under exactly the failures
// RequiredSNRdB exists to judge.
const ReportableSNRCeilingDB = 15.0

// ReportSNRdB is what the modem would have said about a computed ratio.
//
// Use it wherever an SNR leaves the simulator - a ledger row, an event, a
// companion's packet status - and never for a decode decision, which is the
// unclamped ratio's job. Skipping it emits readings no field instrument can
// express: +94.1 dB was measured between two hilltop nodes here, against a
// real-world maximum of +15.0 dB, and every consumer downstream inherited a
// number that could not be compared with a measurement.
func ReportSNRdB(snrDB float64) float64 {
	if snrDB > ReportableSNRCeilingDB {
		return ReportableSNRCeilingDB
	}
	return snrDB
}

// ReportableRSSIFloorDBm and ReportableRSSICeilingDBm bound what an SX126x
// can say about received power: the RssiPkt register is an unsigned byte read
// out as -RssiPkt/2, so the strongest reportable signal is 0 dBm and the
// weakest is -127.5. The 2,000 ScotMesh packets agree exactly - RSSI spans
// -127 to 0 and nothing outside it - which is the same shape of evidence as
// the SNR wall: the register's span is the instrument's vocabulary.
const (
	ReportableRSSIFloorDBm   = -127.5
	ReportableRSSICeilingDBm = 0.0
)

// ReportRSSIdBm is what the modem would have said about a computed receive
// power. Same contract as ReportSNRdB: for readings that leave the simulator,
// never for decisions, which keep the unclamped figure.
func ReportRSSIdBm(rssiDBm float64) float64 {
	if rssiDBm > ReportableRSSICeilingDBm {
		return ReportableRSSICeilingDBm
	}
	if rssiDBm < ReportableRSSIFloorDBm {
		return ReportableRSSIFloorDBm
	}
	return rssiDBm
}

// SymbolErrorRate runs a Monte Carlo trial: modulate random symbols at the given
// SNR, demodulate, and return the fraction wrong.
//
// Deterministic in seed, so a measured sensitivity figure is reproducible and a
// regression is attributable.
func SymbolErrorRate(sf int, snrDB float64, trials int, seed uint64) float64 {
	m, d := Modulator{SF: sf}, Demodulator{SF: sf}
	n := SamplesPerSymbol(sf)
	rng := Philox{Seed: seed}

	// A modulated chirp has unit amplitude, so its mean power is 1; deriving it
	// anyway keeps this correct if the modulator ever changes scale.
	sigPower := SignalPower(m.ModulateSymbol(0))
	noisePower := NoisePowerForSNR(sigPower, snrDB)

	errors := 0
	for t := 0; t < trials; t++ {
		want := int(rng.uint64At(uint64(t)) % uint64(n))
		rx := m.ModulateSymbol(want)
		rng.AddAWGN(rx, noisePower, uint64(t)*uint64(n)+1<<40)
		got, _ := d.DemodulateSymbol(rx)
		if got != want {
			errors++
		}
	}
	return float64(errors) / float64(trials)
}

// PacketErrorRate is the quantity Semtech's sensitivity figures actually
// describe: a packet fails if *any* of its symbols is wrong.
//
// This distinction is not pedantic — it is worth roughly 2 dB. Measuring symbol
// error rate and comparing it to a published packet figure makes a modem look
// better than it is, in exactly the direction this project is prone to.
func PacketErrorRate(sf int, snrDB float64, symbolsPerPacket, packets int, seed uint64) float64 {
	if packets <= 0 {
		return 0
	}
	m := Modulator{SF: sf}
	n := SamplesPerSymbol(sf)
	sigPower := SignalPower(m.ModulateSymbol(0))
	noisePower := NoisePowerForSNR(sigPower, snrDB)

	// Split across cores.
	//
	// Every packet's noise and symbol values come from Philox counters derived
	// from the packet index, so a packet's outcome does not depend on which
	// goroutine ran it or in what order. The result is bit-identical to the
	// serial version — which TestPacketErrorRateIsDeterministic asserts — and
	// this is the only reason splitting it is safe at all.
	//
	// It matters because the six spreading factors in the sensitivity sweep
	// finish at wildly different times: SF7 is cheap, SF12 is not, and without
	// this the whole sweep ends with eleven idle cores waiting for SF12.
	workers := runtime.GOMAXPROCS(0)
	if workers > packets {
		workers = packets
	}
	if workers < 1 {
		workers = 1
	}

	counts := make([]int, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			rng := Philox{Seed: seed}
			rx := make([]complex128, n)
			scratch := make([]complex128, n)
			d := Demodulator{SF: sf}
			mod := Modulator{SF: sf}

			for p := w; p < packets; p += workers {
				for sym := 0; sym < symbolsPerPacket; sym++ {
					ctr := uint64(p*symbolsPerPacket + sym)
					want := int(rng.uint64At(ctr) % uint64(n))
					mod.ModulateSymbolInto(rx, want)
					rng.AddAWGN(rx, noisePower, ctr*uint64(n)+1<<40)
					if got, _ := d.DemodulateSymbolInto(scratch, rx); got != want {
						counts[w]++
						break // packet already lost; no need to finish it
					}
				}
			}
		}(w)
	}
	wg.Wait()

	bad := 0
	for _, c := range counts {
		bad += c
	}
	return float64(bad) / float64(packets)
}

// FindRequiredSNR returns the SNR at which the packet error rate crosses
// targetPER, by bisection. This is the measured counterpart to RequiredSNRdB.
//
// symbolsPerPacket defaults to a short MeshCore-sized frame when zero: an advert
// is tens of symbols, and packet length materially affects sensitivity because
// every symbol is another chance to fail.
func FindRequiredSNR(sf int, targetPER float64, packets, symbolsPerPacket int, seed uint64) float64 {
	if symbolsPerPacket <= 0 {
		symbolsPerPacket = 40
	}
	// PER is ~1 at lo and ~0 at hi for every supported SF.
	//
	// Bisection stops at a tenth of a decibel rather than running a fixed
	// twelve rounds. Twelve rounds over a 50 dB bracket resolves to 0.012 dB,
	// which is far finer than the measurement is meaningful to — the Monte
	// Carlo noise on a 400-packet estimate is larger than that — and each
	// round is a full sweep.
	lo, hi := -40.0, 10.0
	const tolerance = 0.1
	for hi-lo > tolerance {
		mid := (lo + hi) / 2
		if PacketErrorRate(sf, mid, symbolsPerPacket, packets, seed) > targetPER {
			lo = mid // still too noisy
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}
