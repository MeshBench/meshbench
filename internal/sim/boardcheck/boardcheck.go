// Package boardcheck turns the board list on its side: not can this board
// run, but which capabilities does it actually demonstrate, measured rather
// than asserted - and untested distinguished from failed, because a blank
// cell reads as working and it should read as unknown.
//
// Each capability is a short scripted run against the same emulation
// harness the engine's own live tests use: boot the published image under
// QEMU or Renode, drive it from a native peer on the same simulated
// channel, and read the ledger.
package boardcheck

import (
	"fmt"
	"os"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Capability is one thing worth knowing about a board.
type Capability string

const (
	Build Capability = "build" // a published image exists for this board and version
	Boot  Capability = "boot"  // the image starts and attaches to the bridge
	Radio Capability = "radio" // the radio initialises and the node transmits at all
	TX    Capability = "tx"    // an explicit send is observed on the air
	RX    Capability = "rx"    // a peer's transmission is heard and decoded
	Flood Capability = "flood" // a message is relayed onward, not just received
	FEM   Capability = "fem"   // a front-end module (PA/LNA) is asserted correctly
	Power Capability = "power" // the node keeps responding after an idle period
)

// Capabilities is every column, left to right.
var Capabilities = []Capability{Build, Boot, Radio, TX, RX, Flood, FEM, Power}

// State is three-valued on purpose: a board never run must never look the
// same as one that was run and passed.
type State string

const (
	Untested      State = "untested" // never measured
	Passed        State = "passed"
	Failed        State = "failed"
	NotApplicable State = "n/a" // measured the precondition; there is nothing here to test
)

// Result is one capability's outcome, with the evidence for it - a failure
// with no reason sends someone back to logs this exists to replace.
type Result struct {
	Capability Capability
	State      State
	Detail     string
}

// BoardReport is one board's whole row.
type BoardReport struct {
	Board      string
	Version    string
	Results    map[Capability]Result
	EmulatorFP string // what emulator build produced this, for cache invalidation
	MeasuredAt time.Time
	// Stale is set by the cache loader, not by Probe: it means the emulator
	// in use today no longer matches the one that produced this report.
	Stale bool
}

func untestedReport(board, version string) BoardReport {
	r := BoardReport{Board: board, Version: version, Results: map[Capability]Result{}}
	for _, c := range Capabilities {
		r.Results[c] = Result{Capability: c, State: Untested}
	}
	return r
}

func (r *BoardReport) set(c Capability, s State, detail string) {
	r.Results[c] = Result{Capability: c, State: s, Detail: detail}
}

// EmulatorFingerprint identifies the emulator build in use, so a cached
// report can tell whether it is still about the binary that produced it.
// The size and mtime of the QEMU or Renode binary this run resolves to -
// cheap, and it changes exactly when a rebuild would invalidate the report.
func EmulatorFingerprint() string {
	path := os.Getenv(firmware.EnvQEMU)
	if path == "" {
		path = os.Getenv("MESHBENCH_RENODE")
	}
	if path == "" {
		return "unconfigured"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "unconfigured"
	}
	return fmt.Sprintf("%s@%d-%d", path, fi.Size(), fi.ModTime().Unix())
}

// MatrixReports is the whole board list, agreeing with
// scenario.EmulatableBoards() on the can-it-run question: a board that
// cannot be emulated at all is reported boot-failed with that reason,
// without spending any emulator time finding out again. A board that can
// run gets whatever was last measured for it - Untested in every column if
// that is nothing.
func MatrixReports(version string) []BoardReport {
	ok, blocked := scenario.EmulatableBoards()
	okNames := map[string]bool{}
	for _, b := range ok {
		okNames[b.Name] = true
	}
	all := scenario.Boards()
	out := make([]BoardReport, 0, len(all))
	for _, b := range all {
		if reason, isBlocked := blocked[b.Name]; isBlocked && !okNames[b.Name] {
			r := untestedReport(b.Name, version)
			r.set(Boot, Failed, reason)
			out = append(out, r)
			continue
		}
		out = append(out, Load(b.Name, version))
	}
	return out
}
