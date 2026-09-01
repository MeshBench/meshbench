// Starting MeshCore on a whole network, and tearing it down again.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// startFirmware brings real MeshCore up on every node that runs it.
//
// This is the thing the Gio workbench was missing entirely: it built an engine
// and never attached firmware, so nothing relayed and a packet had to be
// injected to make anything happen at all. A simulator that does not run the
// firmware is a channel model with a map on it.
func (s *Sim) startFirmware(st *state.Store, seed uint64) {
	if s.eng == nil || s.starting.Swap(true) {
		return
	}
	// The engine and its nodes as they are now, not as they will be when the
	// worker gets to them.
	//
	// This runs on the store's goroutine and the attach does not, so reading
	// s.eng from in there means attaching to whichever network is live by
	// then. Opening a different one while firmware is starting therefore
	// published a node into an engine the clock was already stepping, and the
	// tick waited on a node it had never sent a tick to. The attach belongs to
	// the network that was asked for; if that one has gone, so has the reason
	// to attach to anything.
	eng, nodes := s.eng, s.nodes
	go func() {
		defer s.starting.Store(false)
		ctx := context.Background()
		// Cleared however this ends. Nothing took the job away, so the strip
		// kept "telling every node what it is - 100%" for the rest of the
		// session: a finished job that stays on screen is one somebody waits
		// on, and it hid every job that came after it.
		defer func() { _, _ = st.Do(ctx, "job.done", "firmware") }()
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "firmware", What: "starting firmware on every node"})
		err := eng.AttachNativeProgress(ctx, seed, func(done, total int) {
			// Every tenth node: a verb per node would make the queue the slow
			// part of starting 154 processes.
			if done%10 != 0 && done != total {
				return
			}
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "firmware", What: "starting firmware on every node",
				Done: done, Total: total})
		})
		if err != nil {
			_, _ = st.Do(ctx, "firmware.failed", err.Error())
			return
		}
		if n := provisionAll(eng, nodes, s.provisionLines); n > 0 {
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "firmware", What: "telling every node what it is",
				Done: n, Total: n})
			// Time has to move for the firmware to read what was queued at
			// its serial input; the old workbench steps sixty here too.
			_, _ = st.Do(ctx, "sim.settle", nil)
		}
		_, _ = st.Do(ctx, "firmware.started", nil)
	}()
}

// sayFirmwareFailures reports the nodes this tick gave up on, by name and
// with the reason each one gave.
//
// One line per node, because the two ways a node stops answering want
// different things done about them: a process that has gone is restarted, and
// a node that missed its deadline is a machine with too much on it. A run
// where the whole mesh goes at once - the machine swapping, or every process
// killed together - would otherwise put three hundred lines in the strip, so
// the tail is counted rather than listed.
func sayFirmwareFailures(w *state.World, down []engine.FirmwareFailure) {
	const named = 5
	for i, f := range down {
		if i == named {
			w.Say(fmt.Sprintf("and %d more node(s) stopped answering in the same tick",
				len(down)-named))
			return
		}
		w.Say(fmt.Sprintf(
			"%s stopped answering and has been dropped from this run: %s; "+
				"the rest of the mesh keeps going", f.Name, f.Why))
	}
}

// firmwareCount is how many nodes are running firmware right now.
func (s *Sim) firmwareCount() int {
	if s.eng == nil {
		return 0
	}
	return s.eng.FirmwareCount()
}

// firmwareNodeCount is how many of this scenario's nodes run firmware at all -
// not every node, since an SDR observer or an emitter never boots one. Shared
// by firmware.state's own count and anything waiting for a start to finish, so
// the two cannot drift into counting different populations again: comparing
// "running" against every node on a scenario holding an observer never met,
// which is what "56 of 58" forever was.
func (s *Sim) firmwareNodeCount() int {
	n := 0
	for _, node := range s.nodes {
		if node.Kind.RunsFirmware() {
			n++
		}
	}
	return n
}

// Close shuts the simulation down, firmware included.
//
// Safe on a Sim that never built an engine, because the common shutdown path
// is a workbench closed before anything was loaded. Called from the
// workbench's own shutdown goroutine rather than the store's, so the served
// listeners are taken out under their lock and closed only once it is
// released - a verb still running on the store's goroutine at the same
// moment must never see a map mid-edit.
func (s *Sim) Close() {
	if s.eng == nil {
		return
	}
	for _, l := range s.takeAllServed() {
		_ = l.Close()
	}
	runTeardowns(s)
	_ = s.eng.Close()
	s.eng = nil
}

// provisionAll tells every node what it is, as soon as its firmware is up.
//
// This is the step that decides whether anything relays. A node that has not
// been told its regions holds none, so it forwards nothing and reports no
// error - which is what a mesh of three hundred nodes sitting in total silence
// for four minutes looked like. The old workbench does this in attachFirmware;
// this build did it only inside an experiment, so the one path an operator
// actually uses - press play - brought up MeshCore everywhere and told it
// nothing.
//
// The same lines the provisioning panel shows, so what is read and what is
// sent cannot drift apart. On the starting goroutine rather than in a verb,
// because three hundred nodes times seven commands is work proportional to the
// network and that does not belong on the store's thread.
//
// The engine and its nodes are arguments rather than the session's own fields,
// for the reason linksOf takes them: this runs on the attach's goroutine, and
// the session's pair is replaced whole when a network is opened. Told which
// network to configure, it cannot configure a different one.
func provisionAll(eng *engine.Engine, nodes []scenario.Node,
	lines func(scenario.Node) []state.ProvisionLine) int {
	if eng == nil {
		return 0
	}
	done := 0
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		sent := false
		for _, line := range lines(n) {
			if line.Comment {
				continue
			}
			if err := en.Firmware.Bridge.Type([]byte(line.Command + "\r\n")); err != nil {
				break
			}
			sent = true
		}
		if sent {
			done++
		}
	}
	return done
}
