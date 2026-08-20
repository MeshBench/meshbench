// The Station_G2, as a hardware profile.
//
// One board to a file: a board is edited on its own, so a change to one
// cannot reach another by accident.
package scenario

import "github.com/MeshBench/meshbench/internal/mesh/energy"

var stationG2Board = Board{
	Name: "Station_G2", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
	MaxTxDBm: 30, FeedlineDB: 1.5, AntennaDBi: 2.15,
	SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 5000,
	Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
	Emulated: false,
	Notes: "Mains-powered with an external PA, so it is the only board here that " +
		"can legally run 30 dBm where the band plan allows it — and the only one " +
		"whose sleep current does not matter. Check the licence conditions before " +
		"simulating it at full power.",
}
