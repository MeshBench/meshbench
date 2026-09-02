package environ

import "testing"

// The sizes are copied from the published index. The parse that was here
// before returned zero for every one of them, so the 8 GB cap had never
// refused anything and a pull of any size was accepted.
func TestIndexBytes(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int64
	}{
		{"74.7KB", 74_700},
		{"8.2KB", 8_200},
		{"391.9KB", 391_900},
		{"1.1GB", 1_100_000_000},
		{"12MB", 12_000_000},
		{" 500B ", 500},
		{"1.5TB", 1_500_000_000_000},
		// A bare number is bytes, for an index that stopped writing units.
		{"1024", 1024},
		// Nothing usable is nothing claimed, rather than a guess.
		{"", 0},
		{"unknown", 0},
	} {
		if got := indexBytes(c.in); got != c.want {
			t.Errorf("indexBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// The whole point of the parse: a pull the index prices above the cap has to
// be refused, and before this it never was.
func TestIndexBytesReachesTheCap(t *testing.T) {
	var total int64
	for range 10 {
		total += indexBytes("1.1GB")
	}
	if total <= microsoftMaxBytes {
		t.Errorf("ten 1.1GB files total %d, which does not reach the %.0f cap",
			total, float64(microsoftMaxBytes))
	}
}
