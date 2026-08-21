// The Xiao_S3_WIO, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var xiaoS3WIOBoard = Board{
	Name: "Xiao_S3_WIO", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Seeed",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 50,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// The first ESP32-S3 to be wired here. Its radio is on GPSPI2 - FSPI on
	// this part, and what Arduino's SPI object drives - which the machine
	// did not model at all until now: it built the flash controller and
	// stopped. Pins from variants/xiao_s3_wio/platformio.ini, which are raw
	// GPIO numbers on an ESP32 rather than an Arduino index.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 2, NSS: 41, Busy: 40, DIO1: 39,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Notes: "Tiny, and the antenna figure reflects it. A companion, not a " +
		"repeater. SX126X_TXEN is RADIOLIB_NC on this board, so there is no " +
		"front-end module to switch in.",
}
