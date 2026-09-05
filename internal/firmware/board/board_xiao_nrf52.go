// The Xiao_nrf52, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package board

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var xiaoNrf52Board = Board{
	Name: "Xiao_nrf52", MCU: "nRF52840", Radio: "SX1262", Vendor: "Seeed",
	MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
	SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 5,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
	Emulated: true,
	// variants/xiao_nrf52/platformio.ini names the pins by the board's own
	// labels - P_LORA_NSS=D4, P_LORA_DIO_1=D1 - which variant.cpp's pin map
	// resolves to P0.04 and P0.03.
	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio0",
		NssPin:   4,
		IrqPort:  "gpio0",
		IrqPin:   3,
		// The Adafruit nRF52 core builds Serial as a TinyUSB CDC device, so the
		// firmware's console is on USB and not on this part's UART.
		ConsoleOnUSB: true,
	},
	// What the board shows and what can be pressed on it, from
	// variants/xiao_nrf52 in MeshCore: variant.h for the names,
	// XiaoNrf52Board.cpp for the battery arithmetic, and platformio.ini for the
	// display, which is NullDisplayDriver - so there is no panel to declare.
	//
	// Pins here are not the flat numbering the Heltec and RAK variants use.
	// This variant numbers by the board's own labels and variant.cpp's
	// g_ADigitalPinMap resolves them, the same map that turns the radio's
	// D4 and D1 into P0.04 and P0.03 above. Every pin below is the resolved
	// one, because that is what the machine wires.
	Hardware: &Panel{
		Parts: []Part{
			// PIN_LED is LED_RED, which is D11 and so P0.26. LED_STATE_ON is 0,
			// so it lights on a low. The green and blue beside it are declared
			// by the variant and driven only on the way to powering off.
			{Kind: Lamp, Name: "LED", Pin: 26},
			// PIN_BUTTON1 is D0, which is P0.02 - a header pin rather than a
			// switch fitted to the board. The XIAO carries only a reset button,
			// so this is what the firmware reads as its user button and not
			// something a person finds under a thumb.
			{Kind: Button, Name: "user (D0)", Pin: 2, ActiveLow: true},
			// getBattMilliVolts reads D32, which is P0.31, and scales by the
			// 1M/512k divider against a 3.0 V reference: raw * 3.0 * 3.0 / 4.096
			// millivolts, so a cell filling a 12-bit converter is 8997.
			//
			// The reading is gated on VBAT_ENABLE, D14 and so P0.14, which the
			// board pulls low for the conversion.
			{Kind: Meter, Name: "battery", Pin: 31, FullScaleMV: 8997},
		},
	},

	Notes: "Same nRF52840 core as the RAK_4631, and wired for the same path. " +
		"Named for the build that produces it rather than for the chip on it: " +
		"this was Xiao_nRF52840 here and Xiao_nrf52 in the release, and no " +
		"image was ever found for it. " +
		"Genuinely low sleep current, which makes it the one board here where " +
		"duty-cycling buys a great deal.",
}
