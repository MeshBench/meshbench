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

	st.HandleSpec("firmware.start", state.Spec{
		What: "bring MeshCore up on every node that runs one, without starting " +
			"the clock, so a mesh can be watched settling before any traffic",
		Returns: []string{"starting"},
		Answers: "The answer says the attempt began, not that anything is up: " +
			"each node attaches on its own goroutine and the outcome arrives " +
			"later as firmware.started or firmware.failed. Poll firmware.state " +
			"to see how far it has got.",
		Example: &state.Example{
			Params: map[string]any{}, What: "bring the mesh up before pressing play",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded: %w", ErrNoSimulation)
		}
		w.Say("starting firmware on every node")
		s.startFirmware(st, w.Seed)
		return map[string]any{"starting": true}, nil
	})

	st.HandleInternalSpec("firmware.started", state.Spec{
		What: "carry the count of running firmware back into the world when an " +
			"attach finishes, and tell whoever pressed play that pressing it " +
			"again will now start the run",
		Returns: []string{"running", "playing"},
		Answers: "`playing` appears only where a play was waiting on the mesh " +
			"coming up, and is false: this reports that the mesh is up, and the " +
			"next press of play is what starts the run.",
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleInternalSpec("firmware.failed", state.Spec{
		What: "report that bringing the mesh up failed, and cancel a play that " +
			"was waiting on it rather than advancing a clock over a mesh that " +
			"is not there",
		Params: []state.Param{
			{Name: "reason", Type: state.ParamString, Primary: true,
				What: "what went wrong, said to the operator as it stands"},
		},
		Answers: "Answers with nothing: it is a report, and what it changes is " +
			"the status line and whether a waiting play is still waiting.",
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("firmware.state", state.Spec{
		What: "ask how far along bringing the mesh up is, which is what every " +
			"wait for firmware is built on",
		Returns: []string{"running", "nodes", "total", "starting"},
		Answers: "`running` counts the processes that are up and `nodes` the " +
			"nodes that run firmware at all, so a wait compares those two and " +
			"never `total`, which counts every node in the scenario including " +
			"the SDR observers and emitters that never boot one. `starting` is " +
			"how many attaches are still in flight.",
		Example: &state.Example{
			Params: map[string]any{}, What: "ask whether the mesh is up yet",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"running": s.firmwareCount(), "nodes": s.firmwareNodeCount(),
			// Every node, for a caller that wants the scenario's size rather
			// than the part of it that boots.
			"total":    len(w.Nodes),
			"starting": s.starting.Load(),
		}, nil
	})

	st.HandleSpec("nodes.stats", state.Spec{
		What: "recompute every node's counters now rather than at the next tick, " +
			"and answer with the rows themselves, which is how anything outside " +
			"the window asks whether a node is running",
		Returns: []string{"nodes", "stats"},
		Answers: "`nodes` is how many rows there are and `stats` is the rows. A " +
			"session with no engine built has no counters to report and answers " +
			"with none, which is not a failure.",
		Example: &state.Example{
			Params: map[string]any{}, What: "read every node's state and counters",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleSpec("node.stop", state.Spec{
		What: "take one node's firmware down while leaving the node in the " +
			"scenario, so it reports its final counters on the way out - which " +
			"are usually the only evidence about a node that was misbehaving",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which node stops; a name this network has not got is " +
					"refused, and so is a node that is not running firmware"},
		},
		Returns: []string{"stopped"},
		Example: &state.Example{
			Params: "West Lomond", What: "take one node off the air",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.stopNode(name); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("stopped " + name)
		return map[string]any{"stopped": name}, nil
	})

	st.HandleSpec("node.start", state.Spec{
		What: "bring a stopped node's firmware back up, which goes through the " +
			"whole-mesh attach and so starts every other stopped node with it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which node starts; a name this network has not got is " +
					"refused, and so is a node that is already running"},
		},
		Returns: []string{"started"},
		Answers: "`started` is the node that was asked for, not everything that " +
			"came up: the attach behind it skips only the nodes already running.",
		Example: &state.Example{
			Params: "West Lomond", What: "put a stopped node back on the air",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if err := s.startNode(context.Background(), name, w.Seed); err != nil {
			return nil, err
		}
		w.Stats = s.nodeStats(w.Events)
		w.Say("started " + name)
		return map[string]any{"started": name}, nil
	})

	st.HandleSpec("node.set_firmware", state.Spec{
		What: "change the build a node runs and apply it now, stopping the node, " +
			"provisioning it and starting it again, because firmware is chosen " +
			"when a node launches and nothing else would take effect",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "which node changes build; absent, blank or unknown is refused"},
			{Name: "version", Type: state.ParamString, Required: true,
				What: "the build to run, as the firmware library names it; " +
					"absent or blank is refused"},
			{Name: "board", Type: state.ParamString,
				What: "the hardware the image is for, because a board image is " +
					"that image for that board; absent means a build for this machine"},
			{Name: "role", Type: state.ParamString,
				What: "which MeshCore role the image is; absent leaves the " +
					"node's role as it was"},
		},
		Returns: []string{"node", "version", "board", "role"},
		Answers: "The answer says the change was accepted, not that it took: the " +
			"stop, provision and start run behind it, and a build that will not " +
			"provision or a node that will not start arrives afterwards as " +
			"node.reflash_failed.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "version": "v1.7.1"},
			What:   "put a host build on one node and restart it into it",
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleInternalSpec("node.reflashed", state.Spec{
		What: "report that a node's build change went through, refreshing the " +
			"counters and the node's own window, which reads the node list " +
			"rather than the stats and so showed the old build for ever",
		Params: []state.Param{
			{Name: "message", Type: state.ParamString, Primary: true,
				What: "what to say, whose first word is the node that changed"},
		},
		Answers: "Answers with nothing: what it changes is the stats, the node " +
			"list and the status line.",
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleInternalSpec("node.reflash_failed", state.Spec{
		What: "report that a build change did not go through, which reaches the " +
			"operator after the caller that asked has already been told it was " +
			"accepted",
		Params: []state.Param{
			{Name: "reason", Type: state.ParamString, Primary: true,
				What: "what went wrong, said to the operator as it stands"},
		},
		Answers: "Answers with nothing: what it changes is the stats and the " +
			"status line.",
	}, func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Stats = s.nodeStats(w.Events)
		w.Say("firmware change failed: " + msg)
		return nil, nil
	})

	st.HandleSpec("node.set_firmware_only", state.Spec{
		What: "record the build a node will run at its next start without " +
			"touching the node now, for setting a fleet up before anything is " +
			"launched",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true,
				What: "which node is set; absent, blank or unknown is refused"},
			{Name: "version", Type: state.ParamString, Required: true,
				What: "the build it will run, as the firmware library names it; " +
					"absent or blank is refused"},
			{Name: "board", Type: state.ParamString,
				What: "the hardware the image is for; absent means a build for " +
					"this machine, and clears any board the node was pinned to"},
			{Name: "role", Type: state.ParamString,
				What: "which MeshCore role the image is; absent leaves the " +
					"node's role as it was"},
		},
		Returns: []string{"node", "version", "board", "role"},
		Answers: "Nothing restarts. A node already running goes on running what " +
			"it has until something stops it, which is the difference between " +
			"this and node.set_firmware.",
		Example: &state.Example{
			Params:   map[string]any{"node": "West Lomond", "version": "v1.7.1"},
			What:     "choose what a node will run without disturbing it",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("node.provisioning", state.Spec{
		What: "read the console lines a node is told before a run, so a region " +
			"defined but never allowed to flood is visible rather than looking " +
			"like broken radio",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "whose script to read; a name this network has not got is refused"},
		},
		Returns: []string{"node", "commands"},
		Answers: "`commands` is the lines that are actually sent, with the " +
			"commentary dropped; the panel keeps the annotated form, each line " +
			"with the reason it exists.",
		Example: &state.Example{
			Params: "West Lomond", What: "see what a node will be told at start",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
