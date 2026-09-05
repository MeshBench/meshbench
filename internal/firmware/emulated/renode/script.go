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

// EasyDMASPI puts that controller back with its EasyDMA half, unless the radio
// is already taking its address.
//
// The radio is not the only SPI device on these boards. Both Heltec boards here
// put their display on the second controller - the Arduino core's SPI1, which
// is SPIM2 - and firmware blocks on a controller it cannot drive whether or not
// anything is on it.
//
// Declared without a device where there is nothing to declare: nothing answers,
// but EVENTS_END arrives, which is the difference between a board that carries
// on and one that polls 0x118 for ever.
func EasyDMASPI(radioBase uint32) string {
	if radioBase == stockSPIBase {
		return ""
	}
	return fmt.Sprintf("%s: SPI.NRF52840_SPI @ sysbus 0x%X\n    easyDMA: true\n\n",
		stockSPIName, stockSPIBase)
}

// Panel puts the board's display on that second controller, where the board
// puts it.
//
// It is alone there, so nothing has to be told apart: the controller clocks
// bytes at one device and the command/data line says whether a byte is a
// command or a pixel. That line is the whole reason a display needs a GPIO of
// its own, and a model without it would read a picture as a command stream.
//
// Nothing where the radio is on this address instead. No board here is wired
// that way, and inventing a second arrangement for a board that does not exist
// is how a description that cannot be loaded gets written.
func Panel(radioBase uint32, port, width, height, cs, dc int, dcPort string) string {
	if radioBase == stockSPIBase || port == 0 || width <= 0 || height <= 0 {
		return ""
	}
	return fmt.Sprintf(`panel: Video.MeshBenchPanel @ %s
    port: %d
    width: %d
    height: %d
    csPin: %d
    dcPin: %d

%s:
    %d -> panel@%d
    %d -> panel@%d
`, stockSPIName, port, width, height, cs, dc, dcPort, cs, cs, dc, dc)
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
//
// The interrupt path is the fourth place a board can wait, and it was the one
// place this could not see. An nRF52 MeshCore build does not poll the radio's
// IRQ register over SPI the way the ESP32 build does - it waits on the DIO1
// pin - so a board whose SPI traffic looks perfect can still never be told a
// packet arrived. That reads as a radio fault and is not one. GPIOTE and both
// ports say whether the firmware ever armed the pin and whether the event
// fired.
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
sysbus LogPeripheralAccess sysbus.gpiote true
sysbus LogPeripheralAccess sysbus.gpio0 true
sysbus LogPeripheralAccess sysbus.gpio1 true
`
}

// inputsBase is where the input channel is registered: an address nothing on an
// nRF52840 uses, between the peripheral block and the GPIO ports.
//
// A .repl registers a peripheral at an address whether or not the guest has any
// business reading it, and this one it has none - a person pressing a button is
// not something firmware can query. So the address is chosen to be out of the
// way rather than to mean anything.
const inputsBase = 0x4F000000

// Inputs puts the far end of the board's buttons in the machine.
//
// Nothing where the board has no inputs: a peripheral that dials a port nobody
// opened would retry for the life of the run and log about it.
//
// The converter is named only where the board has one, because the model takes
// it as an optional argument and a board with no cell to read has no meter to
// point at. Where it is named, the simulation's own battery state arrives on
// the same channel as the presses and lands in the converter the firmware
// reads - which is what makes an nRF52 board report a voltage rather than the
// one constant the model starts with.
func Inputs(port int, hasMeter bool) string {
	if port == 0 {
		return ""
	}
	s := fmt.Sprintf(`
inputs: Miscellaneous.MeshBenchInputs @ sysbus 0x%X
    port: %d
    gpio0: gpio0
    gpio1: gpio1
`, inputsBase, port)
	if hasMeter {
		s += "    meter: saadc\n"
	}
	return s
}
