// Frames as their carriers begin, for a node whose chip lands them as they
// arrive.
//
// A frame is delivered when it ends, and a firmware that reads its radio late
// on hardware finds the next frame's header, and then its bytes, behind the
// last one in the chip's buffer. A chip told only at the end cannot show that,
// so the engine says when each frame starts as well.
package engine

// framesStarting is, for every node, the transmissions that began in
// [since, now] loudly enough to be detected there. Since is included because
// a step collects what the firmware sent after running it, so a frame started
// at the last step's instant is first seen now.
func (e *Engine) framesStarting(since, now uint32) [][]transmission {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	var starting []transmission
	for _, t := range e.inFlight {
		if t.startMs >= since && t.startMs <= now {
			starting = append(starting, t)
		}
	}
	e.mu.Unlock()

	out := make([][]transmission, len(nodes))
	for _, t := range starting {
		for i, dst := range nodes {
			if dst.Firmware != nil && e.detects(nodes, t, i) {
				out[i] = append(out[i], t)
			}
		}
	}
	return out
}
