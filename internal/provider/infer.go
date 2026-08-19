package provider

import (
	"fmt"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/internal/packet"
)

// Inferred is what observed traffic says about a node's configuration.
//
// Evidence, not guesswork: every field here is something a node demonstrated by
// its own behaviour on the air. What cannot be demonstrated is left unset
// rather than estimated.
type Inferred struct {
	Node string

	// ScopedOrigin is true when this node has originated scoped traffic, which
	// means it has a default scope set.
	ScopedOrigin bool
	// DefaultScope is that scope's name, where a candidate matched. Read from
	// what the node originates — adverts and its own messages — because that is
	// what a default scope governs.
	DefaultScope string
	// ScopedRelay is true when this node has relayed somebody else's scoped
	// traffic, which means it holds a matching region and allows flooding for
	// it — a node without one drops the packet rather than forwarding it.
	ScopedRelay bool
	// UnscopedRelay is true when it has relayed unscoped flood traffic.
	UnscopedRelay bool

	// MaxHops is the largest hop count seen on a packet this node relayed. A
	// lower bound on its flood.max: it cannot be below what it has done.
	MaxHops int

	// Regions are the named regions this node's traffic matched, where the
	// names were known in advance. Empty means either that it uses none or
	// that none of the candidates fitted — the two are distinguished by
	// ScopedRelay, which says scoped traffic was carried regardless.
	Regions []string

	// PayloadTypes are the kinds of packet it has been seen to carry.
	PayloadTypes []string

	// Packets is how much evidence there is. A conclusion from three packets
	// and one from three thousand are different claims, and a reader deserves
	// to know which they are looking at.
	Packets int
}

// Summary states what was inferred, and what could not be.
func (i Inferred) Summary() string {
	if i.Packets == 0 {
		return "never seen"
	}
	var s string
	switch {
	case i.ScopedRelay:
		s = "relays scoped traffic, so it holds a region and allows flooding for it"
	case i.UnscopedRelay:
		s = "relays unscoped traffic only"
	default:
		s = "not seen relaying"
	}
	if i.ScopedOrigin {
		if i.DefaultScope != "" {
			s += "; default scope " + i.DefaultScope
		} else {
			s += "; originates scoped traffic, so it has a default scope"
		}
	}
	if len(i.Regions) > 0 {
		s += "; regions " + fmt.Sprint(i.Regions)
	}
	return fmt.Sprintf("%s (%d packets)", s, i.Packets)
}

// RegionMatcher decides whether a packet was scoped to a named region.
//
// The names come from somewhere that knows them — CoreScope publishes the
// regions currently configured, and an operator can add ones they know about.
// Without candidates, a transport code identifies nothing: it is
// calcTransportCode(packet), hashed with the packet, so the same region gives a
// different code on every message and codes cannot be matched to each other.
// With a candidate name and its key, a code can be recomputed and *checked* —
// which turns "this node relays something scoped" into "this node relays
// mesh-east".
type RegionMatcher interface {
	// Match returns the names among the candidates whose transport code equals
	// the one on this packet.
	Match(frame []byte, codes []uint16) []string
}

// InferFromPackets works out what it can about each node from observed traffic.
//
// Two levels of answer, and the difference is worth keeping straight.
//
// Without a matcher, only *behaviour* is inferable: that a node scopes what it
// originates, and that it holds some region admitting what it relays. A
// transport code is calcTransportCode(packet), hashed with the packet, so the
// same region gives a different code on every message and codes cannot be
// matched to one another.
//
// With a matcher — candidate names from CoreScope, or ones an operator knows —
// each code can be recomputed and checked, and the region can be *named*.
//
// A node may hold several regions, so names accumulate: one packet proves one
// region, never the whole set. A node with no named match is not a node with no
// regions; it is a node none of the candidates explained.
func InferFromPackets(packets []PacketRecord, m RegionMatcher) map[string]*Inferred {
	out := map[string]*Inferred{}
	get := func(name string) *Inferred {
		if name == "" {
			return nil
		}
		v, ok := out[name]
		if !ok {
			v = &Inferred{Node: name}
			out[name] = v
		}
		return v
	}

	types := map[string]map[string]bool{}
	for _, p := range packets {
		if len(p.Raw) == 0 {
			continue
		}
		d := packet.Dissect(p.Raw)
		if d.Truncated {
			continue
		}
		scoped := d.HasTransport

		// The origin: a packet with no path has not been relayed yet, so
		// whoever was heard sending it is where it came from.
		origin := p.Origin
		if origin == "" && d.HopCount() == 0 {
			origin = p.Receiver
		}
		// A scoped packet proves its *origin's* scope however many times it
		// has been relayed: the transport code is an HMAC over the payload
		// with the path excluded, so it is the code the origin computed and
		// it does not change hop to hop.
		//
		// Requiring a hop-0 copy threw nearly all of this away - an advert is
		// usually observed after some repeater has already carried it - which
		// is why almost no node came back with a default scope while the
		// network was entirely scoped.
		if o := get(origin); o != nil && scoped {
			o.ScopedOrigin = true
			// A node's default scope is what it scopes its *own* traffic to,
			// so it is read from what it originates rather than what it
			// forwards. Adverts are the clearest case: a node advertises
			// itself, under its own scope.
			if m != nil {
				for _, name := range m.Match(p.Raw, d.TransportCodes) {
					if !containsString(o.Regions, name) {
						o.Regions = append(o.Regions, name)
					}
					if d.PayloadName == "advert" || o.DefaultScope == "" {
						o.DefaultScope = name
					}
				}
			}
		}

		// Every node on the path relayed this packet, and every one of them
		// is therefore evidence about itself.
		//
		// Crediting only the last hop - whoever transmitted the copy that was
		// heard - was why a repeater carrying three regions was credited with
		// one: it appears in hundreds of paths and is the *final* hop in very
		// few of them. This is what HopReach has always done
		// (internal/corescope/scope.go, tallyPacket).
		relays := p.RelayPath
		if len(relays) == 0 && p.Sender != "" {
			relays = []string{p.Sender}
		}
		for _, hop := range relays {
			if hop == "" || hop == p.Sender {
				continue
			}
			h := get(hop)
			if h == nil {
				continue
			}
			h.Packets++
			if scoped {
				h.ScopedRelay = true
				if m != nil {
					for _, name := range m.Match(p.Raw, d.TransportCodes) {
						if !containsString(h.Regions, name) {
							h.Regions = append(h.Regions, name)
						}
					}
				}
			} else {
				h.UnscopedRelay = true
			}
			if d.HopCount() > h.MaxHops {
				h.MaxHops = d.HopCount()
			}
		}

		// The relayer: whoever transmitted this copy, if it already carried a
		// path when it was heard.
		if r := get(p.Sender); r != nil {
			r.Packets++
			if d.HopCount() > 0 {
				if scoped {
					r.ScopedRelay = true
					// Named, where the name was known in advance. A node may
					// hold several regions, so these accumulate rather than
					// replace: one packet proves one region, not the set.
					if m != nil {
						for _, name := range m.Match(p.Raw, d.TransportCodes) {
							if !containsString(r.Regions, name) {
								r.Regions = append(r.Regions, name)
							}
						}
					}
				} else {
					r.UnscopedRelay = true
				}
			}
			if d.HopCount() > r.MaxHops {
				r.MaxHops = d.HopCount()
			}
			if types[p.Sender] == nil {
				types[p.Sender] = map[string]bool{}
			}
			types[p.Sender][d.PayloadName] = true
		}
	}

	for name, set := range types {
		v := out[name]
		if v == nil {
			continue
		}
		for t := range set {
			v.PayloadTypes = append(v.PayloadTypes, t)
		}
		sort.Strings(v.PayloadTypes)
	}
	return out
}

// PacketRecord is one observed packet, as a provider reports it.
type PacketRecord struct {
	// Raw is the frame as it was on the air. Without it nothing here can be
	// inferred: the scope, the path and the type all live in those bytes.
	Raw []byte
	// Sender is who transmitted this copy, Receiver who heard it, and Origin
	// who first sent the message where the source knows.
	Sender   string
	Receiver string
	Origin   string

	// At is when the source recorded it, where it says. A walk bounded by
	// hours needs this; a walk bounded by a packet count did not, which is
	// why it was missing.
	At time.Time

	// RelayPath is every node that carried this packet, in order, as the
	// source resolved them. Every one of them relayed it - which is the
	// evidence, and taking only the last of them was why a node that relays
	// three regions was credited with one.
	RelayPath []string

	// PathHashes is the packet's own path as the source parsed it: its length
	// is the hop count, so an empty one means this copy came straight from the
	// origin. The live feed replays only the first hop, and this is how it
	// knows which copies those are.
	PathHashes []string

	// HasSNR and SNRdB are how strongly this observer heard this copy. The
	// receptions endpoint does not exist on every deployment - a live
	// CoreScope answers /api/packets with JSON and /api/receptions with its
	// own HTML - so this is where a measured signal level actually comes
	// from.
	HasSNR bool
	SNRdB  float64
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
