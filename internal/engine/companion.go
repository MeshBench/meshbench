package engine

import (
	"fmt"
	"io"

	"github.com/MeshBench/meshbench/internal/companion"
)

// nodeSerial is one node's UART, as the companion transports need it.
//
// A thin adapter and nothing more. MeshCore's own framing — '>' or '<' then a
// little-endian length — is already in these bytes; the transports move them
// unaltered, which is the entire fix for a companion link that no real client
// could parse.
type nodeSerial struct {
	node    *Node
	release func()
}

func (s *nodeSerial) Write(b []byte) error {
	if s.node.Firmware == nil {
		return fmt.Errorf("engine: %s runs no firmware", s.node.Spec.Name)
	}
	return s.node.Firmware.Bridge.Type(b)
}

func (s *nodeSerial) Attach(w io.Writer) {
	if s.node.Firmware == nil {
		return
	}
	if s.release != nil {
		s.release()
	}
	// Exclusive, like a USB cable: the workbench console re-attaches itself
	// every frame, and without a claim it would take the port back from the
	// client on the next one.
	s.release = s.node.Firmware.Bridge.Claim(w)
}

func (s *nodeSerial) Detach() {
	if s.release != nil {
		s.release()
		s.release = nil
	}
}

// CompanionLink is a live connection to one node's companion interface.
type CompanionLink struct {
	Node string
	// Kind is "tcp" or "serial", and Addr is the port or device path — what a
	// client is actually pointed at.
	Kind string
	Addr string

	closer io.Closer
}

// Attached reports whether a client currently holds the link.
func (c *CompanionLink) Attached() bool {
	if a, ok := c.closer.(interface{ Attached() bool }); ok {
		return a.Attached()
	}
	return false
}

func (c *CompanionLink) Close() error {
	if c.closer == nil {
		return nil
	}
	return c.closer.Close()
}

// ServeCompanionTCP exposes a node's companion interface on a TCP port.
func (e *Engine) ServeCompanionTCP(name, addr string) (*CompanionLink, error) {
	n, err := e.companionNode(name)
	if err != nil {
		return nil, err
	}
	l, err := companion.ListenTCP(addr, &nodeSerial{node: n})
	if err != nil {
		return nil, err
	}
	return &CompanionLink{Node: name, Kind: "tcp", Addr: l.Addr(), closer: l}, nil
}

// ServeCompanionSerial exposes a node as a virtual serial device.
//
// The transport that makes client software believe: a real character device the
// operating system hands out, so anything that opens a serial port attaches
// without knowing it is talking to a simulation.
func (e *Engine) ServeCompanionSerial(name string) (*CompanionLink, error) {
	n, err := e.companionNode(name)
	if err != nil {
		return nil, err
	}
	l, err := companion.OpenSerial(&nodeSerial{node: n})
	if err != nil {
		return nil, err
	}
	return &CompanionLink{Node: name, Kind: "serial", Addr: l.Path(), closer: l}, nil
}

func (e *Engine) companionNode(name string) (*Node, error) {
	n, ok := e.NodeByName(name)
	if !ok {
		return nil, fmt.Errorf("engine: no node %q", name)
	}
	if n.Firmware == nil {
		return nil, fmt.Errorf("engine: %s runs no firmware, so it has no companion "+
			"interface to expose; start firmware first", name)
	}
	return n, nil
}
