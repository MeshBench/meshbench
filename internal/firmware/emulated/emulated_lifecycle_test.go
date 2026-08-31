package emulated

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// A second Start on a node already running must be refused, not allowed to
// overwrite e.qemu and e.radio: doing so orphans the first pair, which
// nothing holds a reference to any more and Stop can then never reach.
func TestStartRefusesASecondCallWhileQEMUIsRunning(t *testing.T) {
	e := &EmulatedNode{qemu: exec.Command("sleep", "1")}
	err := e.Start(context.Background(), "")
	if err == nil {
		t.Fatal("a second Start on a node already running was allowed")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("the error does not say the node is already running: %v", err)
	}
}

func TestStartRefusesASecondCallWhileTheRadioModelIsRunning(t *testing.T) {
	e := &EmulatedNode{radio: exec.Command("sleep", "1")}
	err := e.Start(context.Background(), "")
	if err == nil {
		t.Fatal("a second Start on a node already running was allowed")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("the error does not say the node is already running: %v", err)
	}
}
