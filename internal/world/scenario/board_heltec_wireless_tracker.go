// The Heltec_Wireless_Tracker, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecWirelessTrackerBoard = Board{
	Name: "Heltec_Wireless_Tracker", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 900,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// The Heltec S3 radio pins, which every board in this family shares.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 18,
		Verified: true,
	},
	// A colour panel on SPI rather than an OLED on I2C, so the screen is
	// declared once the shared-bus work lands; the lamp and button are here.
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 18},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
		},
	},
	Notes: "A GPS and a small colour display on the V3's radio wiring, which is " +
		"where the sleep figure goes: the receiver dominates it and this board is " +
		"not built to be left alone. A tracker rather than a repeater, and worth " +
		"simulating as the node that moves.",
}
