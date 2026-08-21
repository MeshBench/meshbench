// The Station_G3_ESP32, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var stationG3ESP32Board = Board{
	Name: "Station_G3_ESP32", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 27, FeedlineDB: 1.5, AntennaDBi: 2.15,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 5000,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/station_g3_esp32/platformio.ini. Same radio pins as
	// the G2, octal RAM like it, and a second MCU option published separately.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 11, Busy: 47, DIO1: 48,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Notes: "Mains-powered with an external amplifier: the firmware asks the chip " +
		"for 7 dBm and about half a watt leaves the antenna. Check the licence " +
		"conditions before simulating it at full power. The amplifier and the " +
		"low-noise amplifier ahead of the receiver are each switched by their own " +
		"GPIO, neither of which is RadioLib's transmit-enable line, so a " +
		"front-end module is not the right shape for them and none is declared - " +
		"which means a firmware that leaves the amplifier out will not be caught " +
		"here.",
}
