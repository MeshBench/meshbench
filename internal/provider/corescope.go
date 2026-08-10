package provider

import (
	"context"
	"encoding/hex"
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

	// Progress, if set, is called with the running record count as pages
	// arrive. A big deployment takes long enough to fetch that a caller with a
	// user attached needs something to show them.
	Progress func(fetched int)
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

// csPageLimit is the page size asked of GET /api/nodes. CoreScope's own default
// is fifty; asking for more per round trip is just fewer round trips.
const csPageLimit = 200

// csMaxNodes bounds the paging loop.
const csMaxNodes = 100_000

func (c *CoreScope) Nodes(ctx context.Context) ([]NodeRecord, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("provider: corescope needs a BaseURL")
	}
	// Paged, because CoreScope pages. One bare GET /api/nodes returns the
	// server's default page — fifty rows — and a deployment of three hundred
	// repeaters silently became a deployment of fifty, which is the worst kind
	// of wrong: a plausible network that is missing most of itself.
	var nodes []csNode
	for offset := 0; ; {
		var payload struct {
			Nodes []csNode `json:"nodes"`
		}
		url := fmt.Sprintf("%s/api/nodes?limit=%d&offset=%d", c.BaseURL, csPageLimit, offset)
		if err := fetchJSON(ctx, c.HTTP, url, c.headers(), &payload); err != nil {
			return nil, err
		}
		nodes = append(nodes, payload.Nodes...)
		if c.Progress != nil {
			c.Progress(len(nodes))
		}
		if len(payload.Nodes) < csPageLimit {
			break
		}
		offset += len(payload.Nodes)
		// A server that keeps returning full pages for ever is broken or
		// hostile; either way the import should stop rather than spin.
		if offset > csMaxNodes {
			return nil, fmt.Errorf("provider: corescope returned more than %d nodes; refusing to keep paging", csMaxNodes)
		}
	}

	out := make([]NodeRecord, 0, len(nodes))
	for _, n := range nodes {
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

// csPacketPageLimit is the page size for GET /api/packets. CoreScope returns
// newest first, which is what lets a walk stop once it is far enough back.
const csPacketPageLimit = 500

// Packets walks CoreScope's observed traffic, newest first, up to a limit.
//
// The raw frame is the whole point: the scope, the path and the payload type
// all live in those bytes, and a row without raw_hex can tell us nothing about
// a region. Rows that lack it are skipped rather than counted.
func (c *CoreScope) Packets(ctx context.Context, max int, progress func(int)) ([]PacketRecord, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("provider: corescope needs a BaseURL")
	}
	if max <= 0 {
		max = 5000
	}
	var out []PacketRecord
	for offset := 0; len(out) < max; {
		var rows []struct {
			RawHex       string   `json:"raw_hex"`
			ObserverID   string   `json:"observer_id"`
			ObserverName string   `json:"observer_name"`
			ResolvedPath []string `json:"resolved_path"`
			Origin       string   `json:"origin"`
		}
		url := fmt.Sprintf("%s/api/packets?limit=%d&offset=%d&sort=timestamp&order=desc",
			c.BaseURL, csPacketPageLimit, offset)
		if err := fetchJSON(ctx, c.HTTP, url, c.headers(), &rows); err != nil {
			return out, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if r.RawHex == "" {
				continue
			}
			raw, err := hex.DecodeString(r.RawHex)
			if err != nil {
				continue
			}
			rec := PacketRecord{Raw: raw, Receiver: r.ObserverName, Origin: r.Origin}
			// The last name on the resolved path is whoever transmitted the
			// copy that was heard — which is the node whose behaviour this
			// packet is evidence about.
			if n := len(r.ResolvedPath); n > 0 {
				rec.Sender = r.ResolvedPath[n-1]
			} else if r.Origin != "" {
				rec.Sender = r.Origin
			}
			out = append(out, rec)
		}
		if progress != nil {
			progress(len(out))
		}
		if len(rows) < csPacketPageLimit {
			break
		}
		offset += len(rows)
	}
	return out, nil
}

// Regions lists the region names CoreScope knows are configured.
//
// Candidates for identifying a scope. Without them a transport code cannot be
// turned into a name — the key is not in the packet, so a candidate is the only
// way to check one.
func (c *CoreScope) Regions(ctx context.Context) ([]string, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("provider: corescope needs a BaseURL")
	}
	var payload struct {
		Regions []struct {
			Name string `json:"name"`
		} `json:"regions"`
	}
	if err := fetchJSON(ctx, c.HTTP, c.BaseURL+"/api/regions", c.headers(), &payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Regions))
	for _, r := range payload.Regions {
		if r.Name != "" {
			out = append(out, r.Name)
		}
	}
	return out, nil
}
