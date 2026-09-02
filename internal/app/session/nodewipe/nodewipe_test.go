package nodewipe_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/nodewipe"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// One board back to factory, and only that one.
//
// firmware.wipe has always been every node at once, which was the only
// granularity that made sense while an emulated node's flash was rewritten on
// every start. Now that a board keeps what it was told, this is the question
// somebody actually has. Pinned here because node.wipe moved out of session.
func TestWipingOneNodeLeavesTheOthersAlone(t *testing.T) {
	t.Setenv(firmware.EnvNodeFS, nodeFSRoot(t))
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
	s := &session.Sim{}
	session.Register(st, s)
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

// aWipeableNode is one node with files in its directory, on a store with the
// whole verb set.
func aWipeableNode(t *testing.T, files ...string) (*state.Store, string) {
	t.Helper()
	t.Setenv(firmware.EnvNodeFS, nodeFSRoot(t))
	dir := firmware.NodeWorkDir("GB7XYZ")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := state.New(10)
	session.Register(st, &session.Sim{})
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		w.Nodes = p.([]state.Node)
		return nil, nil
	})
	go st.Run(t.Context())
	if _, err := st.Do(t.Context(), "test.nodes", []state.Node{
		{Name: "GB7XYZ", Board: "Heltec_v3"},
	}); err != nil {
		t.Fatal(err)
	}
	return st, dir
}

// confirm:false used to be read by nothing at all, so a caller asking to look
// before leaping had the node wiped and was told it had been wiped.
func TestWipeWithoutConfirmationRemovesNothingAndSaysWhatWouldGo(t *testing.T) {
	st, dir := aWipeableNode(t, "flash.bin", "card.img")

	got, err := st.Do(t.Context(), "node.wipe",
		map[string]any{"node": "GB7XYZ", "confirm": false})
	if err != nil {
		t.Fatalf("node.wipe: %v", err)
	}
	m := got.(map[string]any)
	if m["wiped"] != 0 {
		t.Errorf("a dry run reported %v wiped", m["wiped"])
	}
	would, _ := m["would_remove"].([]string)
	if len(would) != 2 {
		t.Errorf("it would remove %v, want both files named", would)
	}
	for _, f := range []string{"flash.bin", "card.img"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("a dry run removed %s", f)
		}
	}
}

// A file that could not be removed used to be dropped from the result, so a
// node that still holds its settings answered "wiped" and booted next time
// into the state the operator had been told was gone.
func TestAPartialWipeIsReportedAsPartial(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes files out of an unwritable directory")
	}
	st, dir := aWipeableNode(t, "flash.bin")
	// A subdirectory whose contents cannot be unlinked, which is what a
	// RemoveAll failure looks like on a real machine.
	stuck := filepath.Join(dir, "settings")
	if err := os.MkdirAll(stuck, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "prefs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	blockRemoval(t, filepath.Join(stuck, "prefs"))

	_, err := st.Do(t.Context(), "node.wipe", "GB7XYZ")
	if err == nil {
		t.Fatal("a wipe that left files behind reported success")
	}
	for _, want := range []string{"GB7XYZ", "partly wiped", "settings"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not mention %q", err, want)
		}
	}
}
