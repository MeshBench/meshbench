// The RAK_4631, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one cannot
// reach another by accident.
package scenario

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
	},

	Notes: "The reference repeater, and the first board here to run its own " +
		"published image. Ships with an external whip, so the antenna figure " +
		"assumes a half-wave dipole rather than the board. " +
		"Published .uf2 images are linked above a Nordic SoftDevice, fetched " +
		"from Nordic's own site rather than bundled - Nordic has confirmed " +
		"emulating it for firmware testing is not a licensing problem " +
		"(docs/licence.md). The radio is on SPIM3, which stock Renode does not model.",
}
