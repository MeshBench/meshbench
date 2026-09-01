package console

import (
	"strings"
	"testing"
)

// The tail of a node's output before its next newline arrives is bounded the
// same way completed lines already are: a firmware print loop that never
// emits a newline must not grow this buffer without limit for as long as the
// node keeps running.
func TestPartialIsBoundedLikeLines(t *testing.T) {
	var c Buf

	// Twice the cap, in one write and in small ones - a single long write is
	// the case a raw progress indicator produces, and many small ones is the
	// case a UART delivering a byte at a time produces.
	if _, err := c.Write([]byte(strings.Repeat("x", 2*MaxPartial))); err != nil {
		t.Fatal(err)
	}
	if got := len(c.partial); got > MaxPartial {
		t.Fatalf("partial grew to %d bytes, want at most %d", got, MaxPartial)
	}

	c = Buf{}
	for i := 0; i < 4*MaxPartial; i++ {
		if _, err := c.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(c.partial); got > MaxPartial {
		t.Fatalf("partial fed one byte at a time grew to %d bytes, want at most %d", got, MaxPartial)
	}
}

// The overflow drops from the front: whatever arrived most recently is what a
// reader watching a node that never finishes a line actually wants to see.
func TestPartialKeepsTheMostRecentBytes(t *testing.T) {
	var c Buf
	if _, err := c.Write([]byte(strings.Repeat("a", MaxPartial))); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("tail")); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c.partial, "tail") {
		t.Errorf("the most recent bytes were dropped instead of the oldest: %q", c.partial[len(c.partial)-8:])
	}
}

// A completed line still ends up in Snapshot even after the tail that
// followed it has been trimmed for running long - the cap bounds the
// not-yet-terminated remainder, not what has already been turned into a line.
func TestALineSurvivesEvenWhenTheTailAfterItOverflows(t *testing.T) {
	var c Buf
	if _, err := c.Write([]byte("boot ok\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte(strings.Repeat("x", 2*MaxPartial))); err != nil {
		t.Fatal(err)
	}
	snap := c.Snapshot()
	if len(snap) == 0 || !strings.Contains(snap[0], "boot ok") {
		t.Fatalf("the completed line did not survive: %v", snap)
	}
}
