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
	},
	Notes: "nRF52840 with a display, which is why sleep current is not the MCU's own.",
}
