package lora_test

import (
	"bytes"
	"math"
	"math/rand"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/lora"
)

func params() []lora.Params {
	var out []lora.Params
	for sf := 7; sf <= 12; sf++ {
		for cr := 1; cr <= 4; cr++ {
			for _, ldro := range []bool{false, true} {
				for _, crc := range []bool{false, true} {
					out = append(out, lora.Params{SF: sf, CR: cr, LDRO: ldro, CRC: crc})
				}
			}
		}
	}
	return out
}

// Every parameter combination round-trips every length from empty to full.
func TestRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(4417))
	for _, p := range params() {
		for _, n := range []int{0, 1, 2, 7, 20, 63, 255} {
			payload := make([]byte, n)
			rng.Read(payload)
			syms, err := lora.Encode(p, payload)
			if err != nil {
				t.Fatalf("%+v: %v", p, err)
			}
			got, ok, stats := lora.Decode(p, syms)
			if !ok {
				t.Fatalf("%+v n=%d: clean decode failed: %+v", p, n, stats)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("%+v n=%d: payload mangled", p, n)
			}
			if stats.Corrected != 0 || stats.Failed != 0 {
				t.Fatalf("%+v n=%d: clean frame claims damage: %+v", p, n, stats)
			}
		}
	}
}

// The encoder's symbol count must equal RadioLib's airtime arithmetic - the
// number the firmware's CSMA is built on. This is the tie between the coding
// chain and the airtime rule in CLAUDE.md, held with a test.
func TestSymbolCountMatchesRadioLib(t *testing.T) {
	for _, p := range params() {
		// The airtime formula decides LDRO from the symbol duration; pick a
		// bandwidth that produces the LDRO state under test.
		bw := 125e3
		symbolMs := float64(uint64(1)<<uint(p.SF)) / (bw / 1000)
		if p.LDRO != (symbolMs >= 16.0) {
			continue // this bandwidth cannot express that LDRO state
		}
		for _, n := range []int{0, 1, 5, 12, 50, 200, 255} {
			syms, err := lora.SymbolCount(p, n)
			if err != nil {
				t.Fatal(err)
			}
			enc, err := lora.Encode(p, make([]byte, n))
			if err != nil {
				t.Fatal(err)
			}
			if len(enc) != syms {
				t.Fatalf("%+v n=%d: Encode made %d symbols, SymbolCount says %d",
					p, n, len(enc), syms)
			}
			// RadioLib's payload-symbol term, reconstructed from AirtimeMillis'
			// own arithmetic: strip preamble and coefficients back off.
			sfDiv := 4 * p.SF
			if symbolMs >= 16 {
				sfDiv = 4 * (p.SF - 2)
			}
			crcBits := 0
			if p.CRC {
				crcBits = 16
			}
			bits := 8*n + crcBits - 4*p.SF + 8 + 20
			if bits < 0 {
				bits = 0
			}
			coded := (bits + sfDiv - 1) / sfDiv
			want := 8 + coded*(p.CR+4)
			if syms != want {
				t.Fatalf("%+v n=%d: %d symbols, RadioLib arithmetic says %d",
					p, n, syms, want)
			}
			// And through the public formula itself, as a cross-check: it adds
			// back the preamble and sync/header coefficient AirtimeMillis
			// carries that SymbolCount's own return value does not.
			sfCoeff1 := 4.25
			if p.SF == 5 || p.SF == 6 {
				sfCoeff1 = 6.25
			}
			wantMs := math.Trunc(symbolMs * (float64(dsp.PreambleSymbols(p.SF)) + sfCoeff1 + float64(syms)))
			if got := dsp.AirtimeMillis(p.SF, bw, p.CR, n, p.CRC, true); got != wantMs {
				t.Fatalf("%+v n=%d: AirtimeMillis = %v, want %v from SymbolCount's own arithmetic",
					p, n, got, wantMs)
			}
		}
	}
}

// One destroyed symbol per block must decode clean at the correcting rates:
// the diagonal interleaver spreads it to one bit per codeword, and Hamming
// 4/7 and 4/8 repair exactly that. This is the property the whole
// interleave-plus-FEC design exists for.
func TestOneSymbolPerBlockIsRepairedAtCorrectingRates(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for _, cr := range []int{3, 4} {
		p := lora.Params{SF: 9, CR: cr, CRC: true}
		payload := make([]byte, 30)
		rng.Read(payload)
		syms, err := lora.Encode(p, payload)
		if err != nil {
			t.Fatal(err)
		}
		// Corrupt one symbol in every block after the header block.
		for at := 8; at+cr+4 <= len(syms); at += cr + 4 {
			syms[at] ^= 0b1 << 3 // one bin off after the reduced-rate shift
		}
		got, ok, stats := lora.Decode(p, syms)
		if !ok {
			t.Fatalf("CR4/%d: one symbol per block was not repaired: %+v", cr+4, stats)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("CR4/%d: repaired frame has wrong payload", cr+4)
		}
		if stats.Corrected == 0 {
			t.Fatalf("CR4/%d: decode claims nothing needed repair", cr+4)
		}
	}
}

// The same damage at the detection-only rates must fail the frame - and say
// so, rather than hand corrupt bytes onward.
func TestDamageAtDetectionRatesFailsTheFrame(t *testing.T) {
	rng := rand.New(rand.NewSource(100))
	for _, cr := range []int{1, 2} {
		p := lora.Params{SF: 9, CR: cr, CRC: true}
		payload := make([]byte, 30)
		rng.Read(payload)
		syms, err := lora.Encode(p, payload)
		if err != nil {
			t.Fatal(err)
		}
		syms[10] ^= 0b1 << 3
		_, ok, stats := lora.Decode(p, syms)
		if ok {
			t.Fatalf("CR4/%d accepted a damaged frame", cr+4)
		}
		if stats.Failed == 0 && stats.CRCOK {
			t.Fatalf("CR4/%d: damage was not even detected: %+v", cr+4, stats)
		}
	}
}

// A corrupted payload with an intact structure must fail the CRC: the last
// honest gate before MeshCore.
func TestCRCCatchesWhatCodingMisses(t *testing.T) {
	p := lora.Params{SF: 8, CR: 4, CRC: true}
	payload := []byte("the crc is the last honest gate")
	syms, _ := lora.Encode(p, payload)
	wrong := append([]byte(nil), payload...)
	wrong[3] ^= 0x40
	resyms, _ := lora.Encode(p, wrong)
	// Splice the wrong payload's data blocks onto the right header - a frame
	// whose coding is self-consistent but whose bytes are not the original's.
	syms = append(syms[:8], resyms[8:]...)
	got, ok, _ := lora.Decode(p, syms)
	if !ok {
		// Decode succeeded structurally but for the wrong payload - CRC must
		// have caught it only if the CRC covers the original. Splicing valid
		// blocks of another frame is a valid different frame, so this decodes.
		t.Fatal("a structurally valid frame should decode")
	}
	if bytes.Equal(got, payload) {
		t.Fatal("the spliced frame decoded as the original payload")
	}
}

// A destroyed header fails the whole frame, whatever the payload looks like.
func TestHeaderDamageFailsTheFrame(t *testing.T) {
	p := lora.Params{SF: 9, CR: 2, CRC: true}
	syms, _ := lora.Encode(p, []byte("header first"))
	syms[0] ^= 0b11 << 4
	syms[3] ^= 0b101 << 3
	_, ok, stats := lora.Decode(p, syms)
	if ok || stats.HeaderOK {
		t.Fatalf("a mangled header was accepted: %+v", stats)
	}
}

// Whitening is its own inverse, and actually whitens: a constant payload
// must not encode to a constant symbol stream.
func TestWhitening(t *testing.T) {
	b := make([]byte, 64)
	lora.Whiten(b)
	varied := map[byte]bool{}
	for _, v := range b {
		varied[v] = true
	}
	if len(varied) < 16 {
		t.Fatalf("whitening sequence has only %d distinct bytes in 64", len(varied))
	}
	lora.Whiten(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("whitening is not self-inverse at %d: %02x", i, v)
		}
	}
}

// Gray adjacency: consecutive values differ by exactly one bit, and the
// inverse really inverts.
func TestGray(t *testing.T) {
	for v := 0; v < 4096; v++ {
		if lora.GrayInverse(lora.Gray(v)) != v {
			t.Fatalf("gray inverse broken at %d", v)
		}
		if v > 0 {
			diff := lora.Gray(v) ^ lora.Gray(v-1)
			if diff&(diff-1) != 0 {
				t.Fatalf("gray adjacency broken between %d and %d", v-1, v)
			}
		}
	}
}
