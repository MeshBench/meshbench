// The Heltec_t114, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecT114Board = Board{
	Name: "Heltec_t114", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 0,
	SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.1},
	Emulated: true,
	// Pins from the board's own build: variants/heltec_t114/platformio.ini
	// sets P_LORA_NSS=24 and P_LORA_DIO_1=20, both on gpio0. The radio is
	// on SPIM3, like the RAK4631; the display is the thing on SPIM2, and
	// the firmware blocks on that too until it is given its EasyDMA half.
	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio0",
		NssPin:   24,
		IrqPort:  "gpio0",
		IrqPin:   20,
		// The user button, PIN_BUTTON1 = 42 in the flat numbering, which is
		// P1.10. variants/heltec_t114/variant.cpp configures it as a plain
		// INPUT and relies on the board's pull-up, so it has to be held high
		// here or the firmware reads a long press and powers the node off.
		IdleHighPins: []GPIOPin{{Port: "gpio1", Pin: 10}},
		// The Adafruit nRF52 core builds Serial as a TinyUSB CDC device, so the
		// firmware's console is on USB and not on this part's UART.
		ConsoleOnUSB: true,
	},
	// What the board shows and what can be pressed on it, from
	// variants/heltec_t114 in MeshCore: variant.h for the pins, T114Board.h
	// for the battery arithmetic, and ST7789Display.h for the panel's size.
	//
	// Pins are the flat numbering the nRF52 core and the variant both use:
	// P0.x is x and P1.x is 32+x. So the user button at 42 is P1.10, which is
	// the same line the Renode wiring above holds high.
	Hardware: &Panel{
		// The env this board's image is built from - Heltec_t114_repeater -
		// extends Heltec_t114_with_display, so the published image drives an
		// ST7789. The "without display" envs beside it are a different build
		// and are not what the catalogue fetches.
		//
		// 240x135 is the panel; the driver presents a 128x64 surface to
		// MeshCore's own UI and scales. The size here is the glass, because
		// that is what a picture of it is.
		Screen: &Screen{
			Controller: "ST7789", Bus: BusSPI, CS: 11, DC: 12,
			WidthPx: 240, HeightPx: 135, Ink: RGB565,
		},
		Parts: []Part{
			// P_LORA_TX_LED=35 in the build, driven LOW to light: the board
			// turns it on before a transmission and off after.
			{Kind: Lamp, Name: "TX", Pin: 35},
			// PIN_BUTTON1, configured as a plain INPUT against the board's own
			// pull-up, so it reads low when held.
			{Kind: Button, Name: "user", Pin: 42, ActiveLow: true},
			// getBattMilliVolts reads a 12-bit conversion against the internal
			// 3.0 V reference and scales it by 4.9, so a cell filling the
			// converter is 4095 * 3000/4096 * 4.9 millivolts.
			//
			// It is gated: the divider is only connected while pin 6 is high,
			// which the board raises for the reading and drops after. Nothing
			// models an nRF52 converter yet, so the board view says so on this
			// row rather than reporting a voltage nobody measured.
			{Kind: Meter, Name: "battery", Pin: 4, FullScaleMV: 14696},
		},
	},
	Notes: "nRF52840 with a display, which is why sleep current is not the MCU's own.",
}
