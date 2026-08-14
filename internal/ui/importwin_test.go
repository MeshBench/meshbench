package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/scenario"
)

func importTestOpts() scenario.ImportOptions {
	return scenario.ImportOptions{
		DefaultBoard: "RAK4631",
		Radio: scenario.RadioConfig{
			CentreHz: 869.525e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
		},
		MaxUncertaintyKm: 1,
	}
}

// TestFetchImportPreviewsThenMerges is the Phase 2 contract end to end: a
// fetch produces a preview without touching anything, and each merge strategy
// lands it the way its label says.
func TestFetchImportPreviewsThenMerges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"nodes":[
			{"name":"ben-vrackie","public_key":"AA11","role":"repeater","lat":56.75,"lon":-3.72},
			{"name":"dunkeld","public_key":"BB22","role":"repeater","lat":56.56,"lon":-3.58}
		]}`)
	}))
	defer srv.Close()

	out := fetchImport(context.Background(), "corescope", srv.URL, "", importTestOpts(), nil)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if len(out.nodes) != 2 {
		t.Fatalf("preview holds %d nodes", len(out.nodes))
	}
	if out.nodes[0].PublicKey != "aa11" {
		t.Fatalf("public key did not survive the import: %+v", out.nodes[0])
	}

	// Merging onto a scenario that already has ben-vrackie under a different
	// name but the same key: add-only-new must leave it alone and add dunkeld.
	existing := []scenario.Node{{Name: "bv-old", PublicKey: "aa11"}, {Name: "hand-placed"}}
	plan := scenario.PlanMerge(existing, out.nodes, scenario.MergeAddNew)
	if plan.Add != 1 || plan.Keep != 2 {
		t.Fatalf("plan = %+v", plan)
	}
	merged := scenario.Merge(existing, out.nodes, scenario.MergeAddNew)
	if len(merged) != 3 || merged[0].Name != "bv-old" {
		t.Fatalf("merged = %+v", merged)
	}
}

// TestFetchImportReadsSavedNetworkFiles: the file source takes a saved network
// as-is — no import pipeline, no assumptions re-made.
func TestFetchImportReadsSavedNetworkFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "net.json")
	body := `[{"Name":"alpha","PublicKey":"aa11","Position":{"Lat":56.7,"Lon":-3.7}}]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := fetchImport(context.Background(), "file", path, "", importTestOpts(), nil)
	if out.err != nil {
		t.Fatal(out.err)
	}
	if len(out.nodes) != 1 || out.nodes[0].Name != "alpha" {
		t.Fatalf("nodes = %+v", out.nodes)
	}
}
