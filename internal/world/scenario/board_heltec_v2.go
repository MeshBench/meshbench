// The Heltec_v2, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var heltecV2Board = Board{
	Name: "Heltec_v2", MCU: "ESP32", Radio: "SX1276", Vendor: "Heltec",
	MaxTxDBm: 20, FeedlineDB: 1.2, AntennaDBi: -1,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 250,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
	Emulated: false,
	Notes: "Carries an SX1276, not an SX1262, despite sitting next to the V3 in " +
		"every shop. Its firmware speaks SX127x register access rather than " +
		"SX126x commands, so the radio model does not answer it. Recorded here " +
		"because the name invites exactly that mistake.",
}
