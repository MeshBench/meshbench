// The RAK_3112, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var rak3112Board = Board{
	Name: "RAK_3112", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "RAKwireless",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 0,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 120,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Its own pins, not the Heltec family's, despite sharing their board
	// definition: taken from variants/rak3112/platformio.ini.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 7, Busy: 48, DIO1: 47, LED: 46,
	},
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 46},
			{Kind: Button, Name: "user", Pin: PinNone},
		},
	},
	Notes: "RAK's ESP32-S3 module on a WisBlock base, with an IPEX connector, so " +
		"the antenna figure is for whatever is plugged in rather than anything the " +
		"board provides. Better built for being left alone than the Heltec boards " +
		"beside it, and the sleep figure says so.",
}
