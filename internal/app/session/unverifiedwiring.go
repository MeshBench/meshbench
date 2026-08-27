package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Running a board nobody has watched boot yet.
//
// The gate this lifts is a curation claim rather than a safety one, and it had
// no way up. A build imported for a board could not then be run on one, so the
// only boards that could ever be verified were the ones already verified - and
// the probe that does the verifying is itself a run.
func registerUnverifiedWiring(st *state.Store, s *Sim) {
	// sim.unverified_wiring: run boards whose wiring nobody has watched.
	st.Handle("sim.unverified_wiring", func(w *state.World, p any) (any, error) {
		if on, ok := boolField(p, "on"); ok {
			s.unverifiedWiring = on
			s.prefs.UnverifiedWiring = on
			s.savePrefs()
			if on {
				w.Say(unwatchedWarning(s.nodes))
			} else {
				w.Say("only boards whose wiring has been watched boot will run")
			}
		}
		return map[string]any{"on": s.unverifiedWiring}, nil
	})
}

// unwatchedWarning says what is about to be trusted, and names the boards.
//
// Naming them is the point. "Unverified wiring is on" tells an operator
// nothing about which of their forty nodes is the one to distrust when it
// reports silence, and a board reported silent because its chip select is on
// the wrong pin looks exactly like a board out of range.
func unwatchedWarning(nodes []scenario.Node) string {
	seen := map[string]bool{}
	var boards []string
	for _, n := range nodes {
		b := n.Firmware.Board
		if b == "" || seen[b] || hw.EmulationSupported(b) {
			continue
		}
		seen[b] = true
		boards = append(boards, b)
	}
	if len(boards) == 0 {
		return "unwatched wiring will run - no node here needs it yet"
	}
	return fmt.Sprintf("unwatched wiring will run: %s. A board wired wrongly "+
		"reports as silent rather than as mis-wired, so treat a quiet node as "+
		"a question about the wiring first", strings.Join(boards, ", "))
}

// RunUnverifiedWiring lifts the gate before anything has been registered.
//
// For the command line, which sets it before the store's loop is running: a
// verb fired then waits for a goroutine that has not started yet, which is a
// workbench that opens no window at all.
func (s *Sim) RunUnverifiedWiring() {
	s.unverifiedWiring = true
}
