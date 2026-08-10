package engine_test

import (
	"context"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/engine"
)

func TestBisectFindsTheSingleCulprit(t *testing.T) {
	all := []string{"a", "b", "c", "d", "e", "f", "g"}
	calls := 0
	got, err := engine.BisectNodes(context.Background(), all,
		func(_ context.Context, changed []string) (bool, error) {
			calls++
			for _, n := range changed {
				if n == "e" {
					return true, nil
				}
			}
			return false, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "e" {
		t.Fatalf("bisect returned %v, want [e]", got)
	}
	// O(log n) with at most two probes per level; 7 nodes needs well under 14.
	if calls > 8 {
		t.Fatalf("bisect used %d probes for 7 nodes", calls)
	}
}

func TestBisectAdmitsAConspiracy(t *testing.T) {
	// The divergence needs b AND f together: neither half alone reproduces it,
	// and bisect must widen its answer rather than pick a wrong single node.
	all := []string{"a", "b", "c", "d", "e", "f"}
	got, err := engine.BisectNodes(context.Background(), all,
		func(_ context.Context, changed []string) (bool, error) {
			hasB, hasF := false, false
			for _, n := range changed {
				hasB = hasB || n == "b"
				hasF = hasF || n == "f"
			}
			return hasB && hasF, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("bisect claimed a single culprit %v for a two-node effect", got)
	}
}
