// Putting something on the air from outside the firmware.
//
// Two ways in, and the difference matters: Inject asks a node's firmware to
// originate a message, so what flies is a real MeshCore packet the rest of the
// network will relay; InjectFrame replays recorded bytes unaltered, which is
// how observed traffic is flown again as itself.
package engine

import (
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

// Inject introduces a message into the network from a node.
//
// Where the node runs firmware, the firmware is asked to originate it and
// builds a real MeshCore packet — which is the only way the rest of the network
// will relay it. A frame fabricated here is not a valid packet, every receiving
// node drops it, and the result is a flood that reaches its neighbours and stops
// dead. That failure is silent and looks exactly like a network with no relays
// configured, which is why it is worth this branch.
//
// Where there is no firmware, the frame goes on the air as-is. That still
// exercises the channel, the collisions and the ledger, and it is how a
// scenario runs at all without a MeshCore build to hand.
func (e *Engine) Inject(nodeIndex int, payload []byte) {
	e.mu.Lock()
	ok := nodeIndex >= 0 && nodeIndex < len(e.nodes)
	var fw *firmware.Node
	if ok {
		fw = e.nodes[nodeIndex].Firmware
	}
	now := e.nowMs
	e.mu.Unlock()
	if !ok {
		return
	}
	if fw != nil {
		if err := fw.Bridge.Originate(payload); err != nil {
			e.record(Event{AtMs: now, Kind: "miss", Detail: err.Error()})
		}
		return
	}
	e.startTransmission(nodeIndex, payload, now)
}

// InjectFrame puts raw bytes on the air from a node, exactly as recorded.
//
// The live-replay path: a packet the real network's origin transmitted is
// re-transmitted here by the same-named simulated node, bytes unaltered, so
// the region scope, path and type the other nodes' firmware will judge are the
// real ones. Deliberately not Originate(): the firmware would wrap the payload
// in a new packet of its own, and the point is to replay the packet that flew.
func (e *Engine) InjectFrame(nodeIndex int, frame []byte) {
	e.mu.Lock()
	ok := nodeIndex >= 0 && nodeIndex < len(e.nodes)
	now := e.nowMs
	e.mu.Unlock()
	if !ok || len(frame) == 0 {
		return
	}
	e.startTransmission(nodeIndex, frame, now)
}
