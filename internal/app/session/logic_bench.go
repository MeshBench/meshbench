package session

import (
	"net"
	"sort"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// serve opens an endpoint for one companion.
//
// It takes the port from the workbench itself: one holder at a time, and
// serving means letting go. Done here rather than by the UI firing a
// disconnect first, because two verbs dispatched separately arrive in
// whichever order the scheduler picks - and serve landing before disconnect
// released the port out from under the client that had just taken it.
func (s *Sim) serve(name, kind string) (state.Endpoint, error) {
	if s.eng == nil {
		return state.Endpoint{}, ErrNoSimulation
	}
	if c, ok := s.comps[name]; ok {
		if c.release != nil {
			c.release()
		}
		delete(s.comps, name)
	}
	// A second Serve replaces the first listener rather than leaking it.
	if old, ok := s.served[name]; ok {
		_ = old.Close()
		delete(s.served, name)
	}
	var (
		link *engine.CompanionLink
		err  error
	)
	if kind == "serial" {
		link, err = s.eng.ServeCompanionSerial(name)
	} else {
		// Every interface, not just loopback.
		//
		// The point of serving a companion is to point a client at it, and a
		// client is often a phone or another machine. Bound to 127.0.0.1 the
		// port existed and nothing outside this computer could reach it,
		// which reads as a firewall problem rather than a decision.
		//
		// Port zero: the operating system picks a free one and the link
		// reports it. A fixed default collides with the last run that has not
		// finished dying, and that error reads like a permissions problem.
		link, err = s.eng.ServeCompanionTCP(name, "0.0.0.0:0")
	}
	if err != nil {
		return state.Endpoint{}, err
	}
	if s.served == nil {
		s.served = map[string]*engine.CompanionLink{}
	}
	s.served[name] = link
	// Worked out once and remembered, because the endpoint list is rebuilt
	// whenever anything about a companion changes and enumerating the
	// machine's interfaces on every rebuild is work for an answer that does
	// not move.
	if s.servedAddrs == nil {
		s.servedAddrs = map[string][]string{}
	}
	delete(s.servedAddrs, name)
	if link.Kind == "tcp" {
		if addrs := reachableAddrs(link.Addr); len(addrs) > 0 {
			s.servedAddrs[name] = addrs
		}
	}
	return s.endpointFor(name, link), nil
}

// endpointFor is one served node as the interface sees it: the addresses
// somebody can type into a client, not the address the socket was bound to.
func (s *Sim) endpointFor(name string, l *engine.CompanionLink) state.Endpoint {
	ep := state.Endpoint{Node: name, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached()}
	if addrs := s.servedAddrs[name]; len(addrs) > 0 {
		ep.Addr, ep.Addrs = addrs[0], addrs
	}
	return ep
}

// reachableAddrs turns a bound address into the ones another machine can
// use: this computer's own addresses, with the port that was bound.
//
// Loopback last rather than dropped: on a machine with no network it is the
// only answer there is, and a client on this computer can always use it.
func reachableAddrs(bound string) []string {
	_, port, err := net.SplitHostPort(bound)
	if err != nil {
		return nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ip, ok := a.(*net.IPNet)
			// IPv4 only: a link-local IPv6 address carries a zone that most
			// clients will not accept typed in, and every one of these is
			// meant to be typed in.
			if !ok || ip.IP.To4() == nil || ip.IP.IsLoopback() {
				continue
			}
			out = append(out, net.JoinHostPort(ip.IP.String(), port))
		}
	}
	return append(out, net.JoinHostPort("127.0.0.1", port))
}

// endpoints is what is currently served, with whether anything is attached.
func (s *Sim) endpoints() []state.Endpoint {
	out := make([]state.Endpoint, 0, len(s.served))
	for name, l := range s.served {
		out = append(out, s.endpointFor(name, l))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// stopServing closes one node's endpoint whether or not a client has
// attached, and reports how many links went.
func (s *Sim) stopServing(name string) int {
	l, ok := s.served[name]
	if !ok {
		return 0
	}
	_ = l.Close()
	delete(s.served, name)
	delete(s.servedAddrs, name)
	return 1
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
