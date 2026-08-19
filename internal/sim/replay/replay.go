// Package replay turns observed traffic back into transmissions.
//
// Observers record receptions: this node heard that packet at that time. A
// simulation needs the opposite — who transmitted what, when. Recovering one
// from the other is the point of this package, and most of the work is in being
// honest about what the recording cannot tell you.
//
// Three things it cannot:
//
// Observer clocks disagree. Two nodes reporting the same packet a second apart
// have not observed a one-second propagation delay; they have observed that one
// of their clocks is wrong. The earliest reception is therefore a *bound* on the
// transmit time, not a measurement of it.
//
// A flood is heard many times. The same payload arrives at an observer once per
// hop, and treating each as a separate origin transmission produces a mesh far
// busier than the one that was recorded.
//
// And what was never heard was never recorded. A replay reproduces the traffic
// that reached an observer, which is a subset of the traffic that existed — so
// a replayed scenario is a lower bound on channel occupancy, never a faithful
// copy.
package replay

import (
	"fmt"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/world/provider"
)

// Transmission is one recovered transmission.
type Transmission struct {
	// Origin is the node that sent it, where the recording said so.
	Origin   string
	PacketID string

	// At is when it was sent, in simulation time from the start of the session.
	At time.Duration

	// AirtimeMs is how long it occupied the channel, computed from the payload
	// length by the same formula the firmware uses.
	AirtimeMs float64

	// PayloadBytes is what the airtime was computed from. Zero means the
	// recording did not carry a payload and a default was assumed, which is
	// recorded in Assumed rather than hidden.
	PayloadBytes int

	// HeardBy is every observer that reported it, in the order they did.
	HeardBy []string

	// EarliestSkewMs is the spread between the first and last reception of this
	// packet. Large values mean observer clocks disagree, not that propagation
	// took that long — at LoRa ranges the real spread is microseconds.
	SpreadMs float64

	// Assumed lists what had to be invented to make this transmission usable.
	// Never empty silently: a replay built on assumptions should say which.
	Assumed []string
}

// Session is a replayable recording.
type Session struct {
	Transmissions []Transmission
	Start         time.Time
	Duration      time.Duration

	// Origins is every node that transmitted, and Observers every node that
	// heard something. The difference matters: an origin with no observer entry
	// is a node we can place but never validate against.
	Origins   []string
	Observers []string

	// Dropped counts what could not be recovered, by reason.
	DroppedNoOrigin  int
	DroppedNoTime    int
	DuplicateHops    int
	ClockSkewWarning bool
}

// Options control recovery.
type Options struct {
	// SF, BandwidthHz and CodingRate are needed to compute airtime. The
	// recording does not carry them — they are a property of the network, not
	// of a packet.
	SF          int
	BandwidthHz float64
	CodingRate  int

	// DefaultPayloadBytes is assumed when the recording carries no payload.
	// MeshCore adverts and acks are short; 32 bytes is a reasonable middle and
	// is reported as an assumption wherever it is used.
	DefaultPayloadBytes int

	// MaxSkew is how far apart two receptions of one packet may be before the
	// session is flagged as having clock problems. At LoRa ranges the true
	// spread is microseconds, so anything above a second is a clock.
	MaxSkew time.Duration
}

// Build recovers transmissions from receptions.
func Build(rx []provider.Reception, o Options) (Session, error) {
	if o.SF < 5 || o.SF > 12 {
		return Session{}, fmt.Errorf("replay: spreading factor %d is outside SF5-SF12", o.SF)
	}
	if o.BandwidthHz <= 0 {
		o.BandwidthHz = 250_000
	}
	if o.CodingRate < 1 || o.CodingRate > 4 {
		o.CodingRate = 1
	}
	if o.DefaultPayloadBytes <= 0 {
		o.DefaultPayloadBytes = 32
	}
	if o.MaxSkew <= 0 {
		o.MaxSkew = time.Second
	}

	// One entry per packet. A flood heard once per hop collapses here, which is
	// the whole reason PacketID has to be a content identity rather than a
	// per-reception one.
	type group struct {
		receptions  []provider.Reception
		first, last time.Time
	}
	groups := map[string]*group{}
	var order []string

	var s Session
	observers := map[string]bool{}
	origins := map[string]bool{}

	for _, r := range rx {
		if r.PacketID == "" {
			s.DroppedNoOrigin++
			continue
		}
		if r.At.IsZero() {
			s.DroppedNoTime++
			continue
		}
		observers[r.Receiver] = true

		g, seen := groups[r.PacketID]
		if !seen {
			g = &group{first: r.At, last: r.At}
			groups[r.PacketID] = g
			order = append(order, r.PacketID)
		} else {
			s.DuplicateHops++
			if r.At.Before(g.first) {
				g.first = r.At
			}
			if r.At.After(g.last) {
				g.last = r.At
			}
		}
		g.receptions = append(g.receptions, r)
	}

	if len(groups) == 0 {
		return s, nil
	}

	// Session start is the earliest reception anywhere. Simulation time runs
	// from there, so a replay is reproducible regardless of when it was
	// recorded.
	earliest := time.Time{}
	for _, id := range order {
		if earliest.IsZero() || groups[id].first.Before(earliest) {
			earliest = groups[id].first
		}
	}
	s.Start = earliest

	for _, id := range order {
		g := groups[id]
		origin := ""
		for _, r := range g.receptions {
			if r.Origin != "" {
				origin = r.Origin
				break
			}
		}
		if origin == "" {
			s.DroppedNoOrigin++
			continue
		}
		origins[origin] = true

		payload := 0
		for _, r := range g.receptions {
			if n := len(r.RawPayload); n > payload {
				payload = n
			}
		}
		var assumed []string
		if payload == 0 {
			payload = o.DefaultPayloadBytes
			assumed = append(assumed, fmt.Sprintf("payload length assumed %d bytes", payload))
		}

		spread := g.last.Sub(g.first)
		if spread > o.MaxSkew {
			s.ClockSkewWarning = true
			assumed = append(assumed, fmt.Sprintf(
				"receptions %v apart; at LoRa ranges the true spread is microseconds, so this is clock skew",
				spread.Round(time.Millisecond)))
		}

		var heard []string
		for _, r := range g.receptions {
			heard = append(heard, r.Receiver)
		}
		sort.Strings(heard)

		s.Transmissions = append(s.Transmissions, Transmission{
			Origin:       origin,
			PacketID:     id,
			At:           g.first.Sub(earliest),
			AirtimeMs:    dsp.AirtimeMillis(o.SF, o.BandwidthHz, o.CodingRate, payload, true, true),
			PayloadBytes: payload,
			HeardBy:      heard,
			SpreadMs:     float64(spread) / float64(time.Millisecond),
			Assumed:      assumed,
		})
	}

	sort.SliceStable(s.Transmissions, func(i, j int) bool {
		return s.Transmissions[i].At < s.Transmissions[j].At
	})
	if n := len(s.Transmissions); n > 0 {
		last := s.Transmissions[n-1]
		s.Duration = last.At + time.Duration(last.AirtimeMs)*time.Millisecond
	}
	s.Origins = keys(origins)
	s.Observers = keys(observers)
	return s, nil
}

// ChannelUtilisation is the fraction of the session during which something was
// transmitting, as recorded.
//
// A lower bound, always. What was never heard was never recorded, so the real
// channel was at least this busy and possibly much busier — which is exactly
// the wrong direction to be wrong in when the question is whether a mesh is
// congested.
func (s Session) ChannelUtilisation() float64 {
	if s.Duration <= 0 {
		return 0
	}
	var busy time.Duration
	for _, t := range s.Transmissions {
		busy += time.Duration(t.AirtimeMs) * time.Millisecond
	}
	return float64(busy) / float64(s.Duration)
}

// Describe states what the session is and what it is not.
func (s Session) Describe() string {
	if len(s.Transmissions) == 0 {
		return "Nothing recoverable: no reception carried both a packet identity and a time."
	}
	out := fmt.Sprintf(
		"%d transmissions over %v from %d origins, heard by %d observers.\n"+
			"Channel busy %.1f%% of the time — a LOWER BOUND, because traffic no observer "+
			"heard was never recorded.\n"+
			"%d additional receptions were the same packets arriving again (floods and repeats).",
		len(s.Transmissions), s.Duration.Round(time.Second), len(s.Origins), len(s.Observers),
		s.ChannelUtilisation()*100, s.DuplicateHops)
	if s.DroppedNoOrigin > 0 || s.DroppedNoTime > 0 {
		out += fmt.Sprintf("\nDropped: %d with no origin, %d with no timestamp.",
			s.DroppedNoOrigin, s.DroppedNoTime)
	}
	if s.ClockSkewWarning {
		out += "\n\nWARNING: some packets were reported by different observers more than a " +
			"second apart. At LoRa ranges the real spread is microseconds, so those observers' " +
			"clocks disagree. Transmit times here are bounds, not measurements."
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
