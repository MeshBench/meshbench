package engine_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// runLogged runs a small scenario with the event log on and returns the bytes.
func runLogged(t *testing.T, seed uint64) []byte {
	t.Helper()
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: seed})
	defer func() { _ = e.Close() }()
	e.Add(node("a", 56.700, -3.900, 22), nil)
	e.Add(node("b", 56.705, -3.905, 22), nil)

	path := filepath.Join(t.TempDir(), "events.ndjson")
	if err := e.StartEventLog(path); err != nil {
		t.Fatal(err)
	}
	e.Inject(0, []byte("hello"))
	if err := e.Run(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.StopEventLog(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The two ADR-0007 claims worth a test: every line is valid JSON with the
// stable field names, and the log is deterministic per seed — which is what
// makes diffing two runs a regression test.
func TestEventLogIsValidNDJSONAndDeterministic(t *testing.T) {
	a := runLogged(t, 4417)
	if len(a) == 0 {
		t.Fatal("nothing logged")
	}
	sc := bufio.NewScanner(bytes.NewReader(a))
	lines := 0
	for sc.Scan() {
		var row map[string]any
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatalf("line %d is not JSON: %v", lines+1, err)
		}
		if _, ok := row["t_ms"]; !ok {
			t.Fatalf("line %d has no t_ms: %s", lines+1, sc.Text())
		}
		if _, ok := row["kind"]; !ok {
			t.Fatalf("line %d has no kind: %s", lines+1, sc.Text())
		}
		lines++
	}
	if lines == 0 {
		t.Fatal("no lines")
	}

	if b := runLogged(t, 4417); !bytes.Equal(a, b) {
		t.Fatal("same seed, different logs - the log cannot be used for diffing")
	}
}

// The calibration term must actually reach the channel: the same pair, with
// excess loss configured, loses exactly that much more.
func TestExcessPathLossIsApplied(t *testing.T) {
	base := engine.New(flat{100}, engine.Config{StepMs: 10})
	defer func() { _ = base.Close() }()
	base.Add(node("a", 56.700, -3.900, 22), nil)
	base.Add(node("b", 56.705, -3.905, 22), nil)
	l0, ok := base.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("no path")
	}

	cal := engine.New(flat{100}, engine.Config{StepMs: 10, ExcessPathLossDB: 4.2})
	defer func() { _ = cal.Close() }()
	cal.Add(node("a", 56.700, -3.900, 22), nil)
	cal.Add(node("b", 56.705, -3.905, 22), nil)
	l1, ok := cal.PathLossForTest(0, 1)
	if !ok {
		t.Fatal("no path")
	}
	if diff := l1 - l0; diff < 4.19 || diff > 4.21 {
		t.Fatalf("excess loss changed the path by %.2f dB, want 4.2", diff)
	}
}
