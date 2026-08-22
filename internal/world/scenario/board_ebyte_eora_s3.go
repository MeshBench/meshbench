// The Ebyte_EoRa-S3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var ebyteEoRaS3Board = Board{
	Name: "Ebyte_EoRa-S3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Ebyte",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: 2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0},
	Emulated: true,
	// Pins from variants/ebyte_eora_s3/platformio.ini, which are the T3S3's:
	// the two boards are the same layout under different names.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 7, Busy: 34, DIO1: 33, LED: 37,
		PSRAMMB: 8,
	},
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 37},
			{Kind: Button, Name: "BOOT", Pin: 0, ActiveLow: true},
		},
	},
	Notes: "Wired identically to the LilyGo T3S3 and published as its own image. " +
		"Ebyte also sell modules with an integrated amplifier under names close to " +
		"this one; those are a different board and these figures do not describe " +
		"them.",
}
