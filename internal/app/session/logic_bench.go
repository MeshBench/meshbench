package session

import (
	"fmt"
	"net"
	"sort"
	"strconv"

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
func (s *Sim) serve(name, kind string) (state.Endpoint, string, error) {
	if s.eng == nil {
		return state.Endpoint{}, "", ErrNoSimulation
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
		link  *engine.CompanionLink
		moved string
		err   error
	)
	if kind == "serial" {
		link, err = s.eng.ServeCompanionSerial(name)
	} else {
		link, moved, err = s.serveTCP(name)
	}
	if err != nil {
		return state.Endpoint{}, "", err
	}
	// Worked out once and remembered, because the endpoint list is rebuilt
	// whenever anything about a companion changes and enumerating the
	// machine's interfaces on every rebuild is work for an answer that does
	// not move.
	var addrs []string
	if link.Kind == "tcp" {
		addrs = reachableAddrs(link.Addr)
		s.rememberPort(name, link.Addr)
	}
	s.setServedLink(name, link, addrs)
	return s.endpointFor(name, link), moved, nil
}

// serveTCP puts a node back on the port it was served on last time.
//
// Every interface, not just loopback: the point of serving a companion is to
// point a client at it, and a client is often a phone or another machine.
// Bound to 127.0.0.1 the port existed and nothing outside this computer could
// reach it, which reads as a firewall problem rather than a decision.
//
// The port itself is the operating system's to pick the first time, because a
// fixed default collides with the last run that has not finished dying and
// that error reads like a permissions problem. Every serve after that asks for
// the same one back: a client attached to the first address cannot be told
// about a second, so a re-serve that moved left it talking to a closed socket.
func (s *Sim) serveTCP(name string) (*engine.CompanionLink, string, error) {
	if port, ok := s.servedPort(name); ok {
		link, err := s.eng.ServeCompanionTCP(name, fmt.Sprintf("0.0.0.0:%d", port))
		if err == nil {
			return link, "", nil
		}
		// Announced, not swallowed: an endpoint that moves without saying so is
		// the whole fault this remembering exists to stop.
		link, ferr := s.eng.ServeCompanionTCP(name, "0.0.0.0:0")
		if ferr != nil {
			return nil, "", ferr
		}
		return link, fmt.Sprintf(
			"port %d was taken, so this is a new address - anything pointed at "+
				"the old one needs repointing", port), nil
	}
	link, err := s.eng.ServeCompanionTCP(name, "0.0.0.0:0")
	return link, "", err
}

// servedPort is the port this node was last served on, if it has been.
func (s *Sim) servedPort(name string) (int, bool) {
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	p, ok := s.servedPorts[name]
	return p, ok
}

// PortOf is the port half of the address a listener reports, and zero for
// anything that is not one.
//
// Exported for the domains split out of this package, which keep their own
// listeners and have the same reason to ask a node's port back.
func PortOf(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0
	}
	return n
}

// rememberPort keeps the port out of a listener's own address, so the next
// serve of this node can ask for it again.
func (s *Sim) rememberPort(name, addr string) {
	// Zero is what the operating system was asked, not what it answered:
	// keeping it would ask for a fresh port on every serve, which is the fault.
	n := PortOf(addr)
	if n == 0 {
		return
	}
	s.servedMu.Lock()
	defer s.servedMu.Unlock()
	if s.servedPorts == nil {
		s.servedPorts = map[string]int{}
	}
	s.servedPorts[name] = n
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
