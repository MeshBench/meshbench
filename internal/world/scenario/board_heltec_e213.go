// The Heltec_E213, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecE213Board = Board{
	Name: "Heltec_E213", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 300,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 45,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 45},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
		},
	},
	Notes: "The smaller e-paper board of the pair, wired identically to the E290 " +
		"and published as its own image. The figures here are the E290's; nothing " +
		"about the panel changes what leaves the antenna.",
}
