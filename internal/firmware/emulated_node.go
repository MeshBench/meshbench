package firmware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// EnvQEMU overrides the emulator binary, and EnvRadioServer the radio model.
//
// Both are ours rather than distribution packages: the emulator carries an
// SX1262 device and a GPIO implementation that upstream does not have, and the
// radio model is the same one native nodes run against. A released build ships
// both, and until then these say where to find them.
const (
	EnvQEMU        = "MESHCORESIM_QEMU"
	EnvRadioServer = "MESHCORESIM_RADIO_SERVER"
)

// EmulatedNode is one node running a published board image under QEMU.
//
// Two processes rather than one: the emulator, and the radio model it talks to
// over a socket. Keeping the model in its own process is what lets an emulated
// node and a native node share the same chip implementation - the alternative
// was a second model inside the emulator, and two models that must agree
// eventually do not.
type EmulatedNode struct {
	// Image is the flash image to boot: a merged .bin, which carries the
	// bootloader and partition table as well as the application. A bare
	// application image boots nothing.
	Image string

	// Wiring is where the radio sits on this board. Wrong values do not fail
	// loudly - they produce a driver that reports no chip, which reads as a
	// broken emulator.
	Machine  string
	SPI      int
	NSS      int
	Busy     int
	FlashMB  int
	NodeName string

	// Bridge is the engine's listener for this node, host:port. Empty leaves
	// the node deaf and mute: it will boot and initialise its radio and then
	// wait for ever on a transmission that cannot complete, which looks like a
	// hang rather than like a missing argument.
	Bridge string

	// Dir is where the socket and logs live for this node.
	Dir string

	mu    sync.Mutex
	qemu  *exec.Cmd
	radio *exec.Cmd
	sock  string
}

func (e *EmulatedNode) Kind() string { return "emulated" }

// Start brings up the radio model and then the emulator.
//
// Order matters: the device connects to the socket as it is realized, so a
// QEMU started first fails immediately with "cannot reach the radio model".
func (e *EmulatedNode) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.Image == "" {
		return fmt.Errorf("firmware: an emulated node needs a flash image")
	}
	if e.Machine == "" {
		e.Machine = "esp32"
	}
	if e.Dir == "" {
		e.Dir = filepath.Join(os.TempDir(), "meshcoresim-emulated", e.NodeName)
	}
	if err := os.MkdirAll(e.Dir, 0o755); err != nil {
		return err
	}
	e.sock = filepath.Join(e.Dir, "radio.sock")
	_ = os.Remove(e.sock)

	radioBin, err := lookupTool(EnvRadioServer, "radioserver")
	if err != nil {
		return err
	}
	qemuBin, err := lookupTool(EnvQEMU, "qemu-system-xtensa")
	if err != nil {
		return err
	}

	radioLog, err := os.Create(filepath.Join(e.Dir, "radio.log"))
	if err != nil {
		return err
	}
	radioArgs := []string{e.sock}
	if e.Bridge != "" {
		radioArgs = append(radioArgs, "--bridge", e.Bridge)
	}
	e.radio = exec.CommandContext(ctx, radioBin, radioArgs...)
	e.radio.Stdout, e.radio.Stderr = radioLog, radioLog
	if err := e.radio.Start(); err != nil {
		return fmt.Errorf("firmware: starting the radio model: %w", err)
	}
	if err := waitForSocket(ctx, e.sock); err != nil {
		_ = e.stopLocked()
		return err
	}

	machine := fmt.Sprintf("%s,radio-path=%s,radio-spi=%d,radio-nss=%d,radio-busy=%d",
		e.Machine, e.sock, e.SPI, e.NSS, e.Busy)

	qemuLog, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		_ = e.stopLocked()
		return err
	}
	// Serial to a file rather than to us: it carries the whole boot chain, ROM
	// through the application, and that is where an emulated node fails when it
	// fails. The node window reads this back.
	e.qemu = exec.CommandContext(ctx, qemuBin,
		"-machine", machine,
		"-nographic", "-monitor", "none",
		"-drive", "file="+e.Image+",if=mtd,format=raw",
		"-serial", "file:"+filepath.Join(e.Dir, "console.log"),
	)
	e.qemu.Stdout, e.qemu.Stderr = qemuLog, qemuLog
	if err := e.qemu.Start(); err != nil {
		_ = e.stopLocked()
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	return nil
}

// ConsolePath is where this node's boot output is written.
func (e *EmulatedNode) ConsolePath() string {
	return filepath.Join(e.Dir, "console.log")
}

// Stop ends both processes.
func (e *EmulatedNode) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopLocked()
}

func (e *EmulatedNode) stopLocked() error {
	for _, c := range []*exec.Cmd{e.qemu, e.radio} {
		if c == nil || c.Process == nil {
			continue
		}
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	}
	e.qemu, e.radio = nil, nil
	_ = os.Remove(e.sock)
	return nil
}

// lookupTool finds a binary by environment variable, then on PATH.
//
// The message names both, because "qemu-system-xtensa not found" sends people
// to their package manager for a build that will not do: ours carries an SX1262
// and a GPIO implementation upstream has not got.
func lookupTool(env, name string) (string, error) {
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("firmware: %s points at %s, which is not there", env, p)
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("firmware: %s not found - set %s, or build it from "+
			"the meshbench-sx1262 branch (a distribution build will not do, ours "+
			"carries the SX1262 device)", name, env)
	}
	return p, nil
}

// PadImage copies a flash image padded to a size QEMU will accept.
//
// Two traps in one function. QEMU takes only 2, 4, 8 or 16 MB images; and the
// size must match what the image header asks for, or ESP-IDF asserts in
// do_core_init with a message naming both sizes. The header lives at 0x1000 in
// a merged image, because the file starts with padding - reading it from zero
// gives 0xff and a nonsense answer.
func PadImage(src, dst string) (int, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	if len(data) < 0x1004 {
		return 0, fmt.Errorf("firmware: %s is too small to be a merged image", src)
	}
	if data[0x1000] != 0xE9 {
		return 0, fmt.Errorf("firmware: %s has no image header at 0x1000; "+
			"it is probably an application-only build rather than a merged one", src)
	}
	sizes := map[byte]int{0: 1, 1: 2, 2: 4, 3: 8, 4: 16}
	mb, ok := sizes[data[0x1003]>>4]
	if !ok {
		return 0, fmt.Errorf("firmware: %s declares an unknown flash size", src)
	}
	if mb == 1 {
		mb = 2 // QEMU's smallest
	}
	want := mb << 20
	if len(data) > want {
		return 0, fmt.Errorf("firmware: %s is larger than the %d MB its header declares",
			src, mb)
	}
	out := make([]byte, want)
	copy(out, data)
	for i := len(data); i < want; i++ {
		out[i] = 0xFF // erased flash
	}
	return mb, os.WriteFile(dst, out, 0o644)
}

// qemuMachineArg is exported for the tests and for anything that wants to show
// an operator the command that was run.
func qemuMachineArg(machine, sock string, spi, nss, busy int) string {
	return machine +
		",radio-path=" + sock +
		",radio-spi=" + strconv.Itoa(spi) +
		",radio-nss=" + strconv.Itoa(nss) +
		",radio-busy=" + strconv.Itoa(busy)
}

// waitForSocket blocks until the radio model is listening, or the context ends.
//
// Polled rather than assumed: the device connects to this socket as QEMU
// realizes it, and a race there fails the whole boot with a message about the
// radio being unreachable, which points at configuration rather than at timing.
func waitForSocket(ctx context.Context, path string) error {
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return fmt.Errorf("firmware: the radio model never opened %s", path)
}
