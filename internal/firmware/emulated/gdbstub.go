package emulated

import (
	"fmt"
	"os"
)

// qemuGDBArgs starts QEMU's GDB stub, on loopback only: a stub is the whole
// machine, memory and registers, to anything that reaches the port.
func (e *EmulatedNode) qemuGDBArgs() []string {
	if e.GDBPort <= 0 {
		return nil
	}
	args := []string{"-gdb", fmt.Sprintf("tcp:127.0.0.1:%d", e.GDBPort)}
	if e.GDBWait {
		// The CPU does not start until a debugger says continue.
		args = append(args, "-S")
	}
	return args
}

// renodeStart is the end of the node's script: the GDB stub when one is
// asked for, and starting the machine. Renode's stub listens on every
// interface, which is why only a run on this machine asks for it. Held at
// reset, the machine is not started here at all: the stub's autostart starts
// it when a debugger connects and continues.
func (e *EmulatedNode) renodeStart() string {
	if os.Getenv(EnvRenodeLockstep) == "1" {
		// Idle time skipped rather than waited out on the host clock: in
		// lockstep the engine is the clock, and a sleeping CPU waiting for
		// wall time would hold every tick to real time. And Renode's own
		// random numbers - what the nRF52's RNG peripheral answers, which
		// MeshCore draws its jitter from - seeded from the node rather than
		// afresh each run.
		seed := noiseSeedFor(e.RunSeed, e.NodeName, e.BoardName, e.Position)
		return fmt.Sprintf("emulation SetSeed %d\nemulation SetAdvanceImmediately true\n%s",
			int32(seed), e.renodeStartFree())
	}
	return e.renodeStartFree()
}

func (e *EmulatedNode) renodeStartFree() string {
	if e.GDBPort <= 0 {
		return "start\n"
	}
	if e.GDBWait {
		return fmt.Sprintf("machine StartGdbServer %d true\n", e.GDBPort)
	}
	return fmt.Sprintf("machine StartGdbServer %d\nstart\n", e.GDBPort)
}

// GDB is how a debugger reaches this node: the port, and the debugger that
// speaks the board's architecture. Zero when no stub was asked for.
func (e *EmulatedNode) GDB() (port int, debugger string) {
	if e.GDBPort <= 0 {
		return 0, ""
	}
	if e.Emulator == Renode {
		return e.GDBPort, "arm-none-eabi-gdb"
	}
	if e.Machine == "esp32s3" {
		return e.GDBPort, "xtensa-esp32s3-elf-gdb"
	}
	return e.GDBPort, "xtensa-esp32-elf-gdb"
}
