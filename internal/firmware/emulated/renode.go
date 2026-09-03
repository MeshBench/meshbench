package emulated

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/renode"
)

// renodeScript writes the machine description this node needs and the monitor
// script that boots it, and returns the script's path.
//
// Generated rather than kept as a file, because four of the values are
// per-node: the radio model's port, the console's port, the node's own working
// directory, and the image. A shared script would need all four passed in
// anyway.
func (e *EmulatedNode) renodeScript(conPort int) (string, error) {
	// The radio's wiring goes in a platform description of its own rather than
	// into the script: these are declarations, and Renode's monitor does not
	// take declarations. Inlining them in the .resc left the machine without a
	// radio and the firmware waiting on a chip that was never there.
	repl := filepath.Join(e.Dir, "node.repl")
	wiring := fmt.Sprintf(`%s
radiospi: SPI.NRF52840_SPI @ sysbus 0x%X
    easyDMA: true

lora: Radio.RadioServerSX1262 @ radiospi
    host: "127.0.0.1"
    port: %d
    IRQ -> %s@%d

%s:
    %d -> lora@0
`, renode.EasyDMASPI(e.SPIBase), e.SPIBase, e.radioPort, e.IrqPort, e.IrqPin, e.NssPort, e.NssPin)
	if err := os.WriteFile(repl, []byte(wiring), 0o644); err != nil {
		return "", err
	}

	// Inputs the board holds high in copper. Driven from the monitor rather
	// than the platform description because a .repl says how things are wired,
	// not what level a pin sits at.
	idle := ""
	for _, p := range e.IdleHighPins {
		idle += fmt.Sprintf("%s OnGPIO %d true\n", p.Port, p.Pin)
	}

	flash, err := e.renodeFlash()
	if err != nil {
		return "", err
	}

	script := filepath.Join(e.Dir, "node.resc")
	// The host has to be included before the controller: Renode compiles each
	// of these on its own, so a file can only see what came before it.
	body := fmt.Sprintf(`i @%[1]s/peripherals/RadioServerSX1262.cs
i @%[1]s/peripherals/NRF52840_Temp.cs
i @%[1]s/peripherals/NRF52840_Clock.cs
i @%[1]s/peripherals/NRF52840_SAADC.cs
i @%[1]s/peripherals/NRF52840_TWIM.cs
i @%[1]s/peripherals/NRF52840_NVMC.cs
i @%[1]s/peripherals/NRF52840_CryptoCell.cs
i @%[1]s/peripherals/UsbdRegisters.cs
i @%[1]s/peripherals/UsbCdcHost.cs
i @%[1]s/peripherals/NRF52840_USBD.cs

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
machine LoadPlatformDescription @%[1]s/nvmc.repl
machine LoadPlatformDescription @%[1]s/cryptocell.repl
machine LoadPlatformDescription @%[1]s/usbd.repl
%[6]smachine LoadPlatformDescription @%[4]s

%[5]s
%[9]sradiospi.lora Connect
%[8]s%[7]sstart
`, ToolsDir(), firmware.SafeNodeName(e.NodeName), e.Platform, repl, flash,
		renode.UnregisterStockSPI(), renode.RenodeTrace(),
		renode.ConsoleTerminal(conPort, e.ConsoleOnUSB), idle)
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		return "", err
	}
	return script, nil
}

// startRenode boots this node's script, and holds its console open.
func (e *EmulatedNode) startRenode(ctx context.Context) error {
	renodeBin, err := lookupTool("renode")
	if err != nil {
		return err
	}
	conPort, err := e.consolePort(ctx)
	if err != nil {
		return err
	}
	script, err := e.renodeScript(conPort)
	if err != nil {
		return err
	}
	log, err := e.openRenodeLogs(conPort)
	if err != nil {
		return err
	}
	// Renode's monitor reads commands from standard input, and a monitor at
	// end of file quits. With nothing on stdin it ran the script, reached the
	// end and shut the machine down about four hundred milliseconds later -
	// long enough to look like a node that booted and then died on its own.
	// The pipe is never written to; it exists to stay open.
	stdin, hold, err := os.Pipe()
	if err != nil {
		return err
	}
	e.qemu = exec.CommandContext(ctx, renodeBin,
		"--disable-xwt", "--console", "-e", "include @"+script)
	e.qemu.Stdin = stdin
	e.qemu.Stdout, e.qemu.Stderr = log, log
	e.qemu.SysProcAttr = firmware.ChildProcAttr()
	if err := e.qemu.Start(); err != nil {
		_, _ = hold.Close(), stdin.Close()
		return fmt.Errorf("firmware: starting the emulator: %w", err)
	}
	_ = stdin.Close()
	e.renodeStdin = hold
	if conPort != 0 {
		e.serial = peripheral.DialSerial(ctx, fmt.Sprintf("127.0.0.1:%d", conPort), e.console)
	}
	return nil
}

// openRenodeLogs makes the two files a running node writes, and points the
// console at the one the board's own words go in.
//
// Renode's own output is the peripherals it could not load, the properties it
// refused and the reason it exited; the board's is what the firmware printed.
// console.log is created whether or not anything will reach it, because a board
// that said nothing and a node that never started are different answers and a
// missing file gives the same one for both.
func (e *EmulatedNode) openRenodeLogs(conPort int) (*os.File, error) {
	log, err := os.Create(filepath.Join(e.Dir, emulatorLogName))
	if err != nil {
		return nil, err
	}
	conLog, err := os.Create(filepath.Join(e.Dir, consoleLogName))
	if err != nil {
		return nil, err
	}
	if conPort != 0 {
		e.console = firmware.NewConsoleSink(conLog)
	}
	return log, nil
}

// consolePort is where this node publishes its serial port.
//
// A board printing over USB used to get zero here, because nothing modelled the
// part's USB device controller and a terminal on the UART instead would publish
// a port that answers nothing - on some of these boards that UART is the
// receiver's, so it would not even be quiet. NRF52840_USBD is that model, and
// the terminal now goes wherever the board's firmware put Serial.
//
// The port is asked of the kernel and given up again rather than picked from a
// range: a range collides with whatever else the machine is doing, and Renode's
// terminal takes the port it is given and has no way to report one it chose
// itself. The gap between closing this listener and Renode opening the port is a
// race in principle; losing it costs a node that fails to start rather than one
// that starts wrong.
func (e *EmulatedNode) consolePort(ctx context.Context) (int, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("firmware: finding a port for the node's console: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()
		return 0, fmt.Errorf("firmware: a TCP listener answered with a %T", ln.Addr())
	}
	return addr.Port, ln.Close()
}

// Renode's nRF52840 declares exactly one SPI controller, and models only its
// legacy register interface.
//
// Firmware on these boards drives the EasyDMA half instead, so transfers go to
// registers that are not there, reads come back zero and EVENTS_END never
// arrives. The two addresses that look like the other controllers - 0x40003000
// and 0x40004000 - are the TWI blocks, which this script already replaces.
