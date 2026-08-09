// Package provider is where real-world data comes in.
//
// Three sources exist today — CoreScope, MeshCore Beacon and live MQTT — and
// more will. They disagree about almost everything: field names, time formats,
// whether a position is a point or a guess, whether a reception names the
// packet it heard. What they agree on is the shape of the question, and that
// shape is this interface.
//
// The rule that matters here: a provider reports what it was told, and marks
// what it does not know. Filling a missing position with (0,0) or a missing
// uncertainty with zero turns an absent fact into a confident wrong one, and
// nothing downstream can tell the difference afterwards.
package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// NodeRecord is a node as some source describes it.
type NodeRecord struct {
	// Name is the source's own identifier, and PublicKey the MeshCore identity
	// where the source knows it. Keys are the only reliable join across
	// sources; names are set by humans and collide.
	Name      string
	PublicKey string

	// HasPosition is false when the source did not give one. A record without
	// a position is still useful — it says the node exists and was heard — so
	// it is kept rather than dropped.
	HasPosition bool
	Lat, Lon    float64

	// UncertaintyKm is the radius the position is good to. Inherited from
	// hamreach HAM-34: a node imported at +/-5 km does not get a confident
	// answer, and the uncertainty has to survive the import to say so.
	UncertaintyKm float64

	HeightAGLm float64
	Kind       string
	LastSeen   time.Time

	// Source names the provider, so a record can be argued with later.
	Source string
}

// Reception is one node reporting that it heard one packet.
//
// This is the raw material for replaying real traffic (MSIM-27) and for
// checking the RF model against reality (MSIM-28), which is why PacketID
// matters more than it looks: without a stable identifier for the packet, two
// nodes reporting the same transmission cannot be recognised as the same event,
// and the only thing a large pile of receptions can then support is counting.
type Reception struct {
	At         time.Time
	Receiver   string
	PacketID   string
	Origin     string
	HopCount   int
	HasSNR     bool
	SNRdB      float64
	HasRSSI    bool
	RSSIdBm    float64
	Source     string
	RawPayload []byte
}

// Provider is a source of nodes and past receptions.
type Provider interface {
	Name() string
	Nodes(ctx context.Context) ([]NodeRecord, error)
	// Receptions returns everything the source has since a time. A zero time
	// means everything it will give.
	Receptions(ctx context.Context, since time.Time) ([]Reception, error)
}

// Live is a provider that can also push receptions as they happen.
//
// Separate from Provider rather than folded into it: CoreScope is a database
// and MQTT is a firehose, and pretending a database can stream would push a
// polling loop into every caller that only wanted a snapshot.
type Live interface {
	Provider
	// Subscribe delivers receptions until the context is cancelled. It returns
	// only when the subscription ends.
	Subscribe(ctx context.Context, fn func(Reception)) error
}

// Registry holds the providers a build knows about.
type Registry struct {
	mu sync.RWMutex
	ps map[string]Provider
}

func NewRegistry() *Registry { return &Registry{ps: map[string]Provider{}} }

// Register adds a provider. A duplicate name is an error rather than a silent
// replacement — two sources quietly sharing a name is how data ends up
// attributed to the wrong one.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := strings.ToLower(p.Name())
	if _, exists := r.ps[name]; exists {
		return fmt.Errorf("provider: %q is already registered", name)
	}
	r.ps[name] = p
	return nil
}

func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.ps[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("provider: no provider named %q; have %s",
			name, strings.Join(r.names(), ", "))
	}
	return p, nil
}

func (r *Registry) names() []string {
	out := make([]string, 0, len(r.ps))
	for n := range r.ps {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Names lists what is registered, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.names()
}

// MergeNodes combines records from several sources into one node list.
//
// Joined on public key where both sides have one, and on name otherwise.
// Positions are taken from the *most certain* source rather than the most
// recent: a fresh record at town-level accuracy should not displace a surveyed
// position from last year. Recency decides LastSeen and nothing else.
func MergeNodes(sets ...[]NodeRecord) []NodeRecord {
	byKey := map[string]*NodeRecord{}
	var order []string

	for _, set := range sets {
		for _, rec := range set {
			k := joinKey(rec)
			cur, seen := byKey[k]
			if !seen {
				copy := rec
				byKey[k] = &copy
				order = append(order, k)
				continue
			}
			if rec.LastSeen.After(cur.LastSeen) {
				cur.LastSeen = rec.LastSeen
			}
			if cur.PublicKey == "" {
				cur.PublicKey = rec.PublicKey
			}
			if cur.HeightAGLm == 0 {
				cur.HeightAGLm = rec.HeightAGLm
			}
			if cur.Kind == "" {
				cur.Kind = rec.Kind
			}
			if !rec.HasPosition {
				continue
			}
			// Better means smaller uncertainty. A record with no position at
			// all never wins, and a record with an unknown uncertainty (zero)
			// is treated as exact — which is what a source claiming a bare
			// point is asserting, whether it meant to or not.
			if !cur.HasPosition || rec.UncertaintyKm < cur.UncertaintyKm {
				cur.HasPosition = true
				cur.Lat, cur.Lon = rec.Lat, rec.Lon
				cur.UncertaintyKm = rec.UncertaintyKm
				cur.Source = rec.Source
			}
		}
	}

	out := make([]NodeRecord, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

func joinKey(r NodeRecord) string {
	if r.PublicKey != "" {
		return "k:" + strings.ToLower(r.PublicKey)
	}
	return "n:" + strings.ToLower(r.Name)
}
