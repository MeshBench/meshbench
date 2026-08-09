package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Message is one MQTT message, reduced to what this package needs.
type Message struct {
	Topic   string
	Payload []byte
}

// Subscriber is the slice of an MQTT client this package uses.
//
// An interface, and no MQTT client dependency, for one reason: every client
// library has its own opinion about reconnection, TLS, session persistence and
// backpressure, and none of those opinions belong to a provider. A caller
// brings whichever client it already trusts and hands over messages.
type Subscriber interface {
	// Subscribe delivers messages on a topic filter until the context ends.
	Subscribe(ctx context.Context, topic string, fn func(Message)) error
}

// MQTT is a live provider: receptions as they are heard, rather than a snapshot
// of what was heard earlier.
//
// It is the only source that can answer "what is happening now", and the only
// one that cannot answer "what happened last Tuesday" — which is why Live is a
// separate interface rather than an extra method everything has to implement.
type MQTT struct {
	Client Subscriber
	// Topic filter, e.g. "meshcore/+/rx".
	Topic string

	// Retain is how many recent receptions to keep for Receptions() to return.
	// A live feed has no history, so this is the only history it has; zero
	// means keep nothing and Receptions returns empty rather than pretending.
	Retain int

	mu     sync.Mutex
	recent []Reception
}

func (m *MQTT) Name() string { return "mqtt" }

// Nodes returns nothing, and says so rather than erroring.
//
// A live packet feed is not a node registry. Returning an error would make a
// caller that merges several sources have to special-case this one; returning
// an empty list says the same thing and composes.
func (m *MQTT) Nodes(context.Context) ([]NodeRecord, error) { return nil, nil }

// Receptions returns the retained window, filtered by time.
func (m *MQTT) Receptions(_ context.Context, since time.Time) ([]Reception, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Reception, 0, len(m.recent))
	for _, r := range m.recent {
		if since.IsZero() || r.At.After(since) {
			out = append(out, r)
		}
	}
	return out, nil
}

// mqttPayload is the message shape. Fields are pointers for the same reason
// they are elsewhere: a missing SNR and a 0 dB SNR are different facts.
type mqttPayload struct {
	Time     any      `json:"time"`
	Receiver string   `json:"receiver"`
	PacketID string   `json:"packet_id"`
	Hash     string   `json:"hash"`
	Origin   string   `json:"origin"`
	Hops     *int     `json:"hops"`
	SNR      *float64 `json:"snr"`
	RSSI     *float64 `json:"rssi"`
	Payload  []byte   `json:"payload"`
}

// Subscribe delivers receptions until the context is cancelled.
//
// A malformed message is dropped, not fatal. A live feed carrying one bad
// payload should not take the run down with it — but the count is worth having,
// so a caller that wants to know can wrap fn.
func (m *MQTT) Subscribe(ctx context.Context, fn func(Reception)) error {
	if m.Client == nil {
		return fmt.Errorf("provider: mqtt needs a client")
	}
	topic := m.Topic
	if topic == "" {
		topic = "meshcore/+/rx"
	}
	return m.Client.Subscribe(ctx, topic, func(msg Message) {
		rec, ok := m.decode(msg)
		if !ok {
			return
		}
		m.retain(rec)
		if fn != nil {
			fn(rec)
		}
	})
}

func (m *MQTT) decode(msg Message) (Reception, bool) {
	var p mqttPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return Reception{}, false
	}
	id := p.PacketID
	if id == "" {
		id = p.Hash
	}
	if p.Receiver == "" || id == "" {
		// Without a receiver and a packet identity there is nothing to join on,
		// and a reception that cannot be joined can only be counted.
		return Reception{}, false
	}
	rec := Reception{
		Receiver:   p.Receiver,
		PacketID:   id,
		Origin:     p.Origin,
		Source:     m.Name(),
		RawPayload: p.Payload,
	}
	if ts, ok := parseTime(p.Time); ok {
		rec.At = ts
	} else {
		// A live message with no timestamp is happening now. This is the one
		// place inventing a value is right — the alternative is a zero time
		// that sorts before everything ever recorded.
		rec.At = time.Now().UTC()
	}
	if p.Hops != nil {
		rec.HopCount = *p.Hops
	}
	if p.SNR != nil {
		rec.HasSNR, rec.SNRdB = true, *p.SNR
	}
	if p.RSSI != nil {
		rec.HasRSSI, rec.RSSIdBm = true, *p.RSSI
	}
	return rec, true
}

func (m *MQTT) retain(r Reception) {
	if m.Retain <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recent = append(m.recent, r)
	if len(m.recent) > m.Retain {
		m.recent = m.recent[len(m.recent)-m.Retain:]
	}
}
