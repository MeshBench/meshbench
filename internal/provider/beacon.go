package provider

import (
	"context"
	"fmt"
	"time"
)

// Beacon reads a MeshCore Beacon instance.
//
// Beacon reports what its own receivers heard, so its node list is derived from
// traffic rather than surveyed: a node exists in Beacon because something heard
// it. That makes its positions weaker than CoreScope's and its receptions at
// least as good, which is exactly why both are behind one interface — merging
// them takes the better half of each.
type Beacon struct {
	BaseURL string
	Token   string
	HTTP    Doer

	// Progress, if set, is called with the record count once the fetch lands.
	// Beacon answers in one response, so this fires once — the field exists so
	// both providers offer the same contract to a UI.
	Progress func(fetched int)
}

func (b *Beacon) Name() string { return "beacon" }

func (b *Beacon) headers() map[string]string {
	if b.Token == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + b.Token}
}

type beaconNode struct {
	ID        string   `json:"id"`
	Key       string   `json:"key"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Type      string   `json:"type"`
	Updated   any      `json:"updated_at"`
}

func (b *Beacon) Nodes(ctx context.Context) ([]NodeRecord, error) {
	if b.BaseURL == "" {
		return nil, fmt.Errorf("provider: beacon needs a BaseURL")
	}
	var payload []beaconNode
	if err := fetchJSON(ctx, b.HTTP, b.BaseURL+"/nodes", b.headers(), &payload); err != nil {
		return nil, err
	}

	out := make([]NodeRecord, 0, len(payload))
	for _, n := range payload {
		r := NodeRecord{Name: n.ID, PublicKey: n.Key, Kind: n.Type, Source: b.Name()}
		if ts, ok := parseTime(n.Updated); ok {
			r.LastSeen = ts
		}
		if n.Latitude != nil && n.Longitude != nil {
			r.HasPosition = true
			r.Lat, r.Lon = *n.Latitude, *n.Longitude
			// Beacon does not publish an accuracy, and its positions come from
			// advert payloads rather than a survey. Recording them as exact
			// would let them win the merge against CoreScope's surveyed ones,
			// which is precisely backwards.
			r.UncertaintyKm = defaultInferredUncertaintyKm
		}
		out = append(out, r)
	}
	if b.Progress != nil {
		b.Progress(len(out))
	}
	return out, nil
}

type beaconPacket struct {
	Timestamp any      `json:"timestamp"`
	Node      string   `json:"node"`
	Hash      string   `json:"hash"`
	From      string   `json:"from"`
	HopLimit  *int     `json:"hop_limit"`
	Hops      *int     `json:"hops_away"`
	SNR       *float64 `json:"snr"`
	RSSI      *float64 `json:"rssi"`
}

func (b *Beacon) Receptions(ctx context.Context, since time.Time) ([]Reception, error) {
	if b.BaseURL == "" {
		return nil, fmt.Errorf("provider: beacon needs a BaseURL")
	}
	url := b.BaseURL + "/packets"
	if !since.IsZero() {
		url += fmt.Sprintf("?after=%d", since.UTC().Unix())
	}
	var payload []beaconPacket
	if err := fetchJSON(ctx, b.HTTP, url, b.headers(), &payload); err != nil {
		return nil, err
	}

	out := make([]Reception, 0, len(payload))
	for _, p := range payload {
		rec := Reception{
			Receiver: p.Node,
			// Beacon identifies a packet by content hash, which is exactly what
			// is needed to recognise one transmission heard by several nodes.
			PacketID: p.Hash,
			Origin:   p.From,
			Source:   b.Name(),
		}
		if ts, ok := parseTime(p.Timestamp); ok {
			rec.At = ts
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
		out = append(out, rec)
	}
	return out, nil
}
