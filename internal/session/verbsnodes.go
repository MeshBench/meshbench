// Firmware, per node and per network: starting it, reporting what came up,
// and changing what a node will run next time.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerNodeFirmwareVerbs(st *state.Store, s *Sim) {
	st.Handle("firmware.start", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network Loaded")
		}
		w.Say("starting firmware on every node")
		s.startFirmware(st, w.Seed)
		return map[string]any{"starting": true}, nil
	})

	st.Handle("firmware.started", func(w *state.World, _ any) (any, error) {
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

	st.Handle("firmware.failed", func(w *state.World, p any) (any, error) {
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

	st.Handle("firmware.state", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"running": s.firmwareCount(), "nodes": len(w.Nodes),
			"starting": s.starting.Load(),
		}, nil
	})

	st.Handle("nodes.stats", func(w *state.World, _ any) (any, error) {
		// Also on demand, because a paused simulation still costs memory and
		// somebody looking at the node view has usually just paused it.
		w.Stats = s.nodeStats(w.Events)
		return map[string]any{"nodes": len(w.Stats)}, nil
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
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		// Applied, not just recorded: stop, provision, start. Firmware is
		// chosen when a node launches, so setting it on a running node changes
		// nothing until something restarts it.
		s.Reflash(context.Background(), st, name, version, w.Seed)
		w.Say(name + ": changing to " + version)
		return map[string]any{"node": name, "version": version}, nil
	})

	st.Handle("node.reflashed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		w.Say(msg)
		return nil, nil
	})

	st.Handle("node.reflash_failed", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		w.Say("firmware change failed: " + msg)
		return nil, nil
	})

	st.Handle("node.set_firmware_only", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		version, _ := m["version"].(string)
		if err := s.setFirmware(name, version); err != nil {
			return nil, err
		}
		// Not applied until the node restarts, and said so rather than left to
		// be discovered: firmware is chosen at launch, and a version that
		// changes nothing until something else happens is the kind of control
		// somebody presses twice and then distrusts.
		w.Say(name + " will run " + version + " when it next starts")
		return map[string]any{"node": name, "version": version}, nil
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
		// The rule-resolved preview too, so opening this one menu item shows
		// both what the flat settings alone would send and, once a readback
		// has run, what the active rules actually will - without a second
		// click to find out the second one exists.
		if ns, ok := s.readback[name]; ok {
			resolved := s.previewFor(ns)
			w.ProvisioningPreviewNode, w.ProvisioningPreview = name, resolved
		}
		return map[string]any{"node": name, "commands": cmds}, nil
	})
}
