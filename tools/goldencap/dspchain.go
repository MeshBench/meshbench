// From a wideband capture to chip-rate baseband, and the analysis on it.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/lora"
)

// u8ToComplex converts rtl_tcp's unsigned 8-bit interleaved IQ.
func u8ToComplex(raw []byte) []complex128 {
	out := make([]complex128, len(raw)/2)
	for i := range out {
		out[i] = complex(
			(float64(raw[i*2])-127.5)/127.5,
			(float64(raw[i*2+1])-127.5)/127.5)
	}
	return out
}

// findBurstOffsetHz locates the strongest narrowband burst near the centre:
// the dongle's crystal is off by an unknown ppm, and at 869 MHz twenty ppm
// is seventeen kilohertz - a quarter of the channel. The whole capture is
// FFT'd in coarse blocks and the peak bin, averaged over the loudest blocks,
// is the real offset.
func findBurstOffsetHz(iq []complex128, rateHz, centreHz, spanHz float64) float64 {
	const fftN = 8192
	if len(iq) < fftN {
		return 0
	}
	binHz := rateHz / fftN
	lo := int(math.Round((centreHz - spanHz) / binHz))
	hi := int(math.Round((centreHz + spanHz) / binHz))

	// Per-block maximum, not the average: the burst occupies a fraction of
	// the capture and an averaged spectrum belongs to whatever is always
	// there - the dongle's DC spike, mostly, which is why the search is also
	// centred away from zero.
	power := make([]float64, fftN)
	buf := make([]complex128, fftN)
	for at := 0; at+fftN <= len(iq); at += fftN * 2 {
		copy(buf, iq[at:at+fftN])
		dsp.FFT(buf)
		for i, v := range buf {
			p := real(v)*real(v) + imag(v)*imag(v)
			if p > power[i] {
				power[i] = p
			}
		}
	}
	best, bestBin := -1.0, int(math.Round(centreHz/binHz))
	for b := lo; b <= hi; b++ {
		idx := (b%fftN + fftN) % fftN
		if power[idx] > best {
			best, bestBin = power[idx], b
		}
	}
	// A chirp's spectrum is a plateau, not a peak: the argmax lands anywhere
	// in the occupied band, and centring on it clips the signal at the
	// channel filter. Walk out from the peak to the plateau's edges and take
	// the midpoint instead.
	loEdge, hiEdge := bestBin, bestBin
	floor := best / 20 // -13 dB off the plateau
	for b := bestBin; b >= lo; b-- {
		if power[(b%fftN+fftN)%fftN] < floor {
			break
		}
		loEdge = b
	}
	for b := bestBin; b <= hi; b++ {
		if power[(b%fftN+fftN)%fftN] < floor {
			break
		}
		hiEdge = b
	}
	return float64(loEdge+hiEdge) / 2 * binHz
}

// channelize mixes iq down by offsetHz and decimates to the channel rate
// with a simple windowed-sinc low-pass - one sample per chip, the rate every
// piece of internal/dsp speaks. phase picks which of the dec input samples
// each output lands on: a chip-rate stream half a sample off the symbol grid
// reads every bin as a smear between neighbours, and the phase is the knob
// that fixes it.
func channelize(iq []complex128, rateHz, offsetHz, chanHz float64, phase int) []complex128 {
	mixed := make([]complex128, len(iq))
	step := -2 * math.Pi * offsetHz / rateHz
	for i := range iq {
		s, c := math.Sincos(step * float64(i))
		mixed[i] = iq[i] * complex(c, s)
	}
	dec := int(math.Round(rateHz / chanHz))
	taps := firLowpass(chanHz/2/rateHz, 64*dec)
	out := make([]complex128, 0, len(mixed)/dec)
	for at := phase; at+len(taps) < len(mixed); at += dec {
		var acc complex128
		for j, t := range taps {
			acc += mixed[at+j] * complex(t, 0)
		}
		out = append(out, acc)
	}
	return out
}

// bestPhase channelizes at every decimation phase and keeps the one whose
// frame decodes best - sub-chip timing recovery by exhaustive search, scored
// by the receive chain itself, affordable because captures are short.
func bestPhase(iq []complex128, rateHz, offsetHz, chanHz float64, p lora.Params) ([]complex128, int) {
	dec := int(math.Round(rateHz / chanHz))
	layout := frameLayout(p)
	bestScore, bestAt, chosen := math.Inf(-1), 0, []complex128(nil)
	n := dsp.SamplesPerSymbol(p.SF)
	d := dsp.Demodulator{SF: p.SF}
	scratch := make([]complex128, n)
	for ph := 0; ph < dec; ph++ {
		bb := channelize(iq, rateHz, offsetHz, chanHz, ph)
		sync, ok := dsp.Detect(bb, layout)
		if !ok {
			continue
		}
		work := append([]complex128(nil), bb...)
		if sync.CFOBins != 0 {
			dsp.CorrectCFO(work, p.SF, sync.CFOBins)
		}
		var shifts []int
		for at := sync.DataStart; at+n <= len(work) && len(shifts) < 400; at += n {
			got, _ := d.DemodulateSymbolInto(scratch, work[at:at+n])
			shifts = append(shifts, got)
		}
		_, okDec, stats := lora.Decode(p, shifts)
		score := -float64(stats.Failed)*10 - float64(stats.Corrected)
		if okDec && stats.CRCOK {
			score += 1000
		}
		if stats.HeaderOK {
			score += 100
		}
		if score > bestScore {
			bestScore, bestAt, chosen = score, ph, bb
		}
	}
	return chosen, bestAt
}

func firLowpass(cutoffNorm float64, n int) []float64 {
	taps := make([]float64, n)
	sum := 0.0
	for i := range taps {
		x := float64(i) - float64(n-1)/2
		var v float64
		if x == 0 {
			v = 2 * math.Pi * cutoffNorm
		} else {
			v = math.Sin(2*math.Pi*cutoffNorm*x) / x
		}
		// Hamming window.
		v *= 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		taps[i] = v
		sum += v
	}
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}

// analyze runs the receiver over chip-rate baseband and diffs what the air
// carried against what internal/lora says the payload becomes.
func analyze(baseband []complex128, p lora.Params, payload []byte) error {
	layout := frameLayout(p)
	sync, ok := dsp.Detect(baseband, layout)
	if !ok {
		return fmt.Errorf("no preamble lock in the capture - wrong frequency, " +
			"wrong SF, or the sync conventions differ more than Detect tolerates")
	}
	fmt.Printf("locked: data at sample %d, CFO %d bins, confidence %.1f\n",
		sync.DataStart, sync.CFOBins, sync.PreambleConfidence)
	if sync.CFOBins != 0 {
		dsp.CorrectCFO(baseband, p.SF, sync.CFOBins)
	}

	n := dsp.SamplesPerSymbol(p.SF)
	d := dsp.Demodulator{SF: p.SF}
	scratch := make([]complex128, n)
	var shifts []int
	for at := sync.DataStart; at+n <= len(baseband) && len(shifts) < 400; at += n {
		got, _ := d.DemodulateSymbolInto(scratch, baseband[at:at+n])
		shifts = append(shifts, got)
	}

	want, err := lora.Encode(p, payload)
	if err != nil {
		return err
	}
	fmt.Printf("captured %d data symbols; internal/lora expects %d\n", len(shifts), len(want))
	diffs := 0
	for i := range want {
		if i >= len(shifts) {
			break
		}
		if shifts[i] != want[i] {
			if diffs < 16 {
				fmt.Printf("  symbol %3d: air=%4d  ours=%4d  (xor %#x)\n",
					i, shifts[i], want[i], shifts[i]^want[i])
			}
			diffs++
		}
	}
	// The final block can mix real nibbles with the chip's padding, which is
	// uninitialised garbage no receiver reads - captured once, it decoded to
	// nothing deterministic. TX comparison stops at the last block made
	// entirely of meaningful nibbles; RX decoding of the whole air frame is
	// the other half of the check.
	meaningful := comparableSymbols(p, len(payload))
	if goldenOut != "" {
		// Only a clean frame may become a reference: a vector carrying
		// channel damage would teach the test the wrong bits.
		if _, okDec, stats := lora.Decode(p, shifts[:len(want)]); !okDec || stats.Corrected > 0 {
			fmt.Printf("golden vector NOT written: the capture is damaged (%+v) - recapture\n", stats)
			goldenOut = ""
		}
	}
	if goldenOut != "" {
		if err := writeGolden(goldenOut, p, payload, shifts[:len(want)], meaningful); err != nil {
			return err
		}
		fmt.Printf("golden vector written to %s (compare %d of %d symbols)\n",
			goldenOut, meaningful, len(want))
	}
	if diffs == 0 && len(shifts) >= len(want) {
		fmt.Println("EXACT MATCH: every data symbol on the air equals internal/lora's encoding")
		return nil
	}
	tail := 0
	for i := meaningful; i < len(want) && i < len(shifts); i++ {
		if shifts[i] != want[i] {
			tail++
		}
	}
	if diffs == tail {
		fmt.Printf("MATCH within meaning: all %d symbols before the padding block agree; "+
			"the %d differing tail symbols carry only chip padding\n", meaningful, tail)
	}
	fmt.Printf("%d of %d symbols differ - a convention gap between internal/lora and silicon\n",
		diffs, len(want))
	solveParity(shifts[:len(want)], p)

	// Whatever the symbols say, the decoder's own verdict is worth having.
	dec, ok2, stats := lora.Decode(p, shifts)
	fmt.Printf("decode: ok=%v stats=%+v payload=%q\n", ok2, stats, dec)
	if !stats.CRCOK {
		solveCRC(shifts[:len(want)], p, payload)
	}
	return nil
}

func frameLayout(p lora.Params) dsp.FrameLayout {
	a, b := dsp.StandardSync(p.SF)
	return dsp.FrameLayout{SF: p.SF, Preamble: dsp.PreambleSymbols(p.SF), SyncA: a, SyncB: b}
}

// dumpStructure walks the baseband on a fixed symbol grid and prints, for
// each window, the upchirp-dechirped peak bin and confidence plus the
// downchirp concentration - the raw material for reading a real frame's
// structure off the air instead of assuming it.
func dumpStructure(baseband []complex128, sf int, from, count int) {
	n := dsp.SamplesPerSymbol(sf)
	d := dsp.Demodulator{SF: sf}
	m := dsp.Modulator{SF: sf}
	up := m.BaseUpchirp()
	scratch := make([]complex128, n)
	fmt.Println("  win |  upBin  conf |  downBin  down/up power")
	for w := 0; w < count; w++ {
		at := from + w*n
		if at+n > len(baseband) {
			return
		}
		bin, conf := d.DemodulateSymbolInto(scratch, baseband[at:at+n])
		var upPeak float64
		for _, v := range scratch {
			p := real(v)*real(v) + imag(v)*imag(v)
			if p > upPeak {
				upPeak = p
			}
		}
		// Downchirp view: multiply by the (unconjugated) upchirp.
		for i := 0; i < n; i++ {
			scratch[i] = baseband[at+i] * up[i]
		}
		dsp.FFT(scratch)
		var downPeak float64
		downBin := 0
		for i, v := range scratch {
			p := real(v)*real(v) + imag(v)*imag(v)
			if p > downPeak {
				downPeak, downBin = p, i
			}
		}
		ratio := 0.0
		if upPeak > 0 {
			ratio = downPeak / upPeak
		}
		fmt.Printf("  %3d | %6d  %4.1f | %7d  %8.2f\n", w, bin, conf, downBin, ratio)
	}
}

// solveParity reconstructs the chip's Hamming parity equations from the
// air: for every codeword the data nibble is known-good (the data columns
// match internal/lora exactly), so each parity bit is an unknown linear
// function of the nibble, solvable by checking all sixteen masks with and
// without inversion against every observation.
func solveParity(airShifts []int, p lora.Params) {
	blocks := lora.RawCodewords(p, airShifts)
	type obs struct{ d, parity int }
	var all []obs
	for _, cws := range blocks {
		for _, cw := range cws {
			all = append(all, obs{d: cw & 0xF, parity: cw >> 4})
		}
	}
	fmt.Printf("parity solver: %d codewords observed\n", len(all))
	for bit := 0; bit < 4; bit++ {
		found := false
		for mask := 0; mask < 16 && !found; mask++ {
			for inv := 0; inv < 2 && !found; inv++ {
				ok := true
				for _, o := range all {
					want := parityOf(o.d&mask) ^ inv
					if o.parity>>bit&1 != want {
						ok = false
						break
					}
				}
				if ok {
					names := []string{}
					for b := 0; b < 4; b++ {
						if mask>>b&1 == 1 {
							names = append(names, fmt.Sprintf("d%d", b))
						}
					}
					invs := ""
					if inv == 1 {
						invs = " ^ 1"
					}
					fmt.Printf("  parity bit %d (cw bit %d) = XOR(%v)%s\n", bit, bit+4, names, invs)
					found = true
				}
			}
		}
		if !found {
			fmt.Printf("  parity bit %d: NOT a linear function of the nibble - deeper convention gap\n", bit)
		}
	}
}

func parityOf(v int) int {
	n := 0
	for ; v != 0; v &= v - 1 {
		n++
	}
	return n & 1
}

// solveCRC reads the frame's own CRC off the air and tries the known
// convention family against it - initial values, final XORs, and the
// last-two-bytes quirk the reverse-engineering literature reports.
func solveCRC(airShifts []int, p lora.Params, payload []byte) {
	blocks := lora.RawCodewords(p, airShifts)
	var nibbles []byte
	for i, cws := range blocks {
		start := 0
		if i == 0 {
			start = 5 // past the header nibbles
		}
		for j := start; j < len(cws); j++ {
			nibbles = append(nibbles, byte(cws[j]&0xF))
		}
	}
	at := len(payload) * 2
	if len(nibbles) < at+4 {
		fmt.Println("crc solver: not enough nibbles")
		return
	}
	got := uint16(nibbles[at]) | uint16(nibbles[at+1])<<4 |
		uint16(nibbles[at+2])<<8 | uint16(nibbles[at+3])<<12
	fmt.Printf("crc solver: air carries %#04x\n", got)

	ccitt := func(data []byte, init uint16) uint16 {
		crc := init
		for _, v := range data {
			crc ^= uint16(v) << 8
			for i := 0; i < 8; i++ {
				if crc&0x8000 != 0 {
					crc = crc<<1 ^ 0x1021
				} else {
					crc <<= 1
				}
			}
		}
		return crc
	}
	n := len(payload)
	try := func(name string, v uint16) {
		if v == got {
			fmt.Printf("  MATCH: %s\n", name)
		}
	}
	for _, init := range []uint16{0x0000, 0xFFFF, 0x1D0F} {
		full := ccitt(payload, init)
		try(fmt.Sprintf("ccitt init=%#04x", init), full)
		try(fmt.Sprintf("ccitt init=%#04x xorout=0xFFFF", init), full^0xFFFF)
		if n >= 2 {
			trunc := ccitt(payload[:n-2], init)
			try(fmt.Sprintf("ccitt init=%#04x over first n-2, xor last two bytes", init),
				trunc^uint16(payload[n-1])^uint16(payload[n-2])<<8)
			try(fmt.Sprintf("ccitt init=%#04x over first n-2, xor last two swapped", init),
				trunc^uint16(payload[n-2])^uint16(payload[n-1])<<8)
			try(fmt.Sprintf("ccitt init=%#04x over first n-2", init), trunc)
		}
	}
	// Byte-swapped placements of the plain answers.
	full := ccitt(payload, 0x0000)
	try("ccitt init=0 byte-swapped", full>>8|full<<8)
}

// comparableSymbols is how many leading symbols carry only meaningful
// nibbles - header, payload, CRC - with no padding mixed in by the
// interleaver's final block.
func comparableSymbols(p lora.Params, payloadLen int) int {
	meaningNibbles := 5 + payloadLen*2
	if p.CRC {
		meaningNibbles += 4
	}
	hn := p.SF - 2
	dn := p.SF
	if p.LDRO {
		dn = p.SF - 2
	}
	if meaningNibbles <= hn {
		return 8
	}
	fullBlocks := (meaningNibbles - hn) / dn // blocks with no pad at all
	return 8 + fullBlocks*(p.CR+4)
}

type goldenFile struct {
	SF      int    `json:"sf"`
	CR      int    `json:"cr"`
	LDRO    bool   `json:"ldro"`
	CRC     bool   `json:"crc"`
	Payload []byte `json:"payload"`
	Symbols []int  `json:"symbols"`
	Compare int    `json:"compare_symbols"`
	Source  string `json:"source"`
}

func writeGolden(path string, p lora.Params, payload []byte, air []int, compare int) error {
	b, err := json.MarshalIndent(goldenFile{
		SF: p.SF, CR: p.CR, LDRO: p.LDRO, CRC: p.CRC,
		Payload: payload, Symbols: air, Compare: compare,
		Source: "SX1262 (MeshCore KISS modem) over rtl_tcp, 2026-08-18",
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
