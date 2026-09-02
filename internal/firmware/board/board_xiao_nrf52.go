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
	Notes: "Same nRF52840 core as the RAK_4631, and wired for the same path. " +
		"Named for the build that produces it rather than for the chip on it: " +
		"this was Xiao_nRF52840 here and Xiao_nrf52 in the release, and no " +
		"image was ever found for it. " +
		"Genuinely low sleep current, which makes it the one board here where " +
		"duty-cycling buys a great deal.",
}
