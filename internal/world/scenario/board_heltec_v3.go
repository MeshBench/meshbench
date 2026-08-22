// The Heltec_v3, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecV3Board = Board{
	Name: "Heltec_v3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/heltec_v3/platformio.ini - raw GPIO numbers on an
	// ESP32. No PSRAM on this board, so nothing to declare.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 8, Busy: 13, DIO1: 14,
	},
	// A 128x64 OLED on the bus the firmware brings up, a transmit lamp, and
	// the program button - the pin whose stuck-low reading used to power this
	// board off two minutes into every run.
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
	Notes: "Very common and not a good repeater: the stock spring antenna is well " +
		"below a dipole, and sleep current is dominated by the board rather than " +
		"the MCU. Its radio has a bus now - the machine builds GPSPI2 and offers " +
		"the 49 GPIOs this part has rather than the ESP32's 40 - but the published " +
		"image still asserts inside ESP-IDF's own startup, before MeshCore runs.",
}
