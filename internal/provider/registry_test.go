package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCapabilitiesDeclared(t *testing.T) {
	cs := &CoreScope{BaseURL: "http://x"}
	if got := CapabilitiesOf(cs); !got.Has(CapNodes | CapPackets | CapRegions) {
		t.Fatalf("corescope capabilities = %b", got)
	}
	b := &Beacon{BaseURL: "http://x"}
	if got := CapabilitiesOf(b); !got.Has(CapNodes) || got.Has(CapPackets) {
		t.Fatalf("beacon capabilities = %b", got)
	}
}

func TestRegistryAllIsSorted(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&CoreScope{BaseURL: "http://x"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&Beacon{BaseURL: "http://y"}); err != nil {
		t.Fatal(err)
	}
	all := r.All()
	if len(all) != 2 || all[0].Name() != "beacon" || all[1].Name() != "corescope" {
		t.Fatalf("All() = %v", all)
	}
}

func TestHealthDistinguishesRefusedFromUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer srv.Close()

	ok := &CoreScope{BaseURL: srv.URL, Token: "t"}
	if err := ok.Health(context.Background()); err != nil {
		t.Fatalf("healthy source reported: %v", err)
	}
	bad := &CoreScope{BaseURL: srv.URL}
	err := bad.Health(context.Background())
	if err == nil {
		t.Fatal("unauthenticated source reported healthy")
	}
	down := &CoreScope{BaseURL: "http://127.0.0.1:1", Token: "t"}
	if err := down.Health(context.Background()); err == nil {
		t.Fatal("unreachable source reported healthy")
	}
}
