// The Generic_E22_sx1262, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var genericE22Sx1262Board = Board{
	// The first board driven end to end under emulation, which is why it is
	// here rather than for being popular.
	Name: "Generic_E22_sx1262", MCU: "ESP32", Radio: "SX1262", Vendor: "Ebyte",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 2.15,
	SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 250,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	QEMU: &QEMUWiring{
		// FEM 13 is SX126X_TXEN, from the variant's platformio.ini. RadioLib
		// drives it from setRfSwitchPins(RXEN=14, TXEN=13), so it goes high
		// before SetTx and low again before SetRx.
		Machine: "esp32", SPI: 2, NSS: 18, Busy: 32, DIO1: 33, LED: 2, FEM: 13,
		Verified: true,
	},
	// The module's TXEN and RXEN, on MCU pins 13 and 14. No gain stage:
	// MeshCore compiles this variant for LORA_TX_POWER=22 and the E22's own
	// SX1262 produces it, so these switch the path rather than amplify it.
	// The loss is an RF switch's isolation and is a plausible figure rather
	// than a measured one - see Notes.
	FEM: &FEM{TxGainDB: 0, TxLossDB: 25},
	Notes: "An E22 module on a devkit rather than a product, so the antenna figure " +
		"assumes the external whip the module is designed for. The published " +
		"repeater image boots and runs RadioLib's full SX126x init sequence under " +
		"emulation: version read, LoRa mode, modulation and IRQ setup. " +
		"The 25 dB switch isolation is a plausible figure for an SPDT part at " +
		"868 MHz and has not been measured. Upstream also sets " +
		"SX126X_DIO2_AS_RF_SWITCH=true alongside the MCU pins, which its own " +
		"variant.h warns against - so on this board the path may be switched by " +
		"DIO2 whatever the MCU pins do, and a transmit-enable fault here would " +
		"be milder than the model says. The T096 is the honest case for that.",
}
