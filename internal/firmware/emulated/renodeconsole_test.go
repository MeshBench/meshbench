package emulated_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
)

// armfwImage is the SoftDevice-free image tools/armfw builds, and the only
// nRF52 image here whose console is on the UART. The published ones are built
// against the Adafruit core, whose Serial is a USB CDC device; point
// MESHBENCH_IMAGE at one of those .uf2 files and this boots it through
// NRF52840_USBD instead, which is the harder half of the same question.
const armfwImage = "../../../tools/armfw/build/fw.bin"

// A board under Renode used to have a write-only console: its serial output
// went to a file and nothing could be typed back at it, so everything the
// workbench does to a node by console was inert against an nRF52 board. This
// boots one for real and asks it a question.
//
// Live because it needs the emulator and the radio model; and one board at a
// time, because an emulated node carries a whole emulator process.
func TestLiveRenodeBoardAnswersItsConsole(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	image := os.Getenv("MESHBENCH_IMAGE")
	if image == "" {
		image = armfwImage
	}
	if _, err := os.Stat(image); err != nil {
		t.Skipf("no image to boot: build one with tools/armfw/build.sh")
	}
	if _, err := emulated.FindTool("renode"); err != nil {
		t.Skipf("no emulator: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// The engine's end of the bridge, so the radio model has somewhere to put
	// what the board transmits. Without one the firmware waits for ever on a
	// transmission that cannot complete, which reads as a board that hung.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	// A published image is a .uf2 linked above a SoftDevice, and its console is
	// USB; the bare-metal one is a flat binary that prints on the UART. The two
	// say different things on the way up, so what counts as an answer differs
	// with them.
	published := strings.HasSuffix(strings.ToLower(image), ".uf2")
	banner, answer := "ready", "-> MSIM"
	if published {
		banner, answer = "Repeater ID:", "-> v"
	}

	n := &emulated.EmulatedNode{
		Emulator: emulated.Renode,
		Image:    image,
		NodeName: "renode-console",
		Dir:      filepath.Join(t.TempDir(), "node"),
		// The RAK_4631's wiring, from its own profile: the radio is on SPIM3,
		// with chip select and DIO1 on the second GPIO port.
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio1", NssPin: 10,
		IrqPort: "gpio1", IrqPin: 15,
		ConsoleOnUSB:  published,
		SoftDeviceDir: emulated.SoftDeviceDir(),
	}
	if err := n.Start(ctx, ln.Addr().String()); err != nil {
		t.Fatalf("starting the board: %v", err)
	}
	defer func() { _ = n.Stop() }()
	if !n.HasConsole() {
		t.Fatal("a board under Renode reports no console")
	}
	go tickForever(ctx, ln)

	// The board's own output first: a console that answers nothing may be a
	// console that works and a board that has not booted, and those need
	// different reports.
	if !waitForConsole(ctx, t, n, banner, 5*time.Minute) {
		t.Fatal("the board never printed anything on its console")
	}
	if _, err := n.ConsoleIn().Write([]byte("ver\r\n")); err != nil {
		t.Fatalf("typing at the board: %v", err)
	}
	if !waitForConsole(ctx, t, n, answer, 2*time.Minute) {
		t.Fatal("the board did not answer a typed command")
	}
	// The whole file, not the answer alone: the log has to carry the boot from
	// the first character as well, because that is what it is read for
	// afterwards and a socket that started late would not show it here.
	b, _ := n.ConsoleLog()
	t.Logf("console.log:\n%s", string(b))
}

// tickForever paces the node to wall time, as the engine does: an emulated
// board runs on its own clock, and a bridge nobody is answering stalls it.
func tickForever(ctx context.Context, ln net.Listener) {
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	go func() { _, _ = io.Copy(io.Discard, conn) }()
	for ms := uint32(100); ctx.Err() == nil; ms += 100 {
		var p [4]byte
		binary.BigEndian.PutUint32(p[:], ms)
		if _, err := conn.Write(append([]byte{0x02, 0, 4}, p[:]...)); err != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitForConsole(ctx context.Context, t *testing.T, n *emulated.EmulatedNode,
	want string, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if b, err := n.ConsoleLog(); err == nil && strings.Contains(string(b), want) {
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

// The console terminal is what the board's own output and input hang off, so a
// script without it is a board nothing can be asked.
func TestTheRenodeScriptCarriesAConsole(t *testing.T) {
	got := renode.ConsoleTerminal(41234, false)
	for _, want := range []string{
		"CreateServerSocketTerminal 41234",
		"connector Connect sysbus.uart0 console",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the console fragment is missing %q:\n%s", want, got)
		}
	}
	// Telnet negotiation bytes are not something the board said, and the
	// console log cannot tell the difference.
	if !strings.Contains(got, "\"console\" false") {
		t.Errorf("the terminal was not asked to stay quiet:\n%s", got)
	}
}

// A published MeshCore image prints over USB, not on the UART, so its terminal
// has to hang off the USB device controller. Connecting it to uart0 instead
// publishes a port that answers nothing - and on a RAK4631 that port is the
// receiver's, so it would not even be quiet.
func TestABoardPrintingOverUSBGetsItsTerminalThere(t *testing.T) {
	got := renode.ConsoleTerminal(41234, true)
	if !strings.Contains(got, "connector Connect sysbus.usbd console") {
		t.Errorf("the console was not put on the USB controller:\n%s", got)
	}
	if strings.Contains(got, "uart0") {
		t.Errorf("the console was put on the UART as well:\n%s", got)
	}
}

// Port zero is a board this machine cannot publish a console for at all, and it
// gets no terminal rather than one nothing answers on.
func TestABoardWithNoReachableConsoleGetsNoTerminal(t *testing.T) {
	if got := renode.ConsoleTerminal(0, false); got != "" {
		t.Errorf("wanted no fragment, got:\n%s", got)
	}
	if got := renode.ConsoleTerminal(0, true); got != "" {
		t.Errorf("wanted no fragment, got:\n%s", got)
	}
}
