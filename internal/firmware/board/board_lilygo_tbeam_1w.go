// The LilyGo_TBeam_1W, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var tbeam1WBoard = Board{
	Name: "LilyGo_TBeam_1W", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 30, FeedlineDB: 1.5, AntennaDBi: 2,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 900,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0},
	Emulated: true,
	// Pins from variants/lilygo_tbeam_1w/platformio.ini. An S3 despite the
	// name it shares with the ESP32 T-Beam, and its RAM is an octal part.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 15, Busy: 38, DIO1: 1,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Notes: "A watt at the antenna, which almost nowhere permits on these bands - " +
		"check the licence conditions before simulating it at full power, and " +
		"expect a result that flatters any network built around it. The power " +
		"amplifier is not switched by a firmware GPIO the way a T096's module is, " +
		"so there is no equivalent fault to catch; GPIO 21 gates the receive path " +
		"only, and a receive noise figure a decibel better than the bare chip is " +
		"what the low-noise amplifier buys.",
}
