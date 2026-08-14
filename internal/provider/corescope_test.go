package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MeshBench/meshbench/internal/provider"
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
