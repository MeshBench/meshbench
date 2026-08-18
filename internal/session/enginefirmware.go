// Starting MeshCore on a whole network, and tearing it down again.
package session

import (
	"context"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// startupSteps is job-progress accounting: firmware coming up is one phase,
// reading every node back and reconciling it is a second one, and a progress
// bar that jumps from 0 to 100 the moment firmware is merely running
// undersells what provisioning still has left to do on a large network.
const startupSteps = 2

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
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "firmware", What: "reading every node back", Done: 1, Total: startupSteps})
		// Read every node, work out what the active rules want, send only what
		// differs, and write every reply into that node's own console - see
		// runProvisioning. This is the step that decides whether anything
		// relays: a node that has not been told its regions holds none, so it
		// forwards nothing and reports no error, which is what a mesh of three
		// hundred nodes sitting in total silence for four minutes looked like.
		if n := s.runProvisioning(ctx, st, nil); n > 0 {
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "firmware", What: "provisioning", Done: startupSteps, Total: startupSteps})
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
	_ = s.eng.Close()
	s.eng = nil
}
