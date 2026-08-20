package scenario

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/mesh/energy"
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

	// Emulated reports whether MeshcoreSim can run this board's firmware under
	// emulation today. Stated rather than implied, because a scenario built
	// around a board that cannot be emulated should say so at build time and
	// not fail at run time.
	Emulated bool

	// QEMU is how to wire this board up under emulation, where we can.
	//
	// Nil means we have not established it. It is deliberately per board rather
	// than per MCU: the radio sits on a different SPI controller and different
	// pins from one board to the next, and getting it wrong looks exactly like
	// a chip that is not there.
	QEMU *QEMUWiring

	// Renode is the ARM equivalent, for the nRF52 boards. A board has one or
	// the other, never both: which emulator can run it follows from its MCU.
	Renode *RenodeWiring

	// FEM is the board's front-end module, where it has one. Nil means the
	// radio drives the antenna directly.
	FEM *FEM

	// Notes carries anything an engineer would want to know before trusting a
	// figure here.
	Notes string
}

// FEM is a front-end module: an external amplifier and RF switch that the
// firmware brings into circuit by driving a GPIO, not through the radio.
//
// It is modelled because MaxTxDBm above is a claim about the board and not
// about the firmware running on it. A Heltec T096 is compiled for 9 dBm at the
// chip and reaches about 22 dBm at the antenna through a KCT8103L; a firmware
// that fails to switch the module in transmits at 9. Nothing in the radio's own
// registers says so, and before this the simulator reported 22 either way.
type FEM struct {
	// TxGainDB is what the module contributes on transmit while its enable line
	// is asserted.
	TxGainDB float64

	// TxLossDB is what the transmit path costs when it is not asserted. Small
	// where the module is only an amplifier and the signal leaks past it; large
	// where it is also the antenna switch, because then the path is not
	// connected at all.
	TxLossDB float64
}

// RenodeWiring is the same idea for the nRF52 boards, which run under Renode.
//
// Two of these mattered more than the rest and neither is guessable. The radio
// is on SPIM3 at 0x4002F000 - the high-speed controller the nRF52840 adds,
// which Renode does not model at all, so a node wired to any other controller
// transfers bytes into an address with nothing behind it. And chip select is a
// GPIO: Renode's SPI model never calls FinishTransmission, so without the NSS
// pin wired the chip takes bytes for ever and executes no command.
type RenodeWiring struct {
	// Platform is the base platform description, relative to Renode's own
	// directory.
	Platform string

	// SPIBase is where the controller the radio hangs off lives.
	SPIBase uint32

	// NssPort and NssPin are the GPIO carrying chip select: the port as Renode
	// names it, and the pin within that port. On a RAK4631 the radio's NSS is
	// P1.10, which is pin 42 in the flat numbering the Arduino core uses.
	NssPort string
	NssPin  int

	// IrqPort and IrqPin are DIO1, which is how the chip tells this MCU that a
	// packet arrived or a transmission finished. The ESP32 firmware polls the
	// IRQ register over SPI instead and does not need this.
	IrqPort string
	IrqPin  int
}

// QEMUWiring is everything an emulated node needs beyond its firmware image.
//
// The values come from the board's own MeshCore variant configuration, because
// that is what the firmware was compiled against. Guessing them from the MCU
// does not work: "Heltec V2" and "Heltec V3" differ by an SX1276 against an
// SX1262, and a board whose radio is on the wrong SPI controller produces a
// driver that reports no chip, which reads as a broken emulator rather than a
// wrong pin number.
type QEMUWiring struct {
	// Machine is the QEMU machine type, e.g. "esp32".
	Machine string

	// SPI is which controller the radio hangs off. Arduino's default-constructed
	// SPIClass is HSPI, which is controller 2, and that is what most of these
	// boards get by passing it to std_init.
	SPI int

	// NSS and Busy are GPIOs, not the SPI controller's own lines: RadioLib
	// toggles the chip select by hand. NSS is what frames a command, since the
	// controller clocks bytes out one transfer at a time.
	NSS  int
	Busy int

	// LED is the pin the firmware blinks, where the board has one worth
	// showing. Zero means none recorded.
	LED int

	// FEM is the GPIO carrying the front-end module's transmit enable, zero on
	// a board without one. RadioLib drives it from its RF-switch table rather
	// than the firmware driving it directly, so it moves on every transition
	// between receive and transmit.
	FEM int

	// Verified records that firmware for this board has actually been booted
	// with this wiring and driven the radio, rather than the numbers having
	// been read off a config file and assumed.
	Verified bool
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
			"the MCU. Not emulated: QEMU's ESP32-S3 machine models only the flash SPI " +
			"controller, so there is no bus for the radio to sit on. The plain-ESP32 " +
			"machine models SPI0 to SPI3 and does work.",
	},
	{
		// The first board driven end to end under emulation, which is why it is
		// here rather than for being popular.
		Name: "Generic_E22_sx1262", MCU: "ESP32", Radio: "SX1262", Vendor: "Ebyte",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: 2.15,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 250,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: true,
		QEMU: &QEMUWiring{
			// FEM 13 is SX126X_TXEN, from the variant's platformio.ini. RadioLib
			// drives it from setRfSwitchPins(RXEN=14, TXEN=13), so it goes high
			// before SetTx and low again before SetRx.
			Machine: "esp32", SPI: 2, NSS: 18, Busy: 32, LED: 2, FEM: 13,
			Verified: true,
		},
		// The module's TXEN and RXEN, on MCU pins 13 and 14. No gain stage:
		// MeshCore compiles this variant for LORA_TX_POWER=22 and the E22's own
		// SX1262 produces it, so these switch the path rather than amplify it.
		// The loss is an RF switch's isolation and is a plausible figure rather
		// than a measured one - see Notes.
		FEM: &FEM{TxGainDB: 0, TxLossDB: 25},
		Notes: "An E22 module on a devkit rather than a product, so the antenna figure " +
			"assumes the external whip the module is designed for. The published " +
			"repeater image boots and runs RadioLib's full SX126x init sequence under " +
			"emulation: version read, LoRa mode, modulation and IRQ setup. " +
			"The 25 dB switch isolation is a plausible figure for an SPDT part at " +
			"868 MHz and has not been measured. Upstream also sets " +
			"SX126X_DIO2_AS_RF_SWITCH=true alongside the MCU pins, which its own " +
			"variant.h warns against - so on this board the path may be switched by " +
			"DIO2 whatever the MCU pins do, and a transmit-enable fault here would " +
			"be milder than the model says. The T096 is the honest case for that.",
	},
	{
		// Carries the front-end module MeshCore 1.17.1's transmit fix was about.
		// Not emulated yet: it is nRF52, so it wants Renode rather than QEMU.
		Name: "Heltec_t096", MCU: "nRF52840", Radio: "SX1262", Vendor: "Heltec",
		MaxTxDBm: 22, FeedlineDB: 1.0, AntennaDBi: -1,
		SensitivityDBm: -137, NoiseFigureDB: 6, SleepUA: 60,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 1000, Cells: 1, CutoffV: 3.1},
		Emulated: false,
		// A KCT8103L, switched by three GPIOs: an LDO enable, a shutdown line
		// and a transmit/receive path select. The gain figure is upstream's own:
		// variants/heltec_t096/platformio.ini sets LORA_TX_POWER=9 against
		// MAX_LORA_TX_POWER=22 and says "9dBm + ~13dB KCT8103L gain".
		FEM: &FEM{TxGainDB: 13, TxLossDB: 0},
		Notes: "The board whose transmit failure 1.17.1 fixed: PIN_SPI1_MISO was -1 " +
			"against a 48-entry pin map, and the out-of-bounds read left the " +
			"module's transmit enable undriven. The chip is compiled for 9 dBm and " +
			"the module carries it to about 22, so a firmware that does not switch " +
			"the module in is 13 dB down with nothing in the radio's registers to " +
			"say so. MaxTxDBm here is the antenna figure, not the chip's. Antenna " +
			"and sleep figures are taken from the comparable nRF52840 boards rather " +
			"than from this board's own schematic, and should be checked before " +
			"either is trusted.",
	},
	{
		Name: "Heltec_v2", MCU: "ESP32", Radio: "SX1276", Vendor: "Heltec",
		MaxTxDBm: 20, FeedlineDB: 1.2, AntennaDBi: -1,
		SensitivityDBm: -136, NoiseFigureDB: 7, SleepUA: 250,
		Battery:  energy.Battery{Chemistry: energy.LiIon, CapacityMAh: 2000, Cells: 1, CutoffV: 3.2},
		Emulated: false,
		Notes: "Carries an SX1276, not an SX1262, despite sitting next to the V3 in " +
			"every shop. Its firmware speaks SX127x register access rather than " +
			"SX126x commands, so the radio model does not answer it. Recorded here " +
			"because the name invites exactly that mistake.",
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

	rak4631Board,
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

// rak4631Board is the first nRF52 board to run its published firmware here.
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
	FeedlineDB:     0.6,
	AntennaDBi:     2.0,
	SensitivityDBm: -137,
	NoiseFigureDB:  6,

	Renode: &RenodeWiring{
		Platform: "platforms/cpus/nrf52840.repl",
		SPIBase:  0x4002F000,
		NssPort:  "gpio1",
		NssPin:   10,
		IrqPort:  "gpio1",
		IrqPin:   15,
	},

	Notes: "Published .uf2 images are linked above a Nordic SoftDevice, fetched " +
		"from Nordic's own site rather than bundled - Nordic has confirmed " +
		"emulating it for firmware testing is not a licensing problem " +
		"(docs/licence.md). The radio is on SPIM3, which stock Renode does not model.",
}

// EmulationVerified lists the boards whose firmware has actually been booted
// under emulation and driven its radio.
//
// The one place to edit as hardware becomes supported. Support is a claim about
// a specific board, not about its MCU: the radio sits on a different SPI
// controller and different pins from one board to the next, and a wrong pin
// produces a driver reporting no chip, which reads as a broken emulator rather
// than a wrong number. So a board joins this list when someone has watched its
// published image come up, and not before.
//
// The wiring itself lives on the Board, in QEMUWiring. This is the shorter
// question of whether anyone has confirmed it.
var EmulationVerified = []string{
	"Generic_E22_sx1262",
	"RAK_4631",
}

// EmulationSupported reports whether a board can be run under emulation today.
//
// Exported because the firmware catalogue filters published images with it: an
// image for a board with no verified wiring cannot be pointed at a radio, and
// offering it would fail when someone presses run rather than here.
func EmulationSupported(board string) bool { return isEmulationVerified(board) }

// isEmulationVerified reports whether a board is on that list.
func isEmulationVerified(name string) bool {
	for _, n := range EmulationVerified {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// EmulatableBoards is what the firmware picker should offer for an emulated
// node, and why the rest are missing.
//
// This replaced a version that returned names only. The reasons are the point:
// a board is missing for a specific reason and the operator should be told it.
//
// Returned together on purpose. A picker that silently lists three boards out of
// ninety reads as a broken feature; one that says "SX1268 radio, not modelled"
// reads as a fact, and the operator stops looking for the option.
func EmulatableBoards() (ok []Board, blocked map[string]string) {
	blocked = map[string]string{}
	for _, b := range Boards() {
		// Either wiring counts. Gating on QEMU alone meant a board wired for
		// Renode could never be emulable however thoroughly it had been
		// verified - RAK_4631 sat on EmulationVerified and still reported
		// "wiring not yet verified", because the only branch that could admit
		// it asked for the wrong emulator.
		switch {
		case (b.QEMU != nil || b.Renode != nil) && isEmulationVerified(b.Name):
			ok = append(ok, b)
		case b.QEMU != nil || b.Renode != nil:
			blocked[b.Name] = "wiring recorded but never booted"
		case b.Radio != "SX1262":
			blocked[b.Name] = b.Radio + " radio, not modelled"
		case b.MCU == "ESP32-S3" || b.MCU == "ESP32-C3" || b.MCU == "ESP32-C6":
			blocked[b.Name] = b.MCU + " has no general-purpose SPI in QEMU"
		case strings.HasPrefix(b.MCU, "nRF52"):
			// Not a licensing block - see docs/licence.md, DevZone case 362437.
			// Every other nRF52 board still needs the same Renode wiring
			// RAK4631 got: watched booting under its own published image
			// before it can join EmulationVerified.
			blocked[b.Name] = "nRF52 emulation wiring not yet verified for this board"
		default:
			blocked[b.Name] = "no emulation wiring established"
		}
	}
	return ok, blocked
}
