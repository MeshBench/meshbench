// The RAK_4631, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one cannot
// reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

// rak4631Board is the first nRF52 board to run its published firmware here.
//
// One board, one profile. There were two for a while - this one and a "RAK4631"
// with no underscore - and they disagreed: this one booted, and the other sat in
// the matrix reporting that its wiring had never been verified. The name that
// survived is the one the release publishes, because that is the name an image
// is fetched by.
//
// Verified the same way the E22 was: its own image, off the flasher, booting
// and putting an advert on the channel - MBR to SoftDevice to application, then
// SetStandby, SetDIO3AsTCXOCtrl, SetPacketType(LoRa) and a 127-byte advert.
var rak4631Board = Board{
	Name:   "RAK_4631",
	MCU:    "nRF52840",
	Radio:  "SX1262",
	Vendor: "RAKwireless",

	MaxTxDBm:       22,
	FeedlineDB:     0.8,
	AntennaDBi:     2.15,
	SensitivityDBm: -137,
	NoiseFigureDB:  6,
	SleepUA:        20,

	Emulated: true,
	Battery: energy.Battery{
		Chemistry: energy.LiIon, CapacityMAh: 3400, Cells: 1, CutoffV: 3.1,
	},

	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio1",
		NssPin:   10,
		IrqPort:  "gpio1",
		IrqPin:   15,
		// The Adafruit nRF52 core builds Serial as a TinyUSB CDC device, so the
		// firmware's console is on USB and not on this part's UART.
		ConsoleOnUSB: true,
	},

	// What the board shows and what can be pressed on it, from variants/rak4631
	// in MeshCore: variant.h for the pins, RAK4631Board.h for the battery
	// arithmetic, platformio.ini for which parts a build actually drives, and
	// helpers/ui/SSD1306Display.h for the address and the panel's size.
	//
	// Pins are the flat numbering the nRF52 core and the variant both use:
	// P0.x is x and P1.x is 32+x.
	Hardware: &Panel{
		// Every RAK_4631 env in the tree builds SSD1306Display, so the published
		// image drives one whether a WisBlock has the OLED module fitted or not.
		// It is on the first I2C bus, PIN_WIRE_SDA 13 and PIN_WIRE_SCL 14.
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			// LED_GREEN and LED_BLUE, P1.03 and P1.04. LED_STATE_ON is 1, so
			// both light on a high, unlike every Heltec board here.
			{Kind: Lamp, Name: "green", Pin: 35},
			{Kind: Lamp, Name: "blue", Pin: 36},
			// The WisBlock base board's button, P0.09. Declared because it is on
			// the board somebody is holding, but read only by the companion and
			// terminal builds: the repeater env this catalogue fetches does not
			// define PIN_USER_BTN, so pressing it there is a press nothing is
			// listening for. That is the board, not a fault in the model.
			{Kind: Button, Name: "user", Pin: 9, ActiveLow: true},
			// getBattMilliVolts averages eight 12-bit conversions on P0.05 and
			// scales by 3 * 1.73 * 1.187 * 1000 / 4096, so a cell filling the
			// converter is 4095 * 6160.53 / 4096 millivolts.
			{Kind: Meter, Name: "battery", Pin: 5, FullScaleMV: 6159},
		},
	},

	Notes: "The reference repeater, and the first board here to run its own " +
		"published image. Ships with an external whip, so the antenna figure " +
		"assumes a half-wave dipole rather than the board. " +
		"Published .uf2 images are linked above a Nordic SoftDevice, fetched " +
		"from Nordic's own site rather than bundled - Nordic has confirmed " +
		"emulating it for firmware testing is not a licensing problem " +
		"(docs/licence.md). The radio is on SPIM3, which stock Renode does not model.",
}
