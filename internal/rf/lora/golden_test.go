package lora_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/lora"
)

// goldenVector is one externally produced frame: the payload bytes and the
// chirp-shift symbols a proven implementation (gr-lora_sdr, or a capture
// from a real SX1262 demodulated offline) says they become.
type goldenVector struct {
	SF      int    `json:"sf"`
	CR      int    `json:"cr"`
	LDRO    bool   `json:"ldro"`
	CRC     bool   `json:"crc"`
	Payload []byte `json:"payload"`
	Symbols []int  `json:"symbols"`
	// Compare bounds the transmit-side check: symbols past it sit in the
	// final interleaver block alongside the chip's padding, which is
	// uninitialised garbage no receiver reads. Zero means compare all.
	Compare int    `json:"compare_symbols"`
	Source  string `json:"source"`
}

// TestGoldenVectors holds this chain to the outside world. It skips when no
// vectors are present: producing them needs GNU Radio or captured hardware
// IQ, which is on the plan's manual list. Drop files matching
// testdata/golden-*.json to arm it.
func TestGoldenVectors(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "golden-*.json"))
	if len(files) == 0 {
		t.Skip("no golden vectors present - see docs/waveform-source-of-truth.md " +
			"(W2, golden vectors) for how to produce them with gr-lora_sdr")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v goldenVector
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		p := lora.Params{SF: v.SF, CR: v.CR, LDRO: v.LDRO, CRC: v.CRC}
		got, err := lora.Encode(p, v.Payload)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if len(got) != len(v.Symbols) {
			t.Fatalf("%s: %d symbols, the reference says %d", f, len(got), len(v.Symbols))
		}
		limit := v.Compare
		if limit == 0 || limit > len(got) {
			limit = len(got)
		}
		// Compare at each symbol's own coded rate: reduced-rate symbols
		// (the header block, and every block under LDRO) only carry their
		// top bits, and a captured bin's bottom bits are measurement jitter
		// no receiver reads. Full-rate symbols still compare exactly.
		shiftAt := func(i int) int {
			if i < 8 {
				return 2 // header block runs at SF-2 always
			}
			if v.LDRO {
				return 2
			}
			return 0
		}
		for i := 0; i < limit; i++ {
			s := shiftAt(i)
			if got[i]>>s != v.Symbols[i]>>s {
				t.Fatalf("%s: symbol %d is %d, the reference says %d - a "+
					"bit-level convention differs; see the conventions notes in "+
					"internal/lora", f, i, got[i], v.Symbols[i])
			}
		}
		// The receive half: the air's own symbols, chip padding included,
		// must decode to the payload with a valid CRC.
		dec, ok, stats := lora.Decode(p, v.Symbols)
		if !ok || !stats.CRCOK {
			t.Fatalf("%s: the reference frame does not decode: %+v", f, stats)
		}
		if string(dec) != string(v.Payload) {
			t.Fatalf("%s: decoded %q, want %q", f, dec, v.Payload)
		}
	}
}
