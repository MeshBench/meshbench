package dsp

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
	m, d := Modulator{SF: sf}, Demodulator{SF: sf}
	n := SamplesPerSymbol(sf)
	rng := Philox{Seed: seed}
	sigPower := SignalPower(m.ModulateSymbol(0))
	noisePower := NoisePowerForSNR(sigPower, snrDB)

	bad := 0
	for p := 0; p < packets; p++ {
		for sym := 0; sym < symbolsPerPacket; sym++ {
			ctr := uint64(p*symbolsPerPacket + sym)
			want := int(rng.uint64At(ctr) % uint64(n))
			rx := m.ModulateSymbol(want)
			rng.AddAWGN(rx, noisePower, ctr*uint64(n)+1<<40)
			if got, _ := d.DemodulateSymbol(rx); got != want {
				bad++
				break // packet already lost; no need to finish it
			}
		}
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
	lo, hi := -40.0, 10.0 // PER is ~1 at lo and ~0 at hi for every supported SF
	for i := 0; i < 12; i++ {
		mid := (lo + hi) / 2
		if PacketErrorRate(sf, mid, symbolsPerPacket, packets, seed) > targetPER {
			lo = mid // still too noisy
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}
