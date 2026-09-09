package engine

// nativeIdentitySeed is the seed a native node derives its keypair from,
// keyed on the run seed and the node's name.
//
// It used to be seed + index*golden-ratio, so node 0 of every fixture with the
// same run seed shared a keypair with node 0 of every other, and two fixtures
// opened in turn into one profile saw each other's nodes as themselves. Keyed
// on the name instead, a node is the same across runs of one fixture and
// different from every other fixture's, the way the emulated backend already
// derives its receiver-noise seed. FNV-1a spreads the entropy across all 64
// bits, so the low 32 the bridge keeps (the firmware's --seed parser reads a
// uint32) are full entropy; never zero, because a zero seed is the firmware's
// own default and would read as unseeded.
func nativeIdentitySeed(runSeed uint64, name string) uint64 {
	const offset, prime = uint64(1469598103934665603), uint64(1099511628211)
	h := offset
	mix := func(b byte) {
		h ^= uint64(b)
		h *= prime
	}
	for i := 0; i < 8; i++ {
		mix(byte((runSeed >> (8 * i)) & 0xFF))
	}
	for i := 0; i < len(name); i++ {
		mix(name[i])
	}
	mix(0x1F)
	if h == 0 {
		return prime
	}
	return h
}
