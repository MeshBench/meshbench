package scenario

import (
	"fmt"
	"sort"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/energy"
)

// Board is a hardware profile: what a given piece of hardware can actually do.
//
// Figures come from datasheets and published board schematics, not from
// measurement, and the difference matters. A datasheet transmit power is what
// the chip produces at its own pin; what leaves the antenna is that minus the
// board's own losses, and boards with an integrated antenna are frequently
// several dB worse than the number on the box.
type Board struct {
	Name   string
	MCU    string
	Radio  string
	Vendor string

	// MaxTxDBm at the radio pin, per the datasheet.
	MaxTxDBm float64

	// FeedlineDB is the loss between chip and antenna connector: matching
	// network, RF switch and trace. Small, and the reason a board's real
	// radiated power is never its datasheet figure.
	FeedlineDB float64

	// AntennaDBi for the antenna the board ships with. Negative is normal for a
	// chip or PCB antenna, and a positive default here would flatter every
	// result computed with it.
	AntennaDBi float64

	// SensitivityDBm at SF12/BW125, the figure vendors quote.
	SensitivityDBm float64

	// NoiseFigureDB of the receive chain.
	NoiseFigureDB float64

	// Battery and Panel describe what the board ships with, where it ships with
	// anything. A zero PeakW means no solar.
	Battery energy.Battery
	Panel   energy.Panel

	// SleepUA is the board's own deep-sleep current, which is where the
	// datasheet and reality diverge most: an MCU at 3 µA on a board with a
	// linear regulator and a power LED draws hundreds.
	SleepUA float64

	// Emulated reports whether MeshBench can run this board's firmware under
	// emulation today. Stated rather than implied, because a scenario built
	// around a board that cannot be emulated should say so at build time and
	// not fail at run time.
	Emulated bool

	// Notes carries anything an engineer would want to know before trusting a
	// figure here.
	Notes string
}

// Load returns this board's electrical model.
func (b Board) Load() energy.Load {
	l := energy.SX1262Load()
	if b.SleepUA > 0 {
		l.SleepUA = b.SleepUA
	}
	return l
}

// RadiatedDBm is what actually leaves the antenna at a given drive level.
//
// The number people quote is MaxTxDBm; the number that reaches the far end is
// this. On a board with a chip antenna the difference is most of a decade of
// range.
func (b Board) RadiatedDBm(driveDBm float64) float64 {
	if driveDBm > b.MaxTxDBm {
		driveDBm = b.MaxTxDBm
	}
	return driveDBm - b.FeedlineDB + b.AntennaDBi
}

// boards is the starter set: the hardware people actually deploy on a UK mesh.
//
// Deliberately small. Seven profiles that are right are worth more than forty
// that were guessed at, and every figure here can be traced to a datasheet or a
// published schematic. Anything uncertain is in Notes rather than smoothed over.
var boards = []Board{
	{
		Name: "RAK4631", MCU: "nRF52840", Radio: "SX1262", Vendor: "RAKwireless",
		MaxTxDBm: 22, FeedlineDB: 0.8, AntennaDBi: 2.15,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 20,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3400, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		Notes: "The reference repeater. Runs under Renode today, including the shipped " +
			"image's MBR and SoftDevice. Ships with an external whip, so the antenna " +
			"figure assumes a half-wave dipole rather than the board.",
	},
	{
		Name: "Heltec_v3", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 200,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Very common and not a good repeater: the stock spring antenna is well " +
			"below a dipole, and sleep current is dominated by the board rather than " +
			"the MCU. Emulation blocked on an ESP32-side SX1262 model.",
	},
	{
		Name: "Heltec_mesh_solar", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 21, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 150,
		Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 3000, Cells: 1, CutoffV: 3.2},
		Panel: energy.Panel{PeakW: 1.5, TiltDeg: 0, AzimuthDeg: 180,
			SoilingFactor: 0.8, ChargeEfficiency: 0.75},
		Emulated: false,
		Notes: "Integrated panel mounted flat, which at UK latitudes is the worst case " +
			"in December — see internal/energy. PWM charging, so the efficiency figure " +
			"is not MPPT.",
	},
	{
		Name: "Xiao_S3_WIO", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "Seeed",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 50,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes:    "Tiny, and the antenna figure reflects it. A companion, not a repeater.",
	},
	{
		Name: "Xiao_nRF52840", MCU: "nRF52840", Radio: "SX1262", Vendor: "Seeed",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -2,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 5,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		Notes: "Same nRF52840 core as the RAK4631, so it emulates on the same path. " +
			"Genuinely low sleep current, which makes it the one board here where " +
			"duty-cycling buys a great deal.",
	},
	{
		Name: "Heltec_t114", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 0,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.1},
		Emulated: true,
		Notes:    "nRF52840 with a display, which is why sleep current is not the MCU's own.",
	},
	{
		Name: "Station_G2", MCU: "ESP32-S3", Radio: "SX1262", Vendor: "LILYGO",
		MaxTxDBm: 30, FeedlineDB: 1.5, AntennaDBi: 2.15,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 5000,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 0, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Mains-powered with an external PA, so it is the only board here that " +
			"can legally run 30 dBm where the band plan allows it — and the only one " +
			"whose sleep current does not matter. Check the licence conditions before " +
			"simulating it at full power.",
	},
}

// Boards returns the profiles, sorted.
func Boards() []Board {
	out := make([]Board, len(boards))
	copy(out, boards)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// BoardByName looks one up, case-insensitively.
func BoardByName(name string) (Board, error) {
	for _, b := range boards {
		if strings.EqualFold(b.Name, name) {
			return b, nil
		}
	}
	var names []string
	for _, b := range boards {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return Board{}, fmt.Errorf("scenario: no board profile named %q; have %s",
		name, strings.Join(names, ", "))
}

// EmulatableBoards are the ones whose firmware can be run under emulation
// today. Worth asking before building a scenario around one that cannot.
func EmulatableBoards() []string {
	var out []string
	for _, b := range boards {
		if b.Emulated {
			out = append(out, b.Name)
		}
	}
	sort.Strings(out)
	return out
}
