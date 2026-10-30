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
	"path/filepath"
	"strings"
	"time"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
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

// EmulatorFingerprint identifies what a board runs under on this machine, so
// a cached report can tell whether it is still about the binaries that
// produced it.
//
// Which emulator follows from the board, as it does when the board is run:
// QEMU for the ESP32 boards and Renode for the nRF52 ones. It used to read
// MESHBENCH_QEMU first and fall back to Renode, so on a machine set up for
// both every nRF52 row was stamped with QEMU's binary - upgrading Renode
// retired none of them and upgrading QEMU retired all of them (#551).
//
// The chip library is part of it too, because the chip decides what a board
// demonstrates as much as the emulator does, and so for Renode are our C#
// peripherals beside it. Each part is the size and mtime of the file the
// lookup a boot does would find: cheap, and it changes exactly when a rebuild
// or a fetch would invalidate the report.
func EmulatorFingerprint(board string) string {
	b, err := hw.BoardByName(board)
	if err != nil {
		return "unknown board"
	}
	tool := "qemu-system-xtensa"
	if b.Renode != nil {
		tool = "renode"
	}
	parts := []string{
		tool + "=" + fileFingerprint(emulated.FindTool(tool)),
		"chip=" + fileFingerprint(emulated.FindTool(emulated.RadioLib)),
	}
	if b.Renode != nil {
		parts = append(parts, "peripherals="+dirFingerprint(
			filepath.Join(emulated.SupportDir(), "peripherals")))
	}
	return strings.Join(parts, ";")
}

// fileFingerprint is a file's path, size and mtime, or "unconfigured" when
// the lookup found nothing.
func fileFingerprint(path string, err error) string {
	if err != nil {
		return "unconfigured"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "unconfigured"
	}
	return fmt.Sprintf("%s@%d-%d", path, fi.Size(), fi.ModTime().Unix())
}

// dirFingerprint is the total size and newest mtime of a directory's files.
func dirFingerprint(dir string) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "unconfigured"
	}
	var size, newest int64
	for _, e := range ents {
		fi, err := e.Info()
		if err != nil || fi.IsDir() {
			continue
		}
		size += fi.Size()
		newest = max(newest, fi.ModTime().Unix())
	}
	return fmt.Sprintf("%s@%d-%d", dir, size, newest)
}

// MatrixReports is the whole board list, agreeing with
// hw.EmulatableBoards() on the can-it-run question: a board that
// cannot be emulated at all is reported boot-failed with that reason,
// without spending any emulator time finding out again. A board that can
// run gets whatever was last measured for it - Untested in every column if
// that is nothing.
func MatrixReports(version string) []BoardReport {
	ok, blocked := hw.EmulatableBoards()
	okNames := map[string]bool{}
	for _, b := range ok {
		okNames[b.Name] = true
	}
	all := hw.Boards()
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
