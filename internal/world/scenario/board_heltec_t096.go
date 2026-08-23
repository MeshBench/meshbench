// The Heltec_t096, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

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
