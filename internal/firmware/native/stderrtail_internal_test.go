// Internal, because stderrTail is not exported - the bound is the whole
// point of the type, and it is easiest to pin directly rather than through a
// child process that would have to be made to write megabytes.
package native

import (
	"strings"
	"testing"
)

func TestStderrTailStaysBoundedAndKeepsTheEnd(t *testing.T) {
	tail := &stderrTail{}

	head := "HEAD-OF-STDERR"
	filler := strings.Repeat("x", stderrTailCap*3)
	end := "END-OF-STDERR"

	// Written in pieces, as a real child's output arrives, not one giant
	// write that a naive implementation could bound only by accident.
	for _, part := range splitEvery(head+filler+end, 37) {
		if _, err := tail.Write([]byte(part)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got := tail.String()
	if len(got) > stderrTailCap {
		t.Fatalf("tail held %d bytes, want at most %d", len(got), stderrTailCap)
	}
	if !strings.HasSuffix(got, end) {
		t.Fatalf("tail dropped the last bytes written; got the last %d bytes: %q", len(got), got)
	}
	if strings.Contains(got, head) {
		t.Error("tail kept the earliest bytes instead of evicting them for the latest")
	}
}

func splitEvery(s string, n int) []string {
	var parts []string
	for len(s) > n {
		parts = append(parts, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		parts = append(parts, s)
	}
	return parts
}
