package emulated

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
)

// EnvQEMU overrides the emulator binary, and EnvRadioLib the chip model.
//
// Both are ours rather than distribution packages: the emulator carries an
// SX1262 device and a GPIO implementation that upstream does not have, and the
// chip is the same virtual-sx1262 a native node links. A released build ships
// both, and until then these say where to find them.
const (
	EnvQEMU = "MESHBENCH_QEMU"
	// EnvRadioLib is read by the emulator, not by us: QEMU's sx1262 device and
	// Renode's peripheral both load the library named here. It is passed down
	// rather than put on a machine argument because which chip model is
	// installed is a property of this machine and not of the board.
	EnvRadioLib = "MESHBENCH_RADIO_LIB"
	// EnvNoiseSeed seeds the radio model's receiver noise, per node.
	EnvNoiseSeed = "MESHBENCH_NOISE_SEED"
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
// One process. The emulator holds the chip itself, as a library it loads, and
// the only socket left is the one to the engine - which is genuinely elsewhere,
// because the channel is shared with every other node. There used to be a third
// process in between, owning the chip and forwarding SPI a byte at a time, and
// it cost more than the round trips: an emulated node and a native one reached
// the same model down different paths, and the paths disagreed for months
// without anything noticing. Now they call the same library.
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
	FEM int
	// IdleHighPins are inputs this board holds high in copper, which Renode
	// has no notion of: an undriven input reads low, and a low on a button pin
	// is a button held down.
	IdleHighPins []GPIOPin

	NodeName string
	// RunSeed is the simulation's seed. With the node's name it decides this
	// radio's noise, and through that the identity the firmware generates: two
	// nodes in one run must differ, and the same node in two runs with
	// different seeds must differ too.
	RunSeed uint64

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
	Buttons    *peripheral.ButtonSender

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
	GPS     *peripheral.GPSFeed

	// BatChannel is the converter channel the board's cell is read on, and
	// BatRaw what it reads at bring-up. Both zero on a board that declares no
	// meter, which leaves the channel reading nothing - honest, and still
	// enough to stop a firmware waiting for a conversion that never finishes.
	BatChannel int
	BatRaw     uint16

	// Panel is where this node's pictures arrive, when something is listening.
	// Held here so a caller with the node has the screen too, rather than
	// having to keep the two in step itself.
	Panel *peripheral.PanelListener

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

	// radioEnv is the emulator's environment: where the chip model is and what
	// this node's receiver noise is seeded with. Built in Start, because the
	// seed depends on the run and the node's name.
	radioEnv []string

	// workLock is this process's exclusive claim on Dir, held from Start until
	// stopLocked has confirmed every process that might still be touching it
	// is actually gone - not just asked to stop.
	workLock *firmware.WorkDirLock

	// serial is the emulator's own serial port, when it publishes one.
	serial *peripheral.SerialLink

	// console is where the serial port's output goes: the log file always, and
	// whoever is currently listening as well.
	console *firmware.ConsoleSink
}

func (e *EmulatedNode) Kind() string { return "emulated" }

// HasConsole is true once the emulator publishes a serial port we hold open:
// QEMU on a socket file, Renode on a server terminal.
//
// It is a question about the port and not about the emulator, which is why it
// asks the field rather than the backend. A board whose firmware prints over
// USB gets no port from Renode, because nothing there models a USB device, and
// answering yes for it would leave the console pane, provisioning and the fleet
// commands all typing into a machine that cannot hear them.
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

// Start brings the emulator up, with the chip inside it.
//
// bridge is the engine's listener for this node, as host:port. The radio joins
// it as the machine is built, so the engine must already be listening; empty
// leaves the node deaf and mute, booting and then waiting for ever on a
// transmission that cannot complete, which looks like a hang rather than a
// missing argument.
func (e *EmulatedNode) Start(ctx context.Context, bridge string) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// A second Start on a node already running would overwrite e.qemu with a
	// new process, orphaning the first: nothing would hold its PID any more,
	// and Stop would kill only the second.
	if e.qemu != nil {
		return fmt.Errorf("firmware: emulated node already started")
	}
	if e.Image == "" {
		return fmt.Errorf("firmware: an emulated node needs a flash image")
	}
	if e.Machine == "" {
		e.Machine = "esp32"
	}
	if e.Dir == "" {
		e.Dir = filepath.Join(os.TempDir(), "meshbench-emulated", e.NodeName)
	}
	lock, err := firmware.LockWorkDir(e.Dir)
	if err != nil {
		return err
	}
	e.workLock = lock
	// Every failure from here on is handled the one way: stopLocked already
	// knows how to let go of processes that did start and a lock nothing
	// needs any more, so nothing past this point has to call it for itself.
	defer func() {
		if err != nil {
			_ = e.stopLocked()
		}
	}()
	// Where the chip comes from, and what its receiver's noise is seeded with.
	// Both reach the emulator through its environment rather than through a
	// machine argument: which chip model is installed is a property of this
	// machine, not of the board, and the seed is a property of the run.
	//
	// The seed is where the firmware's entropy comes from. RadioLib reads the
	// chip's instantaneous RSSI for random bits and MeshCore derives its
	// identity from them, so every node needs its own stream or every node
	// comes up with the same keypair - which is what happened, and two
	// different nRF52 boards reported the same public key. Derived from the
	// node's name rather than from the machine, so a run stays reproducible:
	// same scenario, same names, same noise.
	radioLib, err := lookupTool(radioLibName)
	if err != nil {
		return err
	}
	e.radioEnv = append(os.Environ(),
		fmt.Sprintf("%s=%s", EnvRadioLib, radioLib),
		fmt.Sprintf("%s=%d", EnvNoiseSeed, noiseSeedFor(e.RunSeed, e.NodeName)))

	if e.Emulator == Renode {
		if err := e.startRenode(ctx, bridge); err != nil {
			return err
		}
		return nil
	}

	// Looked up here rather than beside the radio model: an nRF52 board runs
	// under Renode and has no use for the Xtensa emulator, and asking for it
	// first refused those boards on a machine that only has the one they need.
	qemuBin, err := lookupTool("qemu-system-xtensa")
	if err != nil {
		return err
	}

	machine := e.machineString(bridge)

	// The board's own output and the emulator's are two different things, and
	// separating them is what makes either readable. They shared console.log,
	// so a QEMU error about a drive or a machine property landed in the middle
	// of the boot chain looking like something the firmware had printed - and
	// the boot-banner and assert patterns the board probe matches against that
	// file could be satisfied by the emulator complaining rather than by the
	// board saying anything at all.
	conLog, err := os.Create(filepath.Join(e.Dir, "console.log"))
	if err != nil {
		return err
	}
	e.console = firmware.NewConsoleSink(conLog)
	emuLog, err := os.Create(filepath.Join(e.Dir, emulatorLogName))
	if err != nil {
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
	e.qemu.Env = e.radioEnv
	e.qemu.Stdout, e.qemu.Stderr = emuLog, emuLog
	// The emulator dies with the simulator. Without this a workbench killed
	// outright leaves a qemu-system-xtensa and a radioserver per node running,
	// and the next run's radio socket is answered by the last run's model.
	e.qemu.SysProcAttr = firmware.ChildProcAttr()
	if err := e.qemu.Start(); err != nil {
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	e.serial = peripheral.DialSerial(ctx, conPath, e.console)
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

// Stop and stopLocked live in emulated_reap.go, beside the bounded wait they
// depend on to reap what they kill.
