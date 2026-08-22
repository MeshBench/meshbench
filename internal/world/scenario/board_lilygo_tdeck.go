// The LilyGo_TDeck, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var lilygoTDeckBoard = Board{
	Name: "LilyGo_TDeck", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: 0,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 2000,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: true,
	// Pins from variants/lilygo_tdeck/platformio.ini. The radio's clock and
	// data lines are the display's - both are on the controller Arduino calls
	// HSPI, told apart only by which chip select is low.
	QEMU: &QEMUWiring{
		Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13, DIO1: 45,
		PSRAMMB: 8, PSRAMOctal: true,
	},
	// A colour panel on the radio's own bus, with the keyboard and the touch
	// layer on I2C beside it. The keyboard is not a matrix: it is a second
	// microcontroller with its own firmware, answering at 0x55 with the
	// character last pressed, which is why it is declared by address rather
	// than by pins.
	//
	// The trackball's four direction lines are still absent. MeshCore's own
	// variant declares only its click, so nothing here has established which
	// pins they are, and a part declared on a guess would look like a part
	// that is broken.
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "ST7789", Bus: BusSPI, CS: 12, DC: 11,
			WidthPx: 320, HeightPx: 240, Ink: RGB565,
		},
		Parts: []Part{
			{Kind: Keys, Name: "keyboard", Bus: BusI2C, Addr: 0x55},
			{Kind: Touch, Name: "GT911", Bus: BusI2C, Addr: 0x5D},
			{Kind: Button, Name: "trackball click", Pin: 0, ActiveLow: true},
		},
	},
	Notes: "A handheld rather than a repeater: a colour touchscreen, a keyboard " +
		"that is a second microcontroller on the I2C bus, a trackball and a card " +
		"slot. The sleep figure reflects all of it and makes this a poor node to " +
		"leave somewhere. Its display shares the radio's SPI controller, which is " +
		"why that bus had to carry more than one device before this board could " +
		"show anything.",
}
