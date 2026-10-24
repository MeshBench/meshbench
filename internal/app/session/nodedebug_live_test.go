package session

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A native node being debugged can be stopped, as a breakpoint stops it, and
// the run goes on without it; let go, it rejoins rather than being marked
// down (#191).
func TestLiveADebuggedNativeNodeSurvivesABreakpoint(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	go st.Run(ctx)
	a, b := repeaterNode("A"), repeaterNode("B")
	b.Position.Lon += 0.001
	s.build([]scenario.Node{a, b}, 869.618)
	if err := s.eng.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := st.Do(ctx, "node.debug", map[string]any{"node": "A"})
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	pid, _ := m["pid"].(int)
	if pid <= 0 || m["binary"] == "" {
		t.Fatalf("node.debug answered %v", m)
	}
	t.Logf("attach: %v (symbols: %v)", m["attach"], m["symbols"])

	// What a breakpoint does to the process.
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	before := s.eng.NowMs()
	if err := s.eng.Run(ctx, before+5000); err != nil {
		t.Fatal(err)
	}
	if _, held := s.eng.Held("A"); !held {
		t.Fatal("a stopped node being debugged was not held")
	}
	if s.eng.NowMs() < before+5000 {
		t.Fatalf("the run stopped with the node: %d", s.eng.NowMs())
	}

	if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := s.eng.Run(ctx, s.eng.NowMs()+2000); err != nil {
		t.Fatal(err)
	}
	if _, held := s.eng.Held("A"); held {
		t.Error("let go, the node is still held")
	}
	if down := s.eng.FirmwareFailures(); len(down) != 0 {
		t.Errorf("marked down: %+v", down)
	}
	if s.notReproducible() == "" {
		t.Error("a run with a node being debugged still claims it can be repeated")
	}
}
