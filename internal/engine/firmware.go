package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/A13xB0/meshcoresim/internal/firmware"
)

// AttachNative starts a real MeshCore build for every node that runs firmware.
//
// This is what separates the project from a packet-level simulator, and until
// something calls it the engine is only exercising its own channel. A node with
// firmware relays because MeshCore decided to, on an SNR that came out of a
// demodulator; a node without it relays because a test injected something.
//
// Nodes that do not run firmware — SDR observers, and custom emitters that are
// only present to interfere — are skipped rather than given one.
func (e *Engine) AttachNative(ctx context.Context, seed uint64) error {
	if _, err := firmware.FindNative(""); err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	for i, n := range nodes {
		if !n.Spec.Kind.RunsFirmware() || n.Firmware != nil {
			continue
		}
		// A seed per node, derived from the run's seed and the node's index.
		// Sharing one seed would give every node the same identity, and a mesh
		// where every repeater has the same public key does not behave like a
		// mesh at all.
		fw, err := firmware.Start(ctx, n.Spec.Name, &firmware.Native{
			Seed: seed + uint64(i)*0x9E3779B97F4A7C15,
			SF:   e.Config.SF, BandwidthKHz: e.Config.BandwidthHz / 1000,
			CodingRate: e.Config.CodingRate,
		})
		if err != nil {
			return fmt.Errorf("engine: start firmware for %s: %w", n.Spec.Name, err)
		}
		// Start returns once the process exists, not once it has connected. The
		// engine ticks immediately afterwards, and a tick to a bridge with
		// nothing on the other end fails — so the wait is not a convenience,
		// it is the difference between working and not.
		if err := waitAttached(ctx, fw, attachTimeout); err != nil {
			return fmt.Errorf("engine: %s: %w", n.Spec.Name, err)
		}

		e.mu.Lock()
		e.nodes[i].Firmware = fw
		e.mu.Unlock()
	}
	return nil
}

// attachTimeout is how long a firmware process gets to connect back.
const attachTimeout = 10 * time.Second

func waitAttached(ctx context.Context, n *firmware.Node, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for !n.Bridge.Attached() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("firmware started but never connected within %v", timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// FirmwareCount is how many nodes are running a real build.
func (e *Engine) FirmwareCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, node := range e.nodes {
		if node.Firmware != nil {
			n++
		}
	}
	return n
}

// NodeByName finds a node, for the console and the inspector.
func (e *Engine) NodeByName(name string) (*Node, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.nodes {
		if n.Spec.Name == name {
			return n, true
		}
	}
	return nil, false
}
