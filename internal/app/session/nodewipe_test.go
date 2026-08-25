package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

// One board back to factory, and only that one.
//
// firmware.wipe has always been every node at once, which was the only
// granularity that made sense while an emulated node's flash was rewritten on
// every start. Now that a board keeps what it was told, this is the question
// somebody actually has.
func TestWipingOneNodeLeavesTheOthersAlone(t *testing.T) {
	t.Setenv(firmware.EnvNodeFS, t.TempDir())
	for _, n := range []string{"GB7XYZ", "GB7AAA"} {
		dir := firmware.NodeWorkDir(n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"flash.bin", "flash.bin.src", "card.img", "console.log"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A live socket is the emulator's, not the node's memory, and removing one
	// is removing something this verb was not asked about.
	sock := filepath.Join(firmware.NodeWorkDir("GB7XYZ"), "console.sock")
	if err := os.WriteFile(sock, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	st := state.New(10)
	s := &Sim{}
	registerNodeOutput(st, s)
	registerNodeWipe(st, s)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		w.Nodes = p.([]state.Node)
		return nil, nil
	})
	ctx := t.Context()
	go st.Run(ctx)
	if _, err := st.Do(ctx, "test.nodes", []state.Node{
		{Name: "GB7XYZ", Board: "Heltec_v3"}, {Name: "GB7AAA", Board: "Heltec_v3"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Do(ctx, "node.wipe", "GB7XYZ"); err != nil {
		t.Fatalf("node.wipe: %v", err)
	}
	if _, err := os.Stat(filepath.Join(firmware.NodeWorkDir("GB7XYZ"), "flash.bin")); err == nil {
		t.Error("the wiped node kept its flash")
	}
	if _, err := os.Stat(sock); err != nil {
		t.Error("the wipe removed a live socket, which is the emulator's and not the node's memory")
	}
	if _, err := os.Stat(filepath.Join(firmware.NodeWorkDir("GB7AAA"), "flash.bin")); err != nil {
		t.Error("wiping one node took another node's flash with it")
	}
	if _, err := st.Do(ctx, "node.wipe", "nobody"); err == nil {
		t.Error("node.wipe accepted a node that does not exist")
	}
}
