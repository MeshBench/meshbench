// Package renode builds the Renode script fragments for an emulated nRF52
// board: the SPI controller fix-ups and the optional access trace.
package renode

import (
	"fmt"
	"os"
)

// EnvRenode overrides the Renode executable, as EnvQEMU does for the other
// emulator. Ours is a fork with the peripherals an nRF52 board needs, so a
// distribution build will not do.
const EnvRenode = "MESHBENCH_RENODE"

const (
	stockSPIName = "spi2"
	stockSPIBase = 0x40023000
)

// easyDMASPI puts that controller back with its EasyDMA half, unless the radio
// is already taking its address.
//
// The radio is not the only SPI device on some of these boards - a Heltec_t114
// has a display here - and firmware blocks on a controller it cannot drive
// whether or not there is a radio on it. Declared without a device: nothing
// answers, but EVENTS_END arrives, which is the difference between a board
// that carries on and one that polls 0x118 for ever.
func EasyDMASPI(radioBase uint32) string {
	if radioBase == stockSPIBase {
		return ""
	}
	return fmt.Sprintf("%s: SPI.NRF52840_SPI @ sysbus 0x%X\n    easyDMA: true\n\n",
		stockSPIName, stockSPIBase)
}

func UnregisterStockSPI() string {
	return "sysbus Unregister sysbus." + stockSPIName + "\n"
}

// The two places a board's firmware prints. Every board this emulator runs is
// an nRF52840, so uart0 is its one general-purpose UART; usbd is the USB device
// controller, which is where the published images print, because the Adafruit
// core builds Serial as a TinyUSB CDC device.
const (
	consoleUART = "uart0"
	consoleUSB  = "usbd"
)

// ConsolePort names the peripheral this board's console hangs off.
func ConsolePort(onUSB bool) string {
	if onUSB {
		return consoleUSB
	}
	return consoleUART
}

// ConsoleTerminal puts a two-way terminal on the board's console port.
//
// A server socket rather than the file backend this used to be. A file is
// write-only, so a board under Renode could be watched and never asked
// anything: MeshCore's applications carry their own command interface on the
// serial port, and everything the workbench does to a node by console - the
// console pane, provisioning, fleet commands - was inert against one. The
// terminal is created before the machine starts, so what the board says from
// its first instruction has somewhere to go.
//
// TCP rather than a socket file because Renode offers nothing else, and the
// last argument is what makes it usable: with it true the terminal greets a
// client with telnet negotiation bytes, which are not something the board said
// and would land in the console log as though it had.
//
// Port zero is a board whose firmware prints somewhere this machine has not
// got, and it gets no terminal at all rather than one nothing answers on.
func ConsoleTerminal(port int, onUSB bool) string {
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("emulation CreateServerSocketTerminal %d \"console\" false\n"+
		"connector Connect sysbus.%s console\n", port, ConsolePort(onUSB))
}

// EnvRenodeTrace turns on peripheral-access logging in the generated script.
//
// Off by default and deliberately opt-in: it writes a line per register touch,
// which is megabytes a minute and slows the machine enough to change what is
// being measured, so a traced run is for reading structure and never for
// reporting timings. It exists because the alternative for "where is this
// board spending its time" is guessing, and guessing has a poor record here:
// a board that reads one address a hundred million times and a board that is
// merely slow look identical from outside.
const EnvRenodeTrace = "MESHBENCH_RENODE_TRACE"

// renodeTrace is the tracing preamble, or nothing.
//
// The radio SPI and the two I2C controllers, because those are the three places
// a board can wait: for the chip it talks to, or for a sensor that is not there.
// Silence on all three means the time is going somewhere else entirely, which is
// as useful an answer as noise on one of them.
//
// Both console ports are here for a different question. A board that prints
// nothing may have written to a port this machine does not model, or may never
// have written at all, and those two have different fixes; only the register
// traffic tells them apart. Both, because which one carries the console is a
// property of the firmware build rather than of the part, and a board wired to
// the wrong one looks exactly like a board that never spoke.
func RenodeTrace() string {
	if os.Getenv(EnvRenodeTrace) == "" {
		return ""
	}
	return `logLevel 0
sysbus LogPeripheralAccess sysbus.radiospi true
sysbus LogPeripheralAccess sysbus.twi0 true
sysbus LogPeripheralAccess sysbus.twi1 true
sysbus LogPeripheralAccess sysbus.` + consoleUART + ` true
sysbus LogPeripheralAccess sysbus.` + consoleUSB + ` true
`
}
