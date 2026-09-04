package emulated

import "math"

// noiseSeedFor turns what makes a node that node into a seed for its receiver
// noise.
//
// FNV-1a, because it needs to be stable across runs and machines rather than
// unpredictable: the point is that two nodes differ, not that an observer
// cannot guess. Go's own map hashing is randomised per process and would give
// the same node a different identity on every run, which is the opposite of
// what a reproducible simulation promises.
//
// The name alone was not enough. MeshCore takes its entropy from the radio, so
// this seed decides the identity the firmware generates - and every board the
// probe measures is called "bc-under-test". A LilyGo T-Deck under QEMU and a
// RAK4631 under Renode therefore came up as the same node, byte for byte,
// which reads exactly like one board's state leaking into another's.
//
// Position and board go in with it. Two nodes of one name in one place running
// different hardware are still two nodes; two of one name in different places
// are as well. All three are properties of the scenario, so the same scenario
// still seeds identically every time and determinism is untouched.
//
// What this does NOT fix is a single node getting the same identity on every
// boot, which is what hides a board that cannot persist one (#565). That is a
// deliberate consequence of seeding from the scenario, and changing it is a
// decision about what an emulated node's entropy means rather than a bug in
// this function.
func noiseSeedFor(runSeed uint64, node, board string, pos LatLon) uint64 {
	const offset, prime = uint64(1469598103934665603), uint64(1099511628211)
	h := offset
	mix := func(b byte) {
		h ^= uint64(b)
		h *= prime
	}
	mixU64 := func(v uint64) {
		for i := 0; i < 8; i++ {
			mix(byte((v >> (8 * i)) & 0xFF))
		}
	}
	mixStr := func(s string) {
		for i := 0; i < len(s); i++ {
			mix(s[i])
		}
		// A separator, so ("ab","c") and ("a","bc") are different nodes rather
		// than the same one spelled two ways.
		mix(0x1F)
	}
	// The run's seed first, then what identifies the node within it: a node
	// differs from its neighbours within a run, and from itself between runs
	// that were seeded differently.
	mixU64(runSeed)
	mixStr(node)
	mixStr(board)
	// Quantised to microdegrees and mixed as integers rather than as float
	// bits. About a tenth of a metre, finer than any two nodes anybody places
	// apart, and it cannot vary with how a float was rounded on the way in -
	// a seed that differed by machine would take reproducibility with it.
	mixU64(uint64(microDegrees(pos.Lat)))
	mixU64(uint64(microDegrees(pos.Lon)))
	// Never zero: a zero seed is what this whole mechanism exists to stop being
	// the answer, and a node seeded such that FNV returns it should not
	// silently rejoin the broken case.
	if h == 0 {
		h = prime
	}
	return h
}

// LatLon is where a node stands, in degrees. Mirrors the scenario package's own
// type rather than importing it, the same way GPIOPin mirrors the board
// package's: firmware/emulated sits below world/scenario and may not reach up.
type LatLon struct{ Lat, Lon float64 }

// microDegrees rounds a coordinate to a stable integer.
//
// Round-half-away-from-zero rather than Go's truncation, so a coordinate that
// lands exactly between two microdegrees does not depend on its sign. A NaN or
// an infinity - which a node with no position can carry - becomes zero rather
// than an implementation-defined conversion.
func microDegrees(deg float64) int64 {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return 0
	}
	return int64(math.Round(deg * 1e6))
}

// GPIOPin is a pin as Renode addresses it: the port's name and the pin within
// it. Mirrors the board package's own type rather than importing it, because
// firmware/emulated sits below firmware/board and may not reach up.
type GPIOPin struct {
	Port string
	Pin  int
}
