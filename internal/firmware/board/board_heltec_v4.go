// The heltec_v4, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecV4Board = Board{
	Name: "heltec_v4", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 6, SleepUA: 200,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/heltec_v4/platformio.ini. Quad PSRAM and only two
	// megabytes of it, where the S3 boards around it carry eight octal.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14, LED: 35,
		PSRAMMB:  2,
		Verified: true,
	},
	// The V3's panel exactly: same controller, same address, same pins.
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 35},
			{Kind: Button, Name: "PRG", Pin: 0, ActiveLow: true},
			// Heltec's own arithmetic: the reading is scaled by 5.42
			// against a 3.3 V range, so a cell at the top of it is 17.9 V -
			// a divider sized for a pack this board will never carry.
			{Kind: Meter, Name: "battery", Pin: 1, FullScaleMV: 17886},
		},
	},
	Notes: "The V3's successor, and the amplifier is the difference: a GC1109 " +
		"takes the chip's 10 dBm to about 22 at the antenna. It is switched by the " +
		"radio's own DIO2 rather than by a firmware GPIO, so the fault that costs a " +
		"T096 its output cannot happen here - which is also why no front-end module " +
		"is declared. The stock spring antenna is still well below a dipole.",
}
