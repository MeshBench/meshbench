package lora_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/lora"
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
		got, err := lora.Encode(lora.Params{SF: v.SF, CR: v.CR, LDRO: v.LDRO, CRC: v.CRC}, v.Payload)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if len(got) != len(v.Symbols) {
			t.Fatalf("%s: %d symbols, the reference says %d", f, len(got), len(v.Symbols))
		}
		for i := range got {
			if got[i] != v.Symbols[i] {
				t.Fatalf("%s: symbol %d is %d, the reference says %d - a "+
					"bit-level convention differs; see the conventions notes in "+
					"internal/lora", f, i, got[i], v.Symbols[i])
			}
		}
	}
}
