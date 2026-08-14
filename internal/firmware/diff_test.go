package firmware_test

import (
	"context"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// A cross-check that reports "they matched" when neither side transmitted has
// found nothing, and reporting it as agreement is the exact failure ADR-0010's
// comparison exists to avoid: a green check that could never have gone red.
func TestSilentRunIsInconclusiveNotAgreement(t *testing.T) {
	a, b := pair(t)
	res, err := firmware.Diff(context.Background(), a, b, firmware.DiffOptions{
		UntilMs: 200, StepMs: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Divergences) != 0 {
		t.Fatalf("two silent nodes diverged: %v", res.Divergences)
	}
	if !res.Inconclusive {
		t.Error("a run where nothing was transmitted was not marked inconclusive")
	}
	if res.Agreed() {
		t.Error("Agreed() is true for a run that could not have found a difference")
	}
	if !strings.Contains(res.Describe(), "INCONCLUSIVE") {
		t.Errorf("the report does not say the run proves nothing:\n%s", res.Describe())
	}
}

func TestDiffNeedsBothBackends(t *testing.T) {
	a, _ := pair(t)
	if _, err := firmware.Diff(context.Background(), a, nil, firmware.DiffOptions{}); err == nil {
		t.Fatal("a diff with one node was accepted")
	}
}

// Both nodes must see exactly the same input. Delivering to one and not the
// other produces a divergence that says nothing about the target.
func TestBothNodesGetTheSameFrames(t *testing.T) {
	a, b := pair(t)
	var delivered int
	_, err := firmware.Diff(context.Background(), a, b, firmware.DiffOptions{
		UntilMs: 100, StepMs: 50,
		Deliver: func(atMs uint32) [][]byte {
			if atMs == 50 {
				delivered++
				return [][]byte{{0x12, 0x34}}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 1 {
		t.Errorf("Deliver was consulted %d times, want 1", delivered)
	}
}

// pair returns two native nodes, or skips when the binary is not built.
func pair(t *testing.T) (*firmware.Node, *firmware.Node) {
	t.Helper()
	if _, err := firmware.FindNative("", "simple_repeater"); err != nil {
		t.Skipf("no native node binary: %v", err)
	}
	ctx := context.Background()
	a, err := firmware.Start(ctx, "a", &firmware.Native{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := firmware.Start(ctx, "b", &firmware.Native{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	waitAttached(t, a)
	waitAttached(t, b)
	return a, b
}
