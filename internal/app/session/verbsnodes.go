// Firmware, per node and per network: starting it, reporting what came up,
// and changing what a node will run next time.
package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerNodeFirmwareVerbs(st *state.Store, s *Sim) {
	// The board's own controls are registered from here rather than from the
	// verb hub, because they arrived with the firmware verbs and splitting the
	// file was what the length limit asked for, not a change of ownership.
	registerBoardInput(st, s)

	st.Handle("firmware.start", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded: %w", ErrNoSimulation)
		}
		w.Say("starting firmware on every node")
		s.startFirmware(st, w.Seed)
		return map[string]any{"starting": true}, nil
	})

	st.HandleInternal("firmware.started", func(w *state.World, _ any) (any, error) {
		n := s.firmwareCount()
		// The count, here as well as on the tick.
		//
		// It was only ever written while the engine was stepping, so a mesh
		// that was up but paused reported nought running - which is what the
		// status line said while fifty-six processes answered their consoles.
		w.FirmwareRunning = n
		if w.PendingPlay {
			// The mesh is up, and that is all this reports.
			//
			// It used to start the run here, which meant one press of play
			// produced two things happening a minute apart with nothing
			// asking for the second. Play is what starts a run; this says
			// when pressing it will do something.
			w.PendingPlay = false
			w.Say(fmt.Sprintf("%d nodes running firmware - press play to start the run", n))
			return map[string]any{"running": n, "playing": false}, nil
		}
		w.Say(fmt.Sprintf("%d nodes running firmware", n))
		return map[string]any{"running": n}, nil
	})

	st.HandleInternal("firmware.failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		// A run that was waiting for firmware does not start on a failure. It
		// would advance a clock over a mesh that is not there.
		if w.PendingPlay {
			w.PendingPlay = false
			w.Say("firmware failed, so the run has not started: " + msg)
			return nil, nil
		}
		w.Say("firmware: " + msg)
		return nil, nil
	})

	// "nodes" is the nodes that *run* firmware, not every node there is. It
	// used to be every node, and running is a count of processes - so on any
	// scenario holding an SDR observer or an emitter the two could never meet.
	// fife-strict holds one of each, so the shipped fixture reported 56 of 58
	// for ever and every wait built on it hung. Comparing two different
	// populations is the whole of that bug.
	st.Handle("firmware.state", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"running": s.firmwareCount(), "nodes": s.firmwareNodeCount(),
			// Every node, for a caller that wants the scenario's size rather
			// than the part of it that boots.
			"total":    len(w.Nodes),
			"starting": s.starting.Load(),
		}, nil
	})

	st.Handle("nodes.stats", func(w *state.World, _ any) (any, error) {
		// Also on demand, because a paused simulation still costs memory and
		// somebody looking at the node view has usually just paused it.
		w.Stats = s.nodeStats(w.Events)
		// And the rows, not only how many there are.
		//
		// It answered with a count and put the rows in the snapshot, where
		// only a panel could reach them - so from outside the window there
		// was no way to ask whether a node was running, which is the first
		// thing any script wants to know and the thing every wait is built
		// on. The count stays for whoever is already reading it.
		return map[string]any{"nodes": len(w.Stats), "stats": statRows(w.Stats)}, nil
	})

	st.Handle("node.stop", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.stopNode(name); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("stopped " + name)
		return map[string]any{"stopped": name}, nil
	})

	st.Handle("node.start", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.startNode(context.Background(), name, w.Seed); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("started " + name)
		return map[string]any{"started": name}, nil
	})

	st.Handle("node.set_firmware", func(w *state.World, p any) (any, error) {
		// Checked here, before the goroutine, the way its neighbour
		// set_firmware_only checks. A bad node or an empty version used to
		// return the success shape and deliver the refusal to
		// node.reflash_failed, which the caller that asked never subscribes to -
		// and the client façade spells this as `node.firmware = build`, so the
		// assignment appeared to work and the node went on running what it had.
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		// The board and the role travel with the version, because a board
		// image is not a build on its own: it is that image for that hardware.
		// Absent means a build for this machine, which is what every node ran
		// before board images could be chosen here at all.
		b := Build{Version: version}
		b.Board, _ = m["board"].(string)
		b.Role, _ = m["role"].(string)
		if err := buildAsked("node.set_firmware", name, version); err != nil {
			return nil, err
		}
		if _, found := s.nodeIndex(name); !found {
			return nil, noSuchNode(name)
		}
		// Applied, not just recorded: stop, provision, start. Firmware is
		// chosen when a node launches, so setting it on a running node changes
		// nothing until something restarts it. What that background pass can
		// still fail at - a build that will not provision, a node that will not
		// start - remains a node.reflash_failed, because by then the caller has
		// been told the change was accepted and is watching the node.
		s.Reflash(context.Background(), st, name, b, w.Seed)
		// A different build may insist on storage where the last one did not,
		// so what the node's slot will hold has just changed.
		s.publishCards(w)
		w.Say(name + ": changing to " + b.Describe())
		return map[string]any{"node": name, "version": version,
			"board": b.Board, "role": b.Role}, nil
	})

	st.HandleInternal("node.reflashed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		// The node's own window reads the node list rather than the stats, so
		// a change that stopped at the stats showed in the table and nowhere
		// else. Name is the first word of the message the reflash sent.
		if name, _, ok := strings.Cut(msg, " "); ok {
			s.refreshNodeBuild(w, name)
		}
		w.Say(msg)
		return nil, nil
	})

	st.HandleInternal("node.reflash_failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		w.Say("firmware change failed: " + msg)
		return nil, nil
	})

	st.Handle("node.set_firmware_only", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		b := Build{Version: version}
		b.Board, _ = m["board"].(string)
		b.Role, _ = m["role"].(string)
		if err := buildAsked("node.set_firmware_only", name, version); err != nil {
			return nil, err
		}
		if err := s.setFirmware(name, b); err != nil {
			return nil, err
		}
		s.refreshNodeBuild(w, name)
		// Not applied until the node restarts, and said so rather than left to
		// be discovered: firmware is chosen at launch, and a version that
		// changes nothing until something else happens is the kind of control
		// somebody presses twice and then distrusts.
		w.Say(name + " will run " + b.Describe() + " when it next starts")
		return map[string]any{"node": name, "version": version,
			"board": b.Board, "role": b.Role}, nil
	})

	st.Handle("node.provisioning", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		lines, err := s.provisioningFor(name)
		if err != nil {
			return nil, err
		}
		w.Provisioning, w.ProvisioningNode = lines, name
		var cmds []string
		for _, l := range lines {
			if !l.Comment {
				cmds = append(cmds, l.Command)
			}
		}
		return map[string]any{"node": name, "commands": cmds}, nil
	})
}

// buildAsked refuses a firmware change that names no node or no build.
//
// Shared by the two firmware verbs so they cannot drift apart again: one of
// them validated and one of them did not, in the same file, and the one that
// did not was the one the client façade spells as an assignment.
func buildAsked(verb, node, version string) error {
	if strings.TrimSpace(node) == "" {
		return badParams("%s needs a node: which one is to change build", verb)
	}
	if strings.TrimSpace(version) == "" {
		return badParams("%s needs a version: the build to run, "+
			"as firmware.list and firmware.builds name it", verb)
	}
	return nil
}
