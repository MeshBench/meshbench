package firmware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

// Emulator is which one runs a node. It follows from the board's MCU rather
// than from a preference: QEMU has the Xtensa cores and Renode has the ARM
// ones, and neither will take the other's firmware.
type Emulator int

const (
	QEMU Emulator = iota
	Renode
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

	// Emulator selects QEMU or Renode.
	Emulator Emulator

	// Renode wiring. Platform is its base description; the rest is where the
	// radio hangs off and which pins carry chip select and DIO1. Chip select
	// matters more than it looks: Renode's SPI model never ends a transmission,
	// so without the pin the chip takes bytes for ever and executes no command.
	Platform string
	SPIBase  uint32
	NssPort  string
	NssPin   int
	IrqPort  string
	IrqPin   int

	// Dir is where the socket and logs live for this node.
	Dir string

	mu        sync.Mutex
	qemu      *exec.Cmd
	radio     *exec.Cmd
	sock      string
	radioPort int
}

// startRenode writes the machine description this node needs and runs it.
//
// Generated rather than kept as a file, because three of the values are
// per-node: the radio model's port, the node's own working directory, and the
// image. A shared script would need all three passed in anyway.
func (e *EmulatedNode) startRenode(ctx context.Context) error {
	renodeBin, err := lookupTool(EnvRenode, "renode")
	if err != nil {
		return err
	}
	tools := ToolsDir()
	script := filepath.Join(e.Dir, "node.resc")
	body := fmt.Sprintf(`i @%[1]s/peripherals/RadioServerSX1262.cs
i @%[1]s/peripherals/NRF52840_Temp.cs
i @%[1]s/peripherals/NRF52840_Clock.cs
i @%[1]s/peripherals/NRF52840_SAADC.cs
i @%[1]s/peripherals/NRF52840_TWIM.cs

mach create "%[2]s"
machine LoadPlatformDescription @%[3]s
machine LoadPlatformDescription @%[1]s/ficr.repl
machine LoadPlatformDescription @%[1]s/uicr.repl
machine LoadPlatformDescription @%[1]s/temp.repl
sysbus Unregister sysbus.clock
machine LoadPlatformDescription @%[1]s/clock.repl
machine LoadPlatformDescription @%[1]s/saadc.repl
sysbus Unregister sysbus.twi0
sysbus Unregister sysbus.twi1
machine LoadPlatformDescription @%[1]s/twim.repl

spi3: SPI.NRF52840_SPI @ sysbus 0x%[4]X
    easyDMA: true

lora: Radio.RadioServerSX1262 @ spi3
    host: "127.0.0.1"
    port: %[5]d
    IRQ -> %[6]s@%[7]d

%[8]s:
    %[9]d -> lora@0

sysbus LoadBinary @%[10]s 0x0
spi3.lora Connect
start
`, tools, e.NodeName, e.Platform, e.SPIBase, e.radioPort,
		e.IrqPort, e.IrqPin, e.NssPort, e.NssPin, e.Image)
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		return err
	}

	log, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		return err
	}
	e.qemu = exec.CommandContext(ctx, renodeBin,
		"--disable-xwt", "--console", "-e", "include @"+script)
	e.qemu.Stdout, e.qemu.Stderr = log, log
	if err := e.qemu.Start(); err != nil {
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	return nil
}

// waitForPort reads back the port the radio model chose.
//
// Asked for rather than assumed: a scenario starting several emulated nodes at
// once cannot pick ports itself without racing, so the model takes 0, binds
// whatever it gets, and prints it.
func waitForPort(ctx context.Context, logPath string) (int, error) {
	for i := 0; i < 200; i++ {
		if b, err := os.ReadFile(logPath); err == nil {
			if i := strings.Index(string(b), "127.0.0.1:"); i >= 0 {
				rest := string(b)[i+len("127.0.0.1:"):]
				if j := strings.IndexAny(rest, "\r\n"); j > 0 {
					if p, err := strconv.Atoi(strings.TrimSpace(rest[:j])); err == nil {
						return p, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("firmware: the radio model never said which port it took")
}

func (e *EmulatedNode) Kind() string { return "emulated" }

// Start brings up the radio model and then the emulator.
//
// Order matters: the device connects to the socket as it is realized, so a
// QEMU started first fails immediately with "cannot reach the radio model".
// The engine hands over its listener for this node; empty leaves the node deaf
// and mute, booting and then waiting for ever on a transmission that cannot
// complete, which looks like a hang rather than a missing argument.
func (e *EmulatedNode) Start(ctx context.Context, bridge string) error {
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
	// A Unix socket for QEMU on Linux and macOS; a TCP port for Renode
	// anywhere, and for either emulator on Windows.
	//
	// Renode has always used a port: it runs on Mono, whose Unix domain
	// socket support is not worth betting a node on. QEMU used a socket file,
	// which is the one thing Windows cannot give it - so emulated ESP32
	// boards stopped at the platform rather than at anything technical. The
	// device takes either now (it parses the string), and a Unix socket stays
	// the default where there is one: it needs no port and cannot collide.
	if e.Emulator == Renode || runtime.GOOS == "windows" {
		// Port 0 asks the radio model to choose, and it prints what it got.
		e.sock = ":0"
	} else {
		e.sock = filepath.Join(e.Dir, "radio.sock")
		_ = os.Remove(e.sock)
	}

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
	if bridge != "" {
		radioArgs = append(radioArgs, "--bridge", bridge)
	}
	e.radio = exec.CommandContext(ctx, radioBin, radioArgs...)
	e.radio.Stdout, e.radio.Stderr = radioLog, radioLog
	if err := e.radio.Start(); err != nil {
		return fmt.Errorf("firmware: starting the radio model: %w", err)
	}
	if e.sock == ":0" {
		port, err := waitForPort(ctx, filepath.Join(e.Dir, "radio.log"))
		if err != nil {
			_ = e.stopLocked()
			return err
		}
		e.radioPort = port
	} else if err := waitForSocket(ctx, e.sock); err != nil {
		_ = e.stopLocked()
		return err
	}

	if e.Emulator == Renode {
		if err := e.startRenode(ctx); err != nil {
			_ = e.stopLocked()
			return err
		}
		return nil
	}

	// What the device is told to connect to: the socket file, or the port the
	// radio model chose when there is no socket file to have.
	radioAt := e.sock
	if e.radioPort != 0 {
		radioAt = fmt.Sprintf("127.0.0.1:%d", e.radioPort)
	}
	machine := fmt.Sprintf("%s,radio-path=%s,radio-spi=%d,radio-nss=%d,radio-busy=%d",
		e.Machine, radioAt, e.SPI, e.NSS, e.Busy)

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

// ToolsDir is where the emulator and the radio model are kept.
//
// The same shape as the native build cache, and for the same reason: a desktop
// application is not launched from a shell, so it does not inherit one's PATH.
// Requiring an environment variable meant emulation worked from a terminal and
// failed from the desktop, with an error that read as a missing package.
func ToolsDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "tools"
	}
	return filepath.Join(dir, "meshcoresim", "tools")
}

// lookupTool finds a binary: the environment variable, then beside the
// simulator, then the tools directory, then PATH.
//
// The message names all of them, because "qemu-system-xtensa not found" sends
// people to their package manager for a build that will not do: ours carries an
// SX1262 and a GPIO implementation upstream has not got.
func lookupTool(env, name string) (string, error) {
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("firmware: %s points at %s, which is not there", env, p)
	}
	// The names a tool might be found under in a directory. Windows names its
	// executables, and a zip cannot carry the symlink the Linux tarball and
	// the macOS bundle use - so the emulator's own unpacked layout is
	// searched too, rather than requiring a link nobody could ship.
	candidates := []string{name}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, name+".exe")
	}
	subdirs := []string{"", "qemu/bin", "qemu-meshbench/bin"}
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		// Renode unpacks into a directory carrying its version, so the name
		// changes with every release and cannot be listed above. Globbing for
		// the shape is what the Linux tarball's symlink step already does;
		// this is the same rule on the side that has to find it.
		if matches, err := filepath.Glob(filepath.Join(dir, "renode*-portable")); err == nil {
			for _, m := range matches {
				subdirs = append(subdirs, filepath.Base(m))
			}
		}
		for _, sub := range subdirs {
			for _, cand := range candidates {
				if p := filepath.Join(dir, sub, cand); fileExists(p) {
					return p, nil
				}
			}
		}
	}
	for _, cand := range candidates {
		if p := filepath.Join(ToolsDir(), cand); fileExists(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("firmware: %s not found - looked beside the simulator, "+
		"in %s, and on PATH. Put it in that directory or set %s. A distribution "+
		"build will not do: ours carries the SX1262 device",
		name, ToolsDir(), env)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
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
