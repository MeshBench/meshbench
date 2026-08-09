package provider

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CoreScope reads a CoreScope deployment.
//
// CoreScope is the source of record for where repeaters actually are, and it is
// also the source of observer data: nodes that report what they heard. Those
// two together are what makes replaying real traffic possible at all.
type CoreScope struct {
	// BaseURL of the deployment, without a trailing slash.
	BaseURL string
	// Token, if the deployment needs one.
	Token string
	HTTP  Doer
}

func (c *CoreScope) Name() string { return "corescope" }

func (c *CoreScope) headers() map[string]string {
	if c.Token == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + c.Token}
}

// csNode mirrors the API. Positions are pointers because absent and zero are
// different things and the JSON does not distinguish them any other way: a node
// with no position decodes to nil, not to the Atlantic off Ghana.
type csNode struct {
	Name          string   `json:"name"`
	PublicKey     string   `json:"public_key"`
	Lat           *float64 `json:"lat"`
	Lon           *float64 `json:"lon"`
	AccuracyM     *float64 `json:"position_accuracy_m"`
	HeightAGLm    *float64 `json:"antenna_height_m"`
	Role          string   `json:"role"`
	LastHeard     any      `json:"last_heard"`
	PositionIsFix *bool    `json:"position_is_fix"`
}

func (c *CoreScope) Nodes(ctx context.Context) ([]NodeRecord, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("provider: corescope needs a BaseURL")
	}
	var payload struct {
		Nodes []csNode `json:"nodes"`
	}
	if err := fetchJSON(ctx, c.HTTP, c.BaseURL+"/api/nodes", c.headers(), &payload); err != nil {
		return nil, err
	}

	out := make([]NodeRecord, 0, len(payload.Nodes))
	for _, n := range payload.Nodes {
		r := NodeRecord{
			Name:      n.Name,
			PublicKey: n.PublicKey,
			Kind:      strings.ToLower(n.Role),
			Source:    c.Name(),
		}
		if ts, ok := parseTime(n.LastHeard); ok {
			r.LastSeen = ts
		}
		if n.HeightAGLm != nil {
			r.HeightAGLm = *n.HeightAGLm
		}
		if n.Lat != nil && n.Lon != nil {
			r.HasPosition = true
			r.Lat, r.Lon = *n.Lat, *n.Lon
			switch {
			case n.AccuracyM != nil:
				r.UncertaintyKm = *n.AccuracyM / 1000
			case n.PositionIsFix != nil && !*n.PositionIsFix:
				// A position CoreScope knows is not a fix — typically inferred
				// from where it was heard. Recorded as coarse rather than
				// exact, because the alternative is a guess presented as a
				// survey.
				r.UncertaintyKm = defaultInferredUncertaintyKm
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// defaultInferredUncertaintyKm is what an unsurveyed position is worth.
//
// Deliberately large. A node placed by inference is usually somewhere in the
// area it was heard from, and 5 km is the scale at which that is honest for
// LoRa. It is better to be visibly uncertain than invisibly wrong.
const defaultInferredUncertaintyKm = 5.0

type csReception struct {
	Time     any      `json:"time"`
	Receiver string   `json:"receiver"`
	PacketID string   `json:"packet_id"`
	Origin   string   `json:"origin"`
	Hops     *int     `json:"hops"`
	SNR      *float64 `json:"snr"`
	RSSI     *float64 `json:"rssi"`
}

func (c *CoreScope) Receptions(ctx context.Context, since time.Time) ([]Reception, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("provider: corescope needs a BaseURL")
	}
	url := c.BaseURL + "/api/receptions"
	if !since.IsZero() {
		url += "?since=" + since.UTC().Format(time.RFC3339)
	}
	var payload struct {
		Receptions []csReception `json:"receptions"`
	}
	if err := fetchJSON(ctx, c.HTTP, url, c.headers(), &payload); err != nil {
		return nil, err
	}

	out := make([]Reception, 0, len(payload.Receptions))
	for _, r := range payload.Receptions {
		rec := Reception{
			Receiver: r.Receiver,
			PacketID: r.PacketID,
			Origin:   r.Origin,
			Source:   c.Name(),
		}
		if ts, ok := parseTime(r.Time); ok {
			rec.At = ts
		}
		if r.Hops != nil {
			rec.HopCount = *r.Hops
		}
		// Zero is a real SNR and a real RSSI. Only a null means "not reported",
		// which is why these are pointers and why the flags exist — a 0 dB SNR
		// averaged in as if it were missing, or a missing one averaged in as if
		// it were 0 dB, are both wrong and neither looks it.
		if r.SNR != nil {
			rec.HasSNR, rec.SNRdB = true, *r.SNR
		}
		if r.RSSI != nil {
			rec.HasRSSI, rec.RSSIdBm = true, *r.RSSI
		}
		out = append(out, rec)
	}
	return out, nil
}
