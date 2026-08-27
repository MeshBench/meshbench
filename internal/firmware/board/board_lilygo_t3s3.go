// The LilyGo_T3S3_sx1262, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var lilygoT3S3Board = Board{
	Name: "LilyGo_T3S3_sx1262", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: 2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 250,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0},
	Emulated: true,
	// Pins from variants/lilygo_t3s3/platformio.ini. Quad PSRAM, not octal:
	// the two are probed differently and a firmware built for one reports the
	// other as absent.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 7, Busy: 34, DIO1: 33, LED: 37,
		PSRAMMB:  8,
		Verified: true,
	},
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 37},
			{Kind: Button, Name: "BOOT", Pin: 0, ActiveLow: true},
			// A halving divider read against the converter's 3.3 V
			// range, so full scale is 6.6 V.
			{Kind: Meter, Name: "battery", Pin: 1, FullScaleMV: 6600},
		},
	},
	Notes: "The same board sells with an SX1276 fitted instead, published as a " +
		"separate image; the figures here are for the SX1262 one and nothing on " +
		"the outside of the case distinguishes them. An SMA connector, so the " +
		"antenna figure is for a whip somebody fits rather than anything the " +
		"board provides.",
}
