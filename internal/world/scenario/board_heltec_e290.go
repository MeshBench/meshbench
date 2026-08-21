// The Heltec_E290, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecE290Board = Board{
	Name: "Heltec_E290", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 300,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Heltec's S3 radio pins again, and this one carries octal RAM where the
	// V3 carries none - a firmware built for one reports the other as absent.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 45,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Notes: "A 2.9 inch e-paper panel on the V3's radio wiring. The display costs " +
		"almost nothing between refreshes, which is what makes this family worth " +
		"simulating as a sensor rather than a repeater.",
}
