package boardcheck

import (
	"os"
	"testing"
)

func TestReportKeysCannotSelectDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)
	t.Setenv("LocalAppData", root)
	for _, key := range []string{"", ".", "..", "../outside", `..\outside`, "/tmp/report", "a/b", "a\x00b"} {
		for _, version := range []bool{false, true} {
			board, tag := key, "repeater-v1.17.0"
			if version {
				board, tag = "Heltec_v3", key
			}
			r := untestedReport(board, tag)
			if err := r.Save(); err == nil {
				t.Errorf("Save(%q, %q) accepted a path component", board, tag)
			}
			got := Load(board, tag)
			for _, result := range got.Results {
				if result.State != Untested {
					t.Errorf("Load(%q, %q) read a report outside its entry", board, tag)
				}
			}
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid keys must be rejected before touching the cache: %v, %v", entries, err)
	}
}
