// The Heltec_v3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecV3Board = Board{
	Name: "Heltec_v3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/heltec_v3/platformio.ini - raw GPIO numbers on an
	// ESP32. No PSRAM on this board, so nothing to declare.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14,
	},
	Notes: "Very common and not a good repeater: the stock spring antenna is well " +
		"below a dipole, and sleep current is dominated by the board rather than " +
		"the MCU. Its radio has a bus now - the machine builds GPSPI2 and offers " +
		"the 49 GPIOs this part has rather than the ESP32's 40 - but the published " +
		"image still asserts inside ESP-IDF's own startup, before MeshCore runs.",
}
