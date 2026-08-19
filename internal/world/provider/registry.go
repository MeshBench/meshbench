package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Capability declares what a provider can answer, per ADR-0016.
//
// The UI reads these instead of remembering that "corescope has packets but
// beacon does not" — knowledge that was hardcoded in a combo box and went
// stale each time a source grew a feature.
type Capability uint8

const (
	// CapNodes: positions and metadata.
	CapNodes Capability = 1 << iota
	// CapPackets: observed traffic with raw frames — what inference reads.
	CapPackets
	// CapRegions: the region names the deployment has configured.
	CapRegions
	// CapLive: a stream rather than a query.
	CapLive
)

// Has reports whether every capability in want is present.
func (c Capability) Has(want Capability) bool { return c&want == want }

// capable is what a provider that declares capabilities looks like.
type capable interface{ Capabilities() Capability }

// Checker is a provider that can say whether it is reachable before a caller
// commits to a fetch. An import that fails after twenty seconds of walking
// pages is a health check that should have run first.
type Checker interface {
	Health(ctx context.Context) error
}

// CapabilitiesOf reports what a provider declares, CapNodes for one that
// declares nothing — every Provider can list nodes by contract.
func CapabilitiesOf(p Provider) Capability {
	if c, ok := p.(capable); ok {
		return c.Capabilities()
	}
	if _, ok := p.(Live); ok {
		return CapNodes | CapLive
	}
	return CapNodes
}

// All returns every registered provider, sorted by name — the list the Import
// window enumerates instead of a hardcoded combo.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.ps))
	for _, n := range r.names() {
		out = append(out, r.ps[n])
	}
	return out
}

// Capabilities and Health on the concrete providers.

func (c *CoreScope) Capabilities() Capability { return CapNodes | CapPackets | CapRegions }

func (c *CoreScope) Health(ctx context.Context) error {
	return healthGet(ctx, c.HTTP, c.BaseURL+"/api/nodes", c.headers())
}

func (b *Beacon) Capabilities() Capability { return CapNodes }

func (b *Beacon) Health(ctx context.Context) error {
	return healthGet(ctx, b.HTTP, b.BaseURL+"/api/nodes", nil)
}

// healthGet is the cheap reachability probe: one GET with a short deadline,
// judged on status alone — the body is not read, because health asks "is it
// there and am I allowed in", not "what does it hold".
func healthGet(ctx context.Context, client Doer, url string, headers map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("reachable, but the token was refused (%s)", resp.Status)
	case resp.StatusCode >= 400:
		return fmt.Errorf("reachable, but answered %s", resp.Status)
	}
	return nil
}
