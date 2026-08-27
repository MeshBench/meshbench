// The heltec_tracker_v2, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecTrackerV2Board = Board{
	Name: "heltec_tracker_v2", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 900,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 18,
		// Serial is the USB port on this board: its MeshCore variant is
		// built with ARDUINO_USB_CDC_ON_BOOT.
		ConsoleOnUSB: true,
	},
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 18},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
			// Heltec's own arithmetic: the reading is scaled by 5.42
			// against a 3.3 V range, so a cell at the top of it is 17.9 V -
			// a divider sized for a pack this board will never carry.
			{Kind: Meter, Name: "battery", Pin: 1, FullScaleMV: 17886},
		},
	},
	Notes: "The tracker with an amplifier: the firmware asks the chip for 9 dBm " +
		"and a KCT8103L takes it to about 22 at the antenna. No transmit-enable " +
		"GPIO is declared for it because the variant configures none - the module " +
		"is switched by the radio rather than by firmware - so the fault that " +
		"costs a T096 its output cannot happen here and is not checked for.",
}
