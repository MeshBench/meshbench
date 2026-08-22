// The Heltec_Wireless_Paper, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecWirelessPaperBoard = Board{
	Name: "Heltec_Wireless_Paper", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 300,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 750, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/heltec_wireless_paper/platformio.ini - the V3's, on
	// the same devkit board definition, and with no PSRAM on either.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 18,
	},
	// E-paper, which wants a refresh model rather than a framebuffer, so no
	// screen is declared yet.
	Hardware: &Panel{
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 18},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
		},
	},
	Notes: "An e-paper display, which is why the sleep figure is worse than the " +
		"V3's it otherwise matches: the panel's driver keeps its rails up. The " +
		"small cell makes it a poor repeater and a reasonable sensor. Radio " +
		"wiring is the V3's exactly.",
}
