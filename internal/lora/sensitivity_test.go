package lora_test

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/lora"
)

// packetPER runs whole packets through the full chain - encode, modulate,
// AWGN, demodulate, decode - and reports the packet error rate.
func packetPER(t *testing.T, p lora.Params, snrDB float64, trials int) float64 {
	t.Helper()
	payload := []byte("sensitivity is a packet-level property")
	shifts, err := lora.Encode(p, payload)
	if err != nil {
		t.Fatal(err)
	}
	m := dsp.Modulator{SF: p.SF}
	d := dsp.Demodulator{SF: p.SF}
	n := dsp.SamplesPerSymbol(p.SF)
	sigPower := dsp.SignalPower(m.ModulateSymbol(0))
	noise := dsp.NoisePowerForSNR(sigPower, snrDB)
	rng := dsp.Philox{Seed: 20260818}

	failures := 0
	for trial := 0; trial < trials; trial++ {
		rx := m.Modulate(shifts)
		rng.AddAWGN(rx, noise, uint64(trial)<<32)
		got := make([]int, len(shifts))
		scratch := make([]complex128, n)
		for s := range shifts {
			got[s], _ = d.DemodulateSymbolInto(scratch, rx[s*n:(s+1)*n])
		}
		if _, ok, _ := lora.Decode(p, got); !ok {
			failures++
		}
	}
	return float64(failures) / float64(trials)
}

// With the coding chain in place, packet sensitivity has to sit in the
// neighbourhood of Semtech's published packet-level floors: comfortably
// decoding above the floor, comfortably failing well below it. The exact
// crossover depends on receiver details W3 and W5 add (sync loss,
// implementation loss), so the assertion brackets rather than pins.
func TestPacketSensitivityBracketsTheSemtechFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("Monte Carlo")
	}
	for _, sf := range []int{7, 9, 11} {
		p := lora.Params{SF: sf, CR: 1, CRC: true, LDRO: sf >= 11}
		floor := dsp.RequiredSNRdB[sf]
		if per := packetPER(t, p, floor+3, 30); per > 0.2 {
			t.Errorf("SF%d: PER %.2f at floor+3 dB - the chain is deafer than the chip", sf, per)
		}
		if per := packetPER(t, p, floor-6, 30); per < 0.8 {
			t.Errorf("SF%d: PER %.2f at floor-6 dB - the chain decodes below physics", sf, per)
		}
	}
}
