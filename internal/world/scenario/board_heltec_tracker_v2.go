// The heltec_tracker_v2, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecTrackerV2Board = Board{
	Name: "heltec_tracker_v2", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 900,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 18,
	},
	Notes: "The tracker with an amplifier: the firmware asks the chip for 9 dBm " +
		"and a KCT8103L takes it to about 22 at the antenna. No transmit-enable " +
		"GPIO is declared for it because the variant configures none - the module " +
		"is switched by the radio rather than by firmware - so the fault that " +
		"costs a T096 its output cannot happen here and is not checked for.",
}
