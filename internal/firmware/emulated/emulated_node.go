package emulated

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

	"github.com/MeshBench/meshbench/internal/firmware"
)

// EnvQEMU overrides the emulator binary, and EnvRadioServer the radio model.
//
// Both are ours rather than distribution packages: the emulator carries an
// SX1262 device and a GPIO implementation that upstream does not have, and the
// radio model is the same one native nodes run against. A released build ships
// both, and until then these say where to find them.
const (
	EnvQEMU        = "MESHBENCH_QEMU"
	EnvRadioServer = "MESHBENCH_RADIO_SERVER"
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

	// KbdAddr and TouchAddr are where a keyboard and a touch panel answer on
	// the board's I2C bus, or zero where it carries neither.
	KbdAddr, TouchAddr uint8

	// CardPath is the file behind the board's card slot, and CardCS the pin
	// that selects it. Empty on a board with no slot.
	CardPath string
	CardCS   int

	// ConsoleOnUSB puts the console on the machine's USB Serial/JTAG rather
	// than on UART0, for a board whose firmware has Serial there.
	//
	// Which it is is the build's business rather than the part's, so it comes
	// from the board profile. Getting it wrong is quiet: the board boots, the
	// ROM bootloader prints to UART0, and then everything the application says
	// goes to a peripheral nobody is holding - a node that started and appears
	// never to have spoken.
	ConsoleOnUSB bool

	// GPSPath is where the board's receiver sends its sentences from, or empty
	// on a board that carries none.
	GPSPath string
	GPS     *GPSFeed

	// BatChannel is the converter channel the board's cell is read on, and
	// BatRaw what it reads at bring-up. Both zero on a board that declares no
	// meter, which leaves the channel reading nothing - honest, and still
	// enough to stop a firmware waiting for a conversion that never finishes.
	BatChannel int
	BatRaw     uint16

	// Panel is where this node's pictures arrive, when something is listening.
	// Held here so a caller with the node has the screen too, rather than
	// having to keep the two in step itself.
	Panel *PanelListener

	// PSRAMOctal selects an octal (OPI) part rather than a quad one.
	PSRAMOctal bool

	// CoprocAtReset brings the coprocessors up enabled, which the part does
	// not do. A property of the build rather than of the board, so it arrives
	// from the build's own settings; EnvCoprocAtReset forces it on for every
	// node regardless, which is how it is reached from a script.
	CoprocAtReset bool

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

	// console is where the serial port's output goes: the log file always, and
	// whoever is currently listening as well.
	console *firmware.ConsoleSink
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
		e.Dir = filepath.Join(os.TempDir(), "meshbench-emulated", e.NodeName)
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
	e.radio.SysProcAttr = firmware.ChildProcAttr()
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
	machine := e.machineString(radioAt)

	// The board's own output and the emulator's are two different things, and
	// separating them is what makes either readable. They shared console.log,
	// so a QEMU error about a drive or a machine property landed in the middle
	// of the boot chain looking like something the firmware had printed - and
	// the boot-banner and assert patterns the board probe matches against that
	// file could be satisfied by the emulator complaining rather than by the
	// board saying anything at all.
	conLog, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		_ = e.stopLocked()
		return err
	}
	e.console = firmware.NewConsoleSink(conLog)
	emuLog, err := os.Create(filepath.Join(e.Dir, emulatorLogName))
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
	}
	// The order of these is the order of the ports: UART0, UART1, then the USB
	// Serial/JTAG. Which one the console goes to depends on where this board's
	// firmware put Serial, and the other two are filled in so the count still
	// lands the console where it is meant to.
	//
	// UART0 keeps a log of its own either way. The ROM bootloader prints there
	// before any of this is configured, and that output - the reset reason,
	// the load addresses, the entry point - is what says whether a board
	// started at all, which is exactly the question being asked of a board
	// that says nothing afterwards.
	if e.ConsoleOnUSB {
		args = append(args, "-serial", "file:"+filepath.Join(e.Dir, romLogName))
	} else {
		args = append(args, "-serial", "chardev:con")
	}
	// The receiver's port, where the board has one: the second, which is the
	// port its variant opens.
	switch {
	case e.GPSPath != "":
		args = append(args,
			"-chardev", "socket,id=gps,path="+e.GPSPath+",server=on,wait=off",
			"-serial", "chardev:gps")
	case e.ConsoleOnUSB:
		// A placeholder, so the console lands on the third port rather than
		// the second.
		args = append(args, "-serial", "null")
	}
	if e.ConsoleOnUSB {
		args = append(args, "-serial", "chardev:con")
	}
	if e.CardPath != "" {
		args = append(args, "-drive", "if=sd,format=raw,file="+e.CardPath)
	}
	if e.PSRAMMB > 0 {
		args = append(args, "-m", fmt.Sprintf("%dM", e.PSRAMMB))
	}
	args = append(args, qemuDebugArgs(e.Dir)...)
	e.qemu = exec.CommandContext(ctx, qemuBin, args...)
	e.qemu.Stdout, e.qemu.Stderr = emuLog, emuLog
	// The emulator dies with the simulator. Without this a workbench killed
	// outright leaves a qemu-system-xtensa and a radioserver per node running,
	// and the next run's radio socket is answered by the last run's model.
	e.qemu.SysProcAttr = firmware.ChildProcAttr()
	if err := e.qemu.Start(); err != nil {
		_ = e.stopLocked()
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	e.serial = dialSerial(ctx, conPath, e.console)
	return nil
}

// TeeConsole sends a copy of everything the node says on its serial port to w,
// on top of the log file it always writes. Nil stops the copy.
//
// This is how an emulated node's output reaches the things that read a native
// node's: the console pane, a companion client, meshcore-cli over TCP. Without
// it the port was readable only by opening the log file afterwards, so every
// emulated board was write-only to everything in the application that tried to
// listen to it - which read as a board that had no serial port at all.
func (e *EmulatedNode) TeeConsole(w io.Writer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.console != nil {
		e.console.SetTee(w)
	}
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
	// The receiver stops with the board. It is the one of these that keeps a
	// clock running of its own, so leaving it would have a stopped node still
	// reporting where it is.
	if e.GPS != nil {
		_ = e.GPS.Close()
		e.GPS = nil
	}
	_ = os.Remove(e.sock)
	return nil
}
