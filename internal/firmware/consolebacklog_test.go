package firmware

import (
	"bytes"
	"strings"
	"testing"
)

// A console opened after the node booted is shown the boot.
//
// The port was a single seat with nobody in it, so everything a node printed
// before somebody opened its console went past a nil writer and was dropped.
// The console was therefore empty exactly when it was opened to answer the
// question it exists for.
func TestAConsoleOpenedLateIsShownWhatItMissed(t *testing.T) {
	b := &Bridge{node: "West Lomond"}
	sink := b.ConsoleSink()
	_, _ = sink.Write([]byte("MeshCore v1.7.0\r\n"))
	_, _ = sink.Write([]byte("radio: SX1262 ok\r\n"))

	var seen bytes.Buffer
	b.Console(&seen)
	got := seen.String()
	for _, want := range []string{"MeshCore v1.7.0", "radio: SX1262 ok"} {
		if !strings.Contains(got, want) {
			t.Errorf("the console never showed %q; it showed %q", want, got)
		}
	}
	// Marked as scrollback, because every line of it is about to be stamped
	// with the clock as it arrives rather than with the clock it was said at.
	if !strings.Contains(got, "West Lomond said this before the console was opened") {
		t.Errorf("the replayed boot is not named as scrollback: %q", got)
	}

	// And what the node says afterwards goes straight through, once.
	seen.Reset()
	_, _ = sink.Write([]byte("advert sent\r\n"))
	if got := seen.String(); got != "advert sent\r\n" {
		t.Errorf("live output arrived as %q", got)
	}
}

// The backlog is handed over once. A second reader gets what has arrived since
// and not a duplicate of the boot, which would read as the node booting twice.
func TestTheBacklogIsHandedOverOnce(t *testing.T) {
	b := &Bridge{node: "West Lomond"}
	sink := b.ConsoleSink()
	_, _ = sink.Write([]byte("MeshCore v1.7.0\n"))

	var first bytes.Buffer
	b.Console(&first)
	if !strings.Contains(first.String(), "MeshCore v1.7.0") {
		t.Fatal("the first console did not get the boot")
	}
	b.Console(nil)

	var second bytes.Buffer
	b.Console(&second)
	if strings.Contains(second.String(), "MeshCore v1.7.0") {
		t.Errorf("the boot was replayed a second time: %q", second.String())
	}
}

// What a node says between one console closing and the next opening is kept.
//
// Detaching is Console(nil), and taking the scrollback on the way out would
// drop exactly the window somebody closed the console and came back to ask
// about.
func TestWhatIsSaidBetweenTwoConsolesIsKept(t *testing.T) {
	b := &Bridge{node: "West Lomond"}
	sink := b.ConsoleSink()

	var first bytes.Buffer
	b.Console(&first)
	b.Console(nil)
	_, _ = sink.Write([]byte("radio went quiet\n"))

	var second bytes.Buffer
	b.Console(&second)
	if !strings.Contains(second.String(), "radio went quiet") {
		t.Errorf("what the node said while nobody looked was lost: %q", second.String())
	}
}

// A node left running with nobody looking does not grow without bound: what is
// kept is the end of what it said, which is what its silence needs explaining
// by.
func TestTheBacklogIsBounded(t *testing.T) {
	b := &Bridge{node: "West Lomond"}
	sink := b.ConsoleSink()
	for i := 0; i < 200; i++ {
		_, _ = sink.Write(bytes.Repeat([]byte("x"), 1024))
	}
	_, _ = sink.Write([]byte("the last thing it said\n"))
	if got := len(b.backlog); got > consoleBacklog {
		t.Errorf("the backlog holds %d bytes, past the %d cap", got, consoleBacklog)
	}
	var seen bytes.Buffer
	b.Console(&seen)
	if !strings.Contains(seen.String(), "the last thing it said") {
		t.Error("overflow dropped the newest output rather than the oldest")
	}
}

// A claimed port still belongs to whoever claimed it, and opening a console
// against one takes nothing and is shown nothing.
func TestAClaimedPortKeepsItsBacklog(t *testing.T) {
	b := &Bridge{node: "West Lomond"}
	sink := b.ConsoleSink()
	_, _ = sink.Write([]byte("MeshCore v1.7.0\n"))

	var client bytes.Buffer
	release := b.Claim(&client)
	var pane bytes.Buffer
	b.Console(&pane)
	if pane.Len() != 0 {
		t.Errorf("a console took a claimed port's output: %q", pane.String())
	}
	release()
	b.Console(&pane)
	if !strings.Contains(pane.String(), "MeshCore v1.7.0") {
		t.Error("the boot was lost while the port was claimed")
	}
}
