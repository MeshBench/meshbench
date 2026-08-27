// Package sdr serves a placed observer's antenna to real SDR software over
// rtl_tcp, streaming signal-only IQ from the shared synthesis. Split out of
// internal/app/session: its running servers are the first domain state held off
// the Sim struct through the DomainState seam, closed on an engine teardown and
// re-described into the snapshot each tick through the seam's hooks.
package sdr

import (
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func init() {
	session.RegisterDomain(registerSDRServe)
	// On an engine rebuild every served socket is closed, exactly as the core
	// teardown used to do inline before the state moved out here.
	session.RegisterTeardown(func(s *session.Sim) {
		ss := stateOf(s)
		for name, e := range ss.servers {
			e.shutdown()
			delete(ss.servers, name)
		}
	})
	// A client attaching or leaving is not a verb, so the served list is
	// re-described into the snapshot every step while anything is served.
	session.RegisterTick(func(s *session.Sim, w *state.World) {
		ss := stateOf(s)
		if len(ss.servers) > 0 || len(w.SDRSources) > 0 {
			w.SDRSources = sources(ss)
		}
	})
}
