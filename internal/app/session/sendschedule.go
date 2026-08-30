// Firing the scheduled sends.
//
// A fixture's `sends` and the schedule.add verb both record what a node should
// say and when, the schedule panel draws the list, the snapshot counts it - and
// until now nothing sent any of it. The feature was plumbed end to end except
// for the end that transmits, so a fixture asking for traffic ran in silence
// and its own "at least 10 delivered" could never pass.
//
// It is here rather than in the engine because a send is a console command:
// what a node says is the firmware's business, and the engine has no idea what
// "public hello" means.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// sendClock tracks what has already been said, so a repeating send repeats
// once per interval rather than once per tick.
//
// Keyed by position in the list: two sends can name the same node, the same
// command and the same interval - three companions all saying "hello" is an
// ordinary thing for a fixture to want - so the node and text do not identify
// one.
type sendClock struct {
	lastMs []int64
}

// fireDueSends says whatever is due at this moment of simulated time.
//
// Simulated, not wall clock. A send at 20 s means twenty seconds into the
// mesh's own run, so the same fixture produces the same traffic at the same
// moments however fast or slow the machine is - which is the whole of
// determinism here.
func (s *Sim) fireDueSends(w *state.World) {
	if len(w.Sends) == 0 || s.eng == nil {
		return
	}
	now := int64(s.eng.NowMs())
	if len(s.sendClock.lastMs) != len(w.Sends) {
		// The list changed - a fixture opened, or schedule.add called - so
		// start again rather than carry stale positions across it.
		s.sendClock.lastMs = make([]int64, len(w.Sends))
		for i := range s.sendClock.lastMs {
			s.sendClock.lastMs[i] = -1
		}
	}
	for i, snd := range w.Sends {
		at := int64(snd.AtMs)
		if now < at {
			continue
		}
		last := s.sendClock.lastMs[i]
		switch {
		case last < 0:
			// Not said yet, and its moment has come.
		case snd.EveryMs > 0 && now-last >= int64(snd.EveryMs):
			// Due again.
		default:
			continue
		}
		s.sendClock.lastMs[i] = now
		if err := s.saySend(snd); err != nil {
			// Said once, at the moment it failed, naming the node and the
			// command. A schedule that silently does nothing is what this
			// file exists to stop, and a schedule that silently fails would
			// be the same fault wearing a different hat.
			w.Say(fmt.Sprintf("scheduled send from %s failed: %v", snd.Node, err))
		}
	}
}

// saySend runs one scheduled command at its node.
//
// Which of two consoles it goes to depends on what the node is, and that is
// the trap this function exists to hide. A repeater has a text CLI and reads
// typed bytes. A companion does not: it speaks the framed companion protocol,
// so text typed at it goes nowhere, is echoed locally, and looks for all the
// world like a command that ran and did nothing. That is what a scheduled
// "public hello" at a companion did.
func (s *Sim) saySend(snd state.Send) error {
	cmd := strings.TrimSpace(snd.Command)
	if cmd == "" {
		return fmt.Errorf("no command")
	}
	n, ok := s.eng.NodeByName(snd.Node)
	if !ok {
		return fmt.Errorf("no node named %q", snd.Node)
	}
	if n.Firmware == nil || n.Firmware.Bridge == nil {
		return fmt.Errorf("its firmware is not running")
	}
	if !speaksCompanion(n.Spec().Kind) {
		return n.Firmware.Bridge.Type([]byte(cmd + "\r\n"))
	}
	return s.runCompanionLine(snd.Node, cmd)
}

// speaksCompanion reports whether a node's console is the framed protocol
// rather than a text CLI.
func speaksCompanion(k scenario.Kind) bool {
	return k == scenario.Companion || k == scenario.RoomServer
}

// runCompanionLine runs one meshcore-cli line at a companion.
//
// The same table console.cli uses, so a schedule cannot ask for something a
// person could not type - and an unknown command is an error here rather than
// a line quietly swallowed.
func (s *Sim) runCompanionLine(node, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return fmt.Errorf("no command")
	}
	head, args := fields[0], fields[1:]
	if _, ok := s.comps[node]; !ok {
		if err := s.connectCompanion(node); err != nil {
			return err
		}
	}
	for _, c := range meshcliCommands {
		if head == c.name || (c.short != "" && head == c.short) {
			_, err := c.run(s, node, args)
			return err
		}
	}
	return fmt.Errorf("no command %q for a companion", head)
}

// resetSendClock forgets what has been said, for a run starting over.
func (s *Sim) resetSendClock() { s.sendClock.lastMs = nil }
