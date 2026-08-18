// Starting MeshCore on a whole network, and tearing it down again.
package session

import (
	"context"

	"github.com/MeshBench/meshbench/internal/gui/state"
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
	go func() {
		defer s.starting.Store(false)
		ctx := context.Background()
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "firmware", What: "starting firmware on every node"})
		err := s.eng.AttachNativeProgress(ctx, seed, func(done, total int) {
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
		if n := s.provisionAll(); n > 0 {
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

// firmwareCount is how many nodes are running firmware right now.
func (s *Sim) firmwareCount() int {
	if s.eng == nil {
		return 0
	}
	return s.eng.FirmwareCount()
}

// Close shuts the simulation down, firmware included.
//
// Safe on a Sim that never built an engine, because the common shutdown path
// is a workbench closed before anything was loaded.
func (s *Sim) Close() {
	if s.eng == nil {
		return
	}
	for name, l := range s.served {
		_ = l.Close()
		delete(s.served, name)
	}
	for name, srv := range s.sdrServers {
		_ = srv.Close()
		delete(s.sdrServers, name)
	}
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
func (s *Sim) provisionAll() int {
	if s.eng == nil {
		return 0
	}
	done := 0
	for _, n := range s.nodes {
		en, ok := s.eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		sent := false
		for _, line := range s.provisionLines(n) {
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
