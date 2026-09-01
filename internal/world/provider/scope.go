package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/MeshBench/meshbench/internal/diag"
	"github.com/MeshBench/meshbench/internal/mesh/packet"
)

// RegionKey derives a region's transport key from its name.
//
// SHA-256 of the name, truncated to 16 bytes — MeshCore's own derivation in
// TransportKeyStore.cpp. The key is not in the packet, which is why a code can
// only ever be *checked* against a candidate name rather than decoded into one.
func RegionKey(name string) [16]byte {
	digest := sha256.Sum256([]byte(name))
	var key [16]byte
	copy(key[:], digest[:16])
	return key
}

// NamedRegions matches packets against a set of candidate region names.
//
// The approach HopReach arrived at and validated against production traffic: a
// real captured packet's embedded transport code matched exactly one candidate
// out of a realistic set, computed independently of CoreScope's own
// classification of that packet.
//
// This is cryptographic confirmation rather than inference. A match means the
// packet really was scoped to that region — the code is an HMAC over the
// payload under the region's key, and guessing it is not feasible.
type NamedRegions struct {
	keys map[string][16]byte
}

// NewNamedRegions builds a matcher over candidate names.
//
// Candidates come from somewhere that knows them: CoreScope publishes the
// regions currently configured, and an operator can add ones they know of. A
// region nobody named cannot be identified — its traffic still shows as scoped,
// which is the honest answer rather than a wrong name.
func NewNamedRegions(names []string) *NamedRegions {
	keys := make(map[string][16]byte, len(names))
	for _, n := range names {
		if n != "" {
			keys[n] = RegionKey(n)
		}
	}
	return &NamedRegions{keys: keys}
}

// Names lists the candidates being matched against.
func (n *NamedRegions) Names() []string {
	out := make([]string, 0, len(n.keys))
	for name := range n.keys {
		out = append(out, name)
	}
	return out
}

// Match reports which candidate regions this packet was scoped to.
//
// The code is HMAC-SHA256 over the payload type followed by the payload, keyed
// on the region, truncated to its first two bytes little-endian — matching
// calcTransportCode's own uint16 output. The path is excluded, which is what
// lets the same message be recognised at every hop even though its bytes on the
// air change at each one.
//
// Candidates are tried in name order rather than map order. Two candidates can
// collide on the same two-byte code (vanishingly unlikely, but the whole point
// of a truncated code is that it happens eventually), and a caller such as
// infer.go's DefaultScope logic picks a winner from the order this returns
// them in. Alphabetical order needs no data beyond the name itself, so the
// winner is a property of the candidate set rather than of which run it was.
func (n *NamedRegions) Match(frame []byte, codes []uint16) (out []string) {
	if len(codes) == 0 || len(n.keys) == 0 {
		return nil
	}
	// frame is bytes as heard off the air or replayed out of an import - not
	// something this package controls the shape of. A hand-rolled parser fed
	// directly by that should not be able to take its caller down over one
	// bad frame, and Match has no way of knowing whether its caller is a
	// batch import or a live feed's own goroutine, so the boundary is here.
	defer func() {
		if r := recover(); r != nil {
			diag.Printf("provider", "region match panicked on a %d-byte frame: %v", len(frame), r)
			out = nil
		}
	}()
	d := packet.Dissect(frame)
	if d.Truncated || !d.HasTransport {
		return nil
	}

	msg := make([]byte, 0, 1+len(d.Payload))
	msg = append(msg, d.PayloadType)
	msg = append(msg, d.Payload...)

	names := make([]string, 0, len(n.keys))
	for name := range n.keys {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		key := n.keys[name]
		mac := hmac.New(sha256.New, key[:])
		mac.Write(msg)
		sum := mac.Sum(nil)
		if binary.LittleEndian.Uint16(sum[:2]) == codes[0] {
			// One region per code, but a caller may hold several candidates
			// that collide in two bytes — vanishingly unlikely, and reported
			// as several rather than silently picking one.
			out = append(out, name)
		}
	}
	return out
}
