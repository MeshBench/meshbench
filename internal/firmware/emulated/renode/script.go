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
func RenodeTrace() string {
	if os.Getenv(EnvRenodeTrace) == "" {
		return ""
	}
	return `logLevel 0
sysbus LogPeripheralAccess sysbus.radiospi true
sysbus LogPeripheralAccess sysbus.twi0 true
sysbus LogPeripheralAccess sysbus.twi1 true
`
}
