package native

import (
	"math"
	"strconv"
	"testing"
)

// The engine strides node seeds by a 64-bit golden-ratio constant, so every
// node past the first carries a seed above 2^32. The bridge parses --seed with
// strtoul into a uint32_t, and on Windows, where a long is 32 bits, strtoul
// saturates: 57 of 58 nodes were seeded 0xFFFFFFFF and booted with one
// identity, private key included. What goes down the command line has to be
// a number that parser cannot saturate on, and it has to be the low word
// Linux and macOS were already keeping, so nothing changes where it worked.
func TestTheSeedOnTheCommandLineFitsTheFirmwaresParser(t *testing.T) {
	// The engine's stride, as a variable so the arithmetic wraps the way the
	// engine's does rather than failing as a constant expression.
	var stride uint64 = 0x9E3779B97F4A7C15
	seeds := []uint64{4417, 9001 + 1*stride, 9001 + 57*stride, math.MaxUint64}
	for _, seed := range seeds {
		want := seed & math.MaxUint32
		n := &Native{Seed: seed}
		args := n.args("127.0.0.1:1")
		var got string
		for i, a := range args {
			if a == "--seed" && i+1 < len(args) {
				got = args[i+1]
			}
		}
		v, err := strconv.ParseUint(got, 10, 32)
		if err != nil {
			t.Fatalf("seed %d went down as %q, which a 32-bit parser refuses: %v", seed, got, err)
		}
		if v != want {
			t.Errorf("seed %d went down as %d, want its low word %d", seed, v, want)
		}
	}
	// And two nodes of one run still get two seeds: the stride's low word is
	// as distinct as its high one.
	a := (&Native{Seed: 9001 + 1*stride}).args("x")[3]
	b := (&Native{Seed: 9001 + 2*stride}).args("x")[3]
	if a == b {
		t.Fatalf("two nodes went down with one seed %s", a)
	}
}
