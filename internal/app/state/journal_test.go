package state

import "testing"

// The argument rendering keeps a command legible without keeping its bulk: a
// bare string, a map as sorted key=value, and a frame or matrix as nothing.
func TestJournalArg(t *testing.T) {
	if got := journalArg("scotland-fife.json"); got != "scotland-fife.json" {
		t.Errorf("string arg = %q", got)
	}
	if got := journalArg(map[string]any{"seed": float64(42), "node": "GB7XYZ"}); got != "node=GB7XYZ seed=42" {
		t.Errorf("map arg = %q, want sorted key=value", got)
	}
	if got := journalArg(nil); got != "" {
		t.Errorf("nil arg = %q, want empty", got)
	}
	if got := journalArg([]byte{1, 2, 3}); got != "" {
		t.Errorf("a frame rendered as %q, want empty", got)
	}
	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	if got := journalArg(long); len([]rune(got)) > 120 {
		t.Errorf("a long arg was not clipped: %d runes", len([]rune(got)))
	}
}
