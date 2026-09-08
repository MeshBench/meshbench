package main

import (
	"os"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// The assertion runner gave one answer on a fresh machine and another on
// every run after it, with an identical header: 350 deliveries, then 322,
// because the second run's nodes loaded what the first had stored. The
// number has to depend on the fixture, the build and the seed alone.
func TestATestRunGetsFreshNodeStorageAndRemovesIt(t *testing.T) {
	t.Setenv(firmware.EnvNodeFS, "/somewhere/the/user/keeps/nodes")
	words, cleanup, err := testNodeStorage(false)
	if err != nil {
		t.Fatal(err)
	}
	root := firmware.NodeFSRoot()
	if root == "/somewhere/the/user/keeps/nodes" {
		t.Fatal("the run is using the machine's node storage, which is the fault")
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		t.Fatalf("fresh storage %q is not a directory: %v", root, err)
	}
	if !strings.Contains(words, "fresh") {
		t.Errorf("the header must say the nodes are fresh, got %q", words)
	}
	cleanup()
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("fresh storage %q should be gone after the run", root)
	}
	if got := firmware.NodeFSRoot(); got != "/somewhere/the/user/keeps/nodes" {
		t.Errorf("cleanup must put the environment back, got %q", got)
	}
}

// Reusing stored state is a choice, and a run that makes it says where the
// state came from, so the number carries its own provenance.
func TestKeepingNodeStorageNamesIt(t *testing.T) {
	t.Setenv(firmware.EnvNodeFS, t.TempDir())
	words, cleanup, err := testNodeStorage(true)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !strings.Contains(words, "kept at "+firmware.NodeFSRoot()) {
		t.Errorf("the header must name the storage it kept, got %q", words)
	}
}
