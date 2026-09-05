// The Heltec_t096, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecT096Board = Board{
	// Carries the front-end module MeshCore 1.17.1's transmit fix was about.
	Name: "Heltec_t096", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -1,
	SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
	Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
	// Runs under Renode, and measured doing it: build, boot, radio, tx, rx and
	// still alive at 392 s. It does not relay, which it shares with three other
	// nRF52 boards that are marked emulated anyway - this field is whether the
	// firmware runs at all, not whether the mesh stack does everything.
	Emulated: true,
	// A KCT8103L, switched by three GPIOs: an LDO enable, a shutdown line
	// and a transmit/receive path select. The gain figure is upstream's own:
	// variants/heltec_t096/platformio.ini sets LORA_TX_POWER=9 against
	// MAX_LORA_TX_POWER=22 and says "9dBm + ~13dB KCT8103L gain".
	FEM: &FEM{TxGainDB: 13, TxLossDB: 0},
	// variants/heltec_t096/platformio.ini: P_LORA_NSS=5, P_LORA_DIO_1=21.
	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio0",
		NssPin:   5,
		IrqPort:  "gpio0",
		IrqPin:   21,
		// The user button, PIN_BUTTON1 = P1.10. This variant configures it
		// as a plain INPUT and relies on the board's pull-up, so it has to be
		// held high here or the firmware reads a long press and powers off.
		IdleHighPins: []GPIOPin{{Port: "gpio1", Pin: 10}},
		// The Adafruit nRF52 core builds Serial as a TinyUSB CDC device, so the
		// firmware's console is on USB and not on this part's UART.
		ConsoleOnUSB: true,
	},
	// What the board shows and what can be pressed on it, from
	// variants/heltec_t096 in MeshCore: variant.h for the pins, T096Board.cpp
	// for the battery arithmetic, and helpers/ui/ST7735Display.cpp for the
	// panel, which it drives through TFT_eSPI(160, 80).
	//
	// Pins are the flat numbering the nRF52 core and the variant both use:
	// P0.x is x and P1.x is 32+x. So the user button at 42 is P1.10, which is
	// the same line the Renode wiring above holds high.
	Hardware: &Panel{
		// 160x80 is the glass. MeshCore's own UI draws on the 128x64 surface
		// ST7735Display declares to it and the driver scales, but a picture of
		// this board is a picture of the panel.
		//
		// The build sets DISPLAY_ROTATION=1, so the long axis is across.
		Screen: &Screen{
			Controller: "ST7735", Bus: BusSPI, CS: 22, DC: 15,
			WidthPx: 160, HeightPx: 80, Ink: RGB565,
		},
		Parts: []Part{
			// LED_BUILTIN, P0.28. LED_STATE_ON is 1 here, so it lights high.
			{Kind: Lamp, Name: "LED", Pin: 28},
			// PIN_BUTTON1, P1.10, read against the board's own pull-up.
			{Kind: Button, Name: "user", Pin: 42, ActiveLow: true},
			// getBattMilliVolts reads a 12-bit conversion on P0.03 against the
			// internal 3.0 V reference and scales it by 4.9, the same divider
			// the T114 carries, so full scale is 4095 * 3000/4096 * 4.9 mV.
			//
			// Gated on PIN_BAT_CTL, P1.15, which the board raises for the
			// reading and drops after.
			{Kind: Meter, Name: "battery", Pin: 3, FullScaleMV: 14696},
		},
	},

	Notes: "The board whose transmit failure 1.17.1 fixed: PIN_SPI1_MISO was -1 " +
		"against a 48-entry pin map, and the out-of-bounds read left the " +
		"module's transmit enable undriven. The chip is compiled for 9 dBm and " +
		"the module carries it to about 22, so a firmware that does not switch " +
		"the module in is 13 dB down with nothing in the radio's registers to " +
		"say so. MaxTxDBm here is the antenna figure, not the chip's. Antenna " +
		"and sleep figures are taken from the comparable nRF52840 boards rather " +
		"than from this board's own schematic, and should be checked before " +
		"either is trusted.",
}
