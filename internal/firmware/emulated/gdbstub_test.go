package emulated

import (
	"reflect"
	"strings"
	"testing"
)

// The GDB stub is asked for only when a port is given, on loopback for QEMU,
// and holding the CPU at reset only when asked to.
func TestTheGDBStubIsAskedForOnlyWhenWanted(t *testing.T) {
	if got := (&EmulatedNode{}).qemuGDBArgs(); got != nil {
		t.Errorf("no port, yet %v", got)
	}
	if got := (&EmulatedNode{}).renodeStart(); got != "start\n" {
		t.Errorf("no port, yet %q", got)
	}
	run := &EmulatedNode{GDBPort: 3333}
	if got := run.qemuGDBArgs(); !reflect.DeepEqual(got, []string{"-gdb", "tcp:127.0.0.1:3333"}) {
		t.Errorf("qemu: %v", got)
	}
	if got := run.renodeStart(); got != "machine StartGdbServer 3333\nstart\n" {
		t.Errorf("renode: %q", got)
	}
	held := &EmulatedNode{GDBPort: 3333, GDBWait: true}
	if got := held.qemuGDBArgs(); got[len(got)-1] != "-S" {
		t.Errorf("qemu held at reset: %v", got)
	}
	if got := held.renodeStart(); strings.Contains(got, "start\n") && !strings.Contains(got, "true") {
		t.Errorf("renode held at reset starts anyway: %q", got)
	}
}

func TestTheDebuggerFollowsTheArchitecture(t *testing.T) {
	for _, c := range []struct {
		n    *EmulatedNode
		want string
	}{
		{&EmulatedNode{GDBPort: 1, Emulator: Renode}, "arm-none-eabi-gdb"},
		{&EmulatedNode{GDBPort: 1, Machine: "esp32s3"}, "xtensa-esp32s3-elf-gdb"},
		{&EmulatedNode{GDBPort: 1, Machine: "esp32"}, "xtensa-esp32-elf-gdb"},
	} {
		if _, got := c.n.GDB(); got != c.want {
			t.Errorf("%s/%s: %s, want %s", c.n.Machine, c.want, got, c.want)
		}
	}
}

// In lockstep the machine's random numbers are seeded from the node and idle
// time is skipped, or the same tick would not run the same firmware (#774).
func TestALocksteppedRenodeScriptIsSeeded(t *testing.T) {
	t.Setenv(EnvRenodeLockstep, "1")
	a := (&EmulatedNode{NodeName: "a", RunSeed: 1}).renodeStart()
	b := (&EmulatedNode{NodeName: "b", RunSeed: 1}).renodeStart()
	for _, want := range []string{"emulation SetSeed ", "emulation SetAdvanceImmediately true\n", "start\n"} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in:\n%s", want, a)
		}
	}
	if a == b {
		t.Error("two nodes share one random stream")
	}
	if a != (&EmulatedNode{NodeName: "a", RunSeed: 1}).renodeStart() {
		t.Error("the same node is seeded differently twice")
	}
}
