package emulated

// noiseSeedFor turns a node's name into a seed for its receiver noise.
//
// FNV-1a, because it needs to be stable across runs and machines rather than
// unpredictable: the point is that two nodes differ, not that an observer
// cannot guess. Go's own map hashing is randomised per process and would give
// the same node a different identity on every run.
func noiseSeedFor(runSeed uint64, node string) uint64 {
	const offset, prime = uint64(1469598103934665603), uint64(1099511628211)
	h := offset
	// The run's seed first, then the name: a node differs from its neighbours
	// within a run, and from itself between runs that were seeded differently.
	for i := 0; i < 8; i++ {
		h ^= (runSeed >> (8 * i)) & 0xFF
		h *= prime
	}
	for i := 0; i < len(node); i++ {
		h ^= uint64(node[i])
		h *= prime
	}
	// Never zero: a zero seed is what this whole mechanism exists to stop being
	// the answer, and a node named such that FNV returns it should not silently
	// rejoin the broken case.
	if h == 0 {
		h = prime
	}
	return h
}

// GPIOPin is a pin as Renode addresses it: the port's name and the pin within
// it. Mirrors the board package's own type rather than importing it, because
// firmware/emulated sits below firmware/board and may not reach up.
type GPIOPin struct {
	Port string
	Pin  int
}
