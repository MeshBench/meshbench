// The Heltec_WSL3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecWSL3Board = Board{
	Name: "Heltec_WSL3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: 0,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 180,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// The V3's build with the display left out: its repeater environment
	// extends the same base, so the radio is on the same pins.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14,
	},
	Notes: "The Wireless Stick Lite: a V3 without the screen, and better for it " +
		"as a repeater - no display rails to keep up and an SMA connector instead " +
		"of the V3's spring antenna, so the antenna figure is for whatever gets " +
		"fitted rather than something poor and fixed. No battery holder, so the " +
		"capacity here is zero rather than invented.",
}
