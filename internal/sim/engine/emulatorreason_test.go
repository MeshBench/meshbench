package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// logging is a backend that keeps an emulator log, as an emulated node does.
type logging struct{ path string }

func (logging) Start(context.Context, string) error { return nil }
func (logging) Stop() error                         { return nil }
func (logging) Kind() string                        { return "emulated" }
func (logging) HasConsole() bool                    { return true }
func (logging) ConsoleIn() io.Writer                { return nil }

func (l logging) EmulatorLog() ([]byte, error) { return os.ReadFile(l.path) }

// quiet is a backend with no emulator log at all, as the native one has none.
type quiet struct{}

func (quiet) Start(context.Context, string) error { return nil }
func (quiet) Stop() error                         { return nil }
func (quiet) Kind() string                        { return "native" }
func (quiet) HasConsole() bool                    { return true }
func (quiet) ConsoleIn() io.Writer                { return nil }

func withLog(t *testing.T, body string) *firmware.Node {
	t.Helper()
	p := filepath.Join(t.TempDir(), "emulator.log")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return &firmware.Node{Backend: logging{path: p}}
}

// The failure this exists for. An emulator that refuses a machine property
// aborts before the firmware prints anything, so the board looks from here
// exactly like one that booted and went quiet - and the two want opposite
// things done about them. Whatever the emulator last said has to travel with
// the node being dropped, or the answer stays on disk with nothing pointing
// at it.
func TestADroppedNodeCarriesWhatTheEmulatorSaid(t *testing.T) {
	const abort = "qemu-system-xtensa: Attempt to set property 'apb_freq' on " +
		"anonymous device (type 'esp_soc.uart') after it was realized"
	e := &Engine{}
	e.markFirmwareDown("Abernethy Repeater", withLog(t, "Adding SPI flash device\n"+abort+"\n"),
		"the tick could not be sent to it: bridge closed")

	failures := e.FirmwareFailures()
	if len(failures) != 1 {
		t.Fatalf("want one failure, got %d", len(failures))
	}
	if !strings.Contains(failures[0].Why, abort) {
		t.Errorf("the reason does not carry what the emulator said:\n%s", failures[0].Why)
	}
	if !strings.Contains(failures[0].Why, "bridge closed") {
		t.Errorf("the reason lost what the engine saw:\n%s", failures[0].Why)
	}
}

// A native node has no emulator and no log, and must not acquire an empty
// clause that reads as one having said nothing.
func TestANodeWithNoEmulatorLogAddsNothing(t *testing.T) {
	if got := emulatorReason(&firmware.Node{Backend: quiet{}}); got != "" {
		t.Errorf("a backend with no emulator log added %q", got)
	}
	if got := emulatorReason(nil); got != "" {
		t.Errorf("no node at all added %q", got)
	}
}

// An emulator that started cleanly writes a line or two of its own and is not
// a failure, so the clause has to be the last thing said rather than the
// first: QEMU names the assertion, the file and the line before it names the
// property, and the property is the part that says what is wrong.
func TestTheReasonIsTheLastThingSaid(t *testing.T) {
	fw := withLog(t, "Adding SPI flash device\nqemu: the thing that went wrong\n\n\n")
	got := emulatorReason(fw)
	if !strings.HasSuffix(got, "qemu: the thing that went wrong") {
		t.Errorf("want the last non-blank line, got %q", got)
	}
}

// An empty log is an emulator that said nothing, which is not a reason.
func TestAnEmptyEmulatorLogAddsNothing(t *testing.T) {
	if got := emulatorReason(withLog(t, "\n \n")); got != "" {
		t.Errorf("an empty log added %q", got)
	}
}

// -d unimp writes megabytes a second, and every one of those lines is a
// candidate for this clause. It goes into a line an operator reads, so it is
// capped.
func TestAVeryLongLastLineIsCapped(t *testing.T) {
	got := emulatorReason(withLog(t, strings.Repeat("x", 4000)+"\n"))
	if len(got) > reasonWidth+64 {
		t.Errorf("the clause is %d characters long", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a truncated clause does not say it was truncated: %q", got[len(got)-10:])
	}
}

// A node the run has given up on must stop counting as running.
//
// firmware.state's "running" is what every script waits on before it measures
// anything, and markFirmwareDown deliberately leaves the node's Firmware set
// so its bridge can still be closed. Counting the field rather than the
// verdict reported a full mesh for the whole life of a run in which a node was
// dead from the first tick.
func TestADroppedNodeStopsCountingAsRunning(t *testing.T) {
	e := &Engine{}
	e.Add(scenario.Node{Name: "alive"}, &firmware.Node{})
	e.Add(scenario.Node{Name: "dead"}, &firmware.Node{})
	if got := e.FirmwareCount(); got != 2 {
		t.Fatalf("two nodes with firmware counted as %d", got)
	}

	e.markFirmwareDown("dead", nil, "the tick could not be sent to it")
	if got := e.FirmwareCount(); got != 1 {
		t.Errorf("a dropped node still counts as running: %d of 2", got)
	}
}
