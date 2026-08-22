package firmware

import (
	"context"
	"fmt"
	"io"
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
	Machine string
	SPI     int
	NSS     int
	Busy    int
	DIO1    int

	// FEM is the GPIO the firmware drives as the front-end module's transmit
	// enable, or zero on a board with no module. Zero is safe as "none": GPIO 0
	// is a strapping pin on these parts and no board routes a module to it.
	FEM      int
	NodeName string

	// PSRAMMB is the board's external RAM, in megabytes. Zero means none, and
	// a firmware built expecting some will not start without it.
	PSRAMMB int

	// Panel is the board's display, where it has one, and where somebody is
	// listening for its picture. Empty leaves the machine without a display -
	// which is what a board with none looks like, and what a board whose
	// screen nobody is watching should look like too.
	//
	// The controller decides the column offset: an SH1106 is an SSD1306 whose
	// columns start two to the right, and a picture drawn with the wrong one
	// slides sideways rather than failing.
	PanelPath   string
	PanelAddr   uint8
	PanelOffset int
	// PanelCS and PanelDC put the display on the radio's SPI controller
	// instead of on I2C: its own chip select, and the line that says whether
	// a byte is a command or data. Zero leaves it on I2C.
	PanelCS, PanelDC     int
	PanelWidth, PanelHgt int

	// Buttons is where presses leave from, and the pins they may move. Empty
	// leaves the board's buttons where their pull-ups put them, which is
	// nobody pressing them.
	ButtonPath string
	ButtonPins []int
	Buttons    *ButtonSender

	// Panel is where this node's pictures arrive, when something is listening.
	// Held here so a caller with the node has the screen too, rather than
	// having to keep the two in step itself.
	Panel *PanelListener

	// PSRAMOctal selects an octal (OPI) part rather than a quad one.
	PSRAMOctal bool

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

	// SoftDeviceDir is the cache the Nordic SoftDevice was fetched into.
	//
	// A directory rather than a file because which version is needed follows
	// from the image, and only this package reads that: an application based at
	// 0x26000 pairs with s140 v6.1.1 and one at 0x27000 with v7.x.
	SoftDeviceDir string

	// Dir is where the socket and logs live for this node.
	Dir string

	mu   sync.Mutex
	qemu *exec.Cmd

	// renodeStdin holds Renode's monitor open; see startRenode.
	renodeStdin *os.File
	radio       *exec.Cmd
	sock        string
	radioPort   int

	// serial is the emulator's own serial port, when it publishes one.
	serial *serialLink
}

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

// HasConsole is true once the emulator publishes a serial port we hold open.
// Renode's machines do not yet, so a board booted under it is still watched
// rather than driven.
func (e *EmulatedNode) HasConsole() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.serial != nil
}

// ConsoleIn is the node's serial port. An emulated node carries its own, rather
// than the console frames the native shim answers on the bridge.
func (e *EmulatedNode) ConsoleIn() io.Writer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.serial == nil {
		return nil
	}
	return e.serial
}

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

	// Looked up here rather than beside the radio model: an nRF52 board runs
	// under Renode and has no use for the Xtensa emulator, and asking for it
	// first refused those boards on a machine that only has the one they need.
	qemuBin, err := lookupTool(EnvQEMU, "qemu-system-xtensa")
	if err != nil {
		_ = e.stopLocked()
		return err
	}

	// What the device is told to connect to: the socket file, or the port the
	// radio model chose when there is no socket file to have.
	radioAt := e.sock
	if e.radioPort != 0 {
		radioAt = fmt.Sprintf("127.0.0.1:%d", e.radioPort)
	}
	machine := fmt.Sprintf("%s,radio-path=%s,radio-spi=%d,radio-nss=%d,radio-busy=%d",
		e.Machine, radioAt, e.SPI, e.NSS, e.Busy)
	// Only when the board records one. Without it the machine leaves the line
	// unwired, and the firmware never learns a packet arrived - it reads a
	// received packet solely from the interrupt this pin raises.
	if e.DIO1 != 0 {
		machine += fmt.Sprintf(",radio-dio1=%d", e.DIO1)
	}
	// Only when the board has one. Left off, the machine leaves the line
	// unwired, which is what a board with no module should look like - as
	// opposed to one whose module is permanently switched off.
	if e.FEM != 0 {
		machine += fmt.Sprintf(",radio-fem=%d", e.FEM)
	}
	if e.PSRAMOctal {
		machine += ",psram-octal=on"
	}
	if e.ButtonPath != "" && len(e.ButtonPins) > 0 {
		pins := make([]string, len(e.ButtonPins))
		for i, p := range e.ButtonPins {
			pins[i] = strconv.Itoa(p)
		}
		machine += fmt.Sprintf(",input-path=%s,input-pins=%s",
			e.ButtonPath, strings.Join(pins, ","))
	}
	// The display, on the same terms as the radio: only when the board has
	// one and only when something is listening.
	if e.PanelPath != "" {
		addr := e.PanelAddr
		if addr == 0 {
			addr = 0x3C
		}
		machine += fmt.Sprintf(",panel-path=%s,panel-addr=%d,panel-offset=%d",
			e.PanelPath, addr, e.PanelOffset)
		if e.PanelCS != 0 {
			machine += fmt.Sprintf(",panel-cs=%d,panel-dc=%d,panel-w=%d,panel-h=%d",
				e.PanelCS, e.PanelDC, e.PanelWidth, e.PanelHgt)
		}
	}

	qemuLog, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		_ = e.stopLocked()
		return err
	}
	// Serial to a socket rather than to a file. The file it used to be written
	// to was write-only, which made the node's command interface unreachable -
	// MeshCore's applications carry their own on Serial, so a board that could
	// not be typed at could not be configured, only watched. The socket carries
	// the same boot chain, ROM through application, and is copied on to the
	// same log, so what could be read before still can be.
	// The flash device the machine gets is the image grown to the size its own
	// partition table describes, so the partitions past the application exist
	// to be mounted rather than falling off the end of the chip.
	flash, err := padToDeclaredFlash(e.Image, e.Dir)
	if err != nil {
		_ = e.stopLocked()
		return err
	}

	conPath := filepath.Join(e.Dir, "console.sock")
	args := []string{
		"-machine", machine,
		"-nographic", "-monitor", "none",
		"-drive", "file=" + flash + ",if=mtd,format=raw",
		"-chardev", "socket,id=con,path=" + conPath + ",server=on,wait=off",
		"-serial", "chardev:con",
	}
	if e.PSRAMMB > 0 {
		args = append(args, "-m", fmt.Sprintf("%dM", e.PSRAMMB))
	}
	e.qemu = exec.CommandContext(ctx, qemuBin, args...)
	e.qemu.Stdout, e.qemu.Stderr = qemuLog, qemuLog
	if err := e.qemu.Start(); err != nil {
		_ = e.stopLocked()
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	e.serial = dialSerial(ctx, conPath, qemuLog)
	return nil
}

// ConsoleLog is everything the node has said on its serial port: the whole
// boot chain, ROM through application, and the replies to anything typed at it.
func (e *EmulatedNode) ConsoleLog() ([]byte, error) {
	return os.ReadFile(e.ConsolePath())
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
	if e.serial != nil {
		_ = e.serial.Close()
		e.serial = nil
	}
	for _, c := range []*exec.Cmd{e.qemu, e.radio} {
		if c == nil || c.Process == nil {
			continue
		}
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	}
	e.qemu, e.radio = nil, nil
	if e.renodeStdin != nil {
		_ = e.renodeStdin.Close()
		e.renodeStdin = nil
	}
	_ = os.Remove(e.sock)
	return nil
}

// Screen is the last picture this board's display sent, and whether there is
// one at all.
//
// The last return says which: a board that declares no display and a board
// whose display has drawn nothing are different facts, and drawing an empty
// picture for both would report the first as the second.
func (e *EmulatedNode) Screen() (width, height, bpp int, on bool, bits []byte, have bool) {
	if e.Panel == nil {
		return 0, 0, 0, false, nil, false
	}
	f, _ := e.Panel.Frame()
	if f == nil {
		return 0, 0, 0, false, nil, false
	}
	return f.Width, f.Height, f.BPP, f.On, f.Bits, true
}

// PressButton holds one of this board's buttons down or lets it go.
func (e *EmulatedNode) PressButton(pin int, down bool) error {
	if e.Buttons == nil {
		return ErrNoButtons()
	}
	return e.Buttons.Press(pin, down)
}

// ButtonHeld reports whether a pin is being held.
func (e *EmulatedNode) ButtonHeld(pin int) bool {
	return e.Buttons != nil && e.Buttons.Held(pin)
}
