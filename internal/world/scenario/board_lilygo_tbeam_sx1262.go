// The Tbeam_SX1262, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var tbeamSX1262Board = Board{
	Name: "Tbeam_SX1262", MCU: "ESP32", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 22, FeedlineDB: 1.2, AntennaDBi: 2,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 800,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.0},
	Emulated: true,
	// Pins from variants/lilygo_tbeam_SX1262/platformio.ini. The plain ESP32,
	// so no PSRAM to declare - the Supreme is the variant that has it.
	QEMU: &QEMUWiring{
		Machine: "esp32", SPI: 2, NSS: 18, Busy: 32, DIO1: 33, LED: 4,
	},
	Hardware: &Panel{
		Screen: &Screen{
			Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C,
			WidthPx: 128, HeightPx: 64, Ink: Mono,
		},
		Parts: []Part{
			{Kind: Lamp, Name: "TX", Pin: 4},
			{Kind: Button, Name: "user", Pin: 38, ActiveLow: true},
		},
	},
	Notes: "An SMA connector, so the antenna figure is for a half-wave whip " +
		"rather than anything the board provides; fit something else and the " +
		"figure is wrong. Sleep current is the board's, not the MCU's: the AXP192 " +
		"power management, the GPS and the charge LED are all still drawing. The " +
		"18650 holder takes an unprotected cell, which is why the cutoff here is " +
		"3.0 V rather than the 3.2 V used for a protected pack.",
}
