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
		// Serial is the USB port on this board: its MeshCore variant is
		// built with ARDUINO_USB_CDC_ON_BOOT.
		ConsoleOnUSB: true,
	},
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 45},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
			// Heltec's own arithmetic: the reading is scaled by 5.42
			// against a 3.3 V range, so a cell at the top of it is 17.9 V -
			// a divider sized for a pack this board will never carry.
			{Kind: Meter, Name: "battery", Pin: 7, FullScaleMV: 17886},
		},
	},
	Notes: "A 2.9 inch e-paper panel on the V3's radio wiring. The display costs " +
		"almost nothing between refreshes, which is what makes this family worth " +
		"simulating as a sensor rather than a repeater.",
}
