package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/provider"
)

// The live-instance failure this holds down: CoreScope answers
// {"packets":[...]} while the code decoded a bare array, and the operator saw
// "cannot unmarshal object into Go value of type []struct" with no hint which
// side was wrong. Both shapes must work.
func TestPacketsAcceptsBothPayloadShapes(t *testing.T) {
	const row = `{"raw_hex":"110200aabb","observer_name":"obs","origin":"alpha",` +
		`"_parsedPath":["7DEC4F","30DB06"]}`
	for name, body := range map[string]string{
		"wrapped in an object": `{"packets":[` + row + `]}`,
		"a bare array":         `[` + row + `]`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("offset") != "0" {
					_, _ = io.WriteString(w, `{"packets":[]}`)
					return
				}
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()

			cs := &provider.CoreScope{BaseURL: srv.URL}
			got, err := cs.Packets(context.Background(), 10, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d packets", len(got))
			}
			if got[0].Origin != "alpha" || got[0].Receiver != "obs" {
				t.Fatalf("row = %+v", got[0])
			}
			// Two path entries means two hops, which is what keeps this copy
			// out of the live feed's first-hop-only window.
			if len(got[0].PathHashes) != 2 {
				t.Fatalf("path hashes = %v", got[0].PathHashes)
			}
		})
	}
}

// The sender of a heard copy, resolved from whichever field carries it.
//
// This chain is what turned a calibration run's 0 matched pairs into 851: the
// last path entry is whoever transmitted a relayed copy, and a copy with no
// path at all was heard straight from its origin - whose key, for an advert,
// is in the payload CoreScope has already decoded. A live ScotMesh page
// carries 850 such direct receptions with SNR, and every one resolved to
// nobody before this.
func TestPacketSenderIsWhoTransmittedTheCopy(t *testing.T) {
	serve := func(t *testing.T, row string) provider.PacketRecord {
		t.Helper()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("offset") != "0" {
				_, _ = io.WriteString(w, `{"packets":[]}`)
				return
			}
			_, _ = io.WriteString(w, `{"packets":[`+row+`]}`)
		}))
		defer srv.Close()
		cs := &provider.CoreScope{BaseURL: srv.URL}
		got, err := cs.Packets(context.Background(), 10, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d packets", len(got))
		}
		return got[0]
	}

	t.Run("a relayed copy's sender is the last path entry", func(t *testing.T) {
		rec := serve(t, `{"raw_hex":"110200aabb","observer_name":"obs",`+
			`"resolved_path":["aaaa1111","bbbb2222"],"hash":"h-42"}`)
		if rec.Sender != "bbbb2222" {
			t.Fatalf("sender = %q, want the last relay", rec.Sender)
		}
		if rec.PacketID != "h-42" {
			t.Fatalf("packet id = %q, want the row's hash", rec.PacketID)
		}
	})

	t.Run("a direct advert's sender is the key in its own payload", func(t *testing.T) {
		// decoded_json as CoreScope actually sends it on a live instance: a
		// JSON-encoded *string*, and the key under pubKey, not public_key.
		rec := serve(t, `{"raw_hex":"110200aabb","observer_name":"obs",`+
			`"decoded_json":"{\"type\":\"advert\",\"pubKey\":\"04f1fe66cafe\"}"}`)
		if rec.Sender != "04f1fe66cafe" {
			t.Fatalf("sender = %q, want the advert's own key", rec.Sender)
		}
	})

	t.Run("an object-shaped decode works too", func(t *testing.T) {
		rec := serve(t, `{"raw_hex":"110200aabb","observer_name":"obs",`+
			`"decoded_json":{"type":"advert","public_key":"30db06d2beef"}}`)
		if rec.Sender != "30db06d2beef" {
			t.Fatalf("sender = %q", rec.Sender)
		}
	})

	t.Run("an origin field still wins over the payload", func(t *testing.T) {
		rec := serve(t, `{"raw_hex":"110200aabb","observer_name":"obs",`+
			`"origin":"alpha","decoded_json":"{\"pubKey\":\"ffff\"}"}`)
		if rec.Sender != "alpha" {
			t.Fatalf("sender = %q, want the deployment's own origin", rec.Sender)
		}
	})
}
