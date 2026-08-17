package session

import (
	"fmt"
	"sort"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// serve opens an endpoint for one companion.
func (s *Sim) serve(name, kind string) (state.Endpoint, error) {
	if s.eng == nil {
		return state.Endpoint{}, fmt.Errorf("no simulation")
	}
	var (
		link *engine.CompanionLink
		err  error
	)
	if kind == "serial" {
		link, err = s.eng.ServeCompanionSerial(name)
	} else {
		// Port zero: the operating system picks a free one and the link
		// reports it. A fixed default collides with the last run that has not
		// finished dying, and that error reads like a permissions problem.
		link, err = s.eng.ServeCompanionTCP(name, "127.0.0.1:0")
	}
	if err != nil {
		return state.Endpoint{}, err
	}
	if s.served == nil {
		s.served = map[string]*engine.CompanionLink{}
	}
	s.served[name] = link
	return state.Endpoint{Node: name, Kind: link.Kind, Addr: link.Addr}, nil
}

// endpoints is what is currently served, with whether anything is attached.
func (s *Sim) endpoints() []state.Endpoint {
	out := make([]state.Endpoint, 0, len(s.served))
	for name, l := range s.served {
		out = append(out, state.Endpoint{
			Node: name, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// dropClients unplugs every attached client.
//
// The listener goes with the connection, so this is "the device was unplugged"
// rather than "the link glitched". An application that reconnects cleanly from
// this is one that survives a phone going to sleep.
func (s *Sim) dropClients() int {
	var names []string
	for name, l := range s.served {
		if l.Attached() {
			names = append(names, name)
		}
	}
	for _, name := range names {
		_ = s.served[name].Close()
		delete(s.served, name)
	}
	return len(names)
}
