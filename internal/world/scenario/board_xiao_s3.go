// The Xiao_S3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var xiaoS3Board = Board{
	Name: "Xiao_S3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Seeed",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 50,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/xiao_s3/platformio.ini. The same module as the WIO
	// on a different carrier, so the radio moves and the RAM does not.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 5, Busy: 4, DIO1: 2,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Notes: "The Xiao S3 with a LoRa expansion rather than the WIO's integrated " +
		"radio: the same MCU module, a different set of pins, and its own image. " +
		"GPIO 6 gates the receive path; there is no transmit-enable line, so " +
		"nothing here is a front-end module. Tiny, and the antenna figure " +
		"reflects it.",
}
