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
	if old, ok := s.dropServedLink(name); ok {
		_ = old.Close()
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
	// Worked out once and remembered, because the endpoint list is rebuilt
	// whenever anything about a companion changes and enumerating the
	// machine's interfaces on every rebuild is work for an answer that does
	// not move.
	var addrs []string
	if link.Kind == "tcp" {
		addrs = reachableAddrs(link.Addr)
	}
	s.setServedLink(name, link, addrs)
	return s.endpointFor(name, link), nil
}

// servedLink is the listener currently open for a node, if any.
func (s *Sim) servedLink(name string) (*engine.CompanionLink, bool) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	l, ok := s.served[name]
	return l, ok
}

// setServedLink records a freshly opened listener and the addresses it can be
// reached on. Call only once any old listener for the node has been taken out
// and closed - this only ever adds.
func (s *Sim) setServedLink(name string, link *engine.CompanionLink, addrs []string) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	if s.served == nil {
		s.served = map[string]*engine.CompanionLink{}
	}
	s.served[name] = link
	if s.servedAddrs == nil {
		s.servedAddrs = map[string][]string{}
	}
	delete(s.servedAddrs, name)
	if len(addrs) > 0 {
		s.servedAddrs[name] = addrs
	}
}

// dropServedLink removes one node's listener and reports whether there was
// one. Closing it is left to the caller, done once the lock is released, so a
// slow close never holds up whoever else is reaching for these maps.
func (s *Sim) dropServedLink(name string) (*engine.CompanionLink, bool) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	l, ok := s.served[name]
	if !ok {
		return nil, false
	}
	delete(s.served, name)
	delete(s.servedAddrs, name)
	return l, true
}

// eachServed runs fn once per open listener, holding the lock for the whole
// walk. fn must only use what it is handed - calling back into servedLink,
// setServedLink or dropServedLink from inside it deadlocks against this same
// lock. That is why endpoints and dropClients each enumerate here first and
// act on the result afterwards, rather than acting inside the walk.
func (s *Sim) eachServed(fn func(name string, l *engine.CompanionLink, addrs []string)) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	for name, l := range s.served {
		fn(name, l, s.servedAddrs[name])
	}
}

// servedCount is how many listeners are open right now.
func (s *Sim) servedCount() int {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	return len(s.served)
}

// takeAllServed empties both maps and hands back what they held, for Close to
// shut every listener down outside the lock - so a slow one closing never
// blocks a verb running concurrently on the store's own goroutine.
func (s *Sim) takeAllServed() map[string]*engine.CompanionLink {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	out := s.served
	s.served, s.servedAddrs = nil, nil
	return out
}

// endpointView is one served node as the interface sees it: the addresses
// somebody can type into a client, not the address the socket was bound to.
// Takes no lock, so it can be built either from a single locked lookup or
// from inside a walk that already holds one.
func endpointView(name string, l *engine.CompanionLink, addrs []string) state.Endpoint {
	ep := state.Endpoint{Node: name, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached()}
	if len(addrs) > 0 {
		ep.Addr, ep.Addrs = addrs[0], addrs
	}
	return ep
}

// endpointFor is one served node as the interface sees it.
func (s *Sim) endpointFor(name string, l *engine.CompanionLink) state.Endpoint {
	s.servedMu.Lock()
	addrs := s.servedAddrs[name]
	s.servedMu.Unlock()
	return endpointView(name, l, addrs)
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
	var out []state.Endpoint
	s.eachServed(func(name string, l *engine.CompanionLink, addrs []string) {
		out = append(out, endpointView(name, l, addrs))
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// stopServing closes one node's endpoint whether or not a client has
// attached, and reports how many links went.
func (s *Sim) stopServing(name string) int {
	l, ok := s.dropServedLink(name)
	if !ok {
		return 0
	}
	_ = l.Close()
	return 1
}

// dropClients unplugs every attached client.
//
// The listener goes with the connection, so this is "the device was unplugged"
// rather than "the link glitched". An application that reconnects cleanly from
// this is one that survives a phone going to sleep.
func (s *Sim) dropClients() int {
	var names []string
	s.eachServed(func(name string, l *engine.CompanionLink, _ []string) {
		if l.Attached() {
			names = append(names, name)
		}
	})
	for _, name := range names {
		if l, ok := s.dropServedLink(name); ok {
			_ = l.Close()
		}
	}
	return len(names)
}
