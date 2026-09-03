package board

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

	// Hardware is what this board shows and what somebody can press on it -
	// its screen, its lamps, its buttons. Nil where nobody has established
	// it, which is not the same as a board that carries nothing: that one
	// declares an empty panel and says so.
	//
	// Here rather than in the interface because none of it is a fact about
	// the interface. A lamp's pin is a property of the board, and the
	// emulator reads the same declaration to know which output to watch.
	Hardware *Panel

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
// GPIOPin is a pin as Renode addresses it: the port's name and the pin within
// it, not the flat number an Arduino core uses.
type GPIOPin struct {
	Port string
	Pin  int
}

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
	// packet arrived or a transmission finished. Every board needs it - the
	// ESP32's QEMUWiring carries the same line under a flat pin number - and a
	// board wired without it hears everything and forwards nothing.
	IrqPort string
	IrqPin  int

	// IdleHighPins are inputs the board holds high with an external pull-up,
	// which Renode has no notion of.
	//
	// An undriven input reads low here, and a low on a button pin is a button
	// held down. The Heltec T114 configures its user button as a plain INPUT and
	// leans on the board's own pull-up, so MeshCore saw a a thousand-millisecond
	// long press within a second of boot, printed "Powering Off" and shut the
	// node down before it could relay anything. The RAK4631 has no user button
	// and was never affected, which is why this looked like a fault in the radio
	// the two boards share.
	//
	// The same shape as the ESP32-S3's GPIO0 strapping pin in
	// docs/emulated-published-firmware.md: an input nobody drives is not a zero,
	// it is whatever the board wired it to.
	IdleHighPins []GPIOPin

	// ConsoleOnUSB says the firmware's Serial is the USB device rather than
	// UART0, as it is on the QEMU boards that carry the same field.
	//
	// It costs the console outright here, where on an ESP32 it only moves it:
	// the emulator models no USB device for this part, so a board that prints
	// over USB cannot be heard or typed at whatever is attached to its UART.
	// UART0 on such a board is not idle, which is the trap - a RAK4631 opens it
	// as Serial1 to look for a GPS - so a console attached there would carry
	// somebody else's traffic and answer nothing.
	ConsoleOnUSB bool
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

	// SPI is which controller the radio hangs off, numbered as the chip
	// numbers its controllers rather than as Arduino names them.
	//
	// Arduino's default-constructed SPIClass is HSPI, and HSPI is not the same
	// peripheral on the two parts: on an ESP32 it is controller 2, on an
	// ESP32-S3 it is controller 3. Every board here takes that default, so the
	// ESP32 boards say 2 and the S3 boards say 3, and a board that says the
	// wrong one produces a driver that reports no chip - which reads as a
	// board with no radio fitted rather than as a wrong number.
	SPI int

	// NSS and Busy are GPIOs, not the SPI controller's own lines: RadioLib
	// toggles the chip select by hand. NSS is what frames a command, since the
	// controller clocks bytes out one transfer at a time.
	NSS  int
	Busy int

	// DIO1 is the radio's interrupt line, and the firmware learns a packet
	// arrived from nowhere else: MeshCore's receive path is gated on a flag
	// set only by the packet-received ISR this pin fires. A board wired
	// without it receives perfectly and forwards nothing, which is how five
	// boards spent a long time looking like they had a routing problem.
	//
	// Zero means none recorded, and leaves the line unwired rather than
	// raising edges on a GPIO the firmware is using for something else.
	DIO1 int

	// LED is the pin the firmware blinks, where the board has one worth
	// showing. Zero means none recorded.
	LED int

	// PSRAMMB is the external RAM the board carries, in megabytes, or zero on
	// a board with none.
	//
	// Not a detail the radio cares about, and not optional either: a firmware
	// built for a board with PSRAM calls the driver at startup, and one that
	// finds no chip fails initialisation and asserts. The Xiao S3 WIO does
	// exactly that - "SPI RAM enabled but initialization failed. Bailing out."
	// - and reboots for ever without reaching MeshCore at all.
	PSRAMMB int

	// PSRAMOctal says that RAM is an octal (OPI) part rather than a quad one.
	// The firmware probes for the one it was built against and reports a quad
	// chip as missing entirely.
	PSRAMOctal bool

	// FEM is the GPIO carrying the front-end module's transmit enable, zero on
	// a board without one. RadioLib drives it from its RF-switch table rather
	// than the firmware driving it directly, so it moves on every transition
	// between receive and transmit.
	FEM int

	// ConsoleOnUSB says the application's Serial is the USB Serial/JTAG
	// peripheral rather than UART0.
	//
	// Which it is follows from the build, not from the silicon: a variant
	// compiled with ARDUINO_USB_CDC_ON_BOOT puts Arduino's Serial on the USB
	// port, and every one of MeshCore's own builds for such a board does. It
	// has to be recorded because the two are different pieces of hardware and
	// the emulator has to be told which one to hand the console to - a board
	// listening on the wrong one boots, prints its ROM banner to a file and
	// then appears to fall silent, which is exactly how the T-Deck read.
	//
	// UART0 keeps its own log either way: the ROM bootloader prints there
	// before any of this is configured, and that output is what says whether a
	// board started at all.
	ConsoleOnUSB bool

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

// Boards returns the profiles, sorted.
func Boards() []Board {
	out := make([]Board, len(boards))
	copy(out, boards)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// renamedBoards maps names this project used to publish to the ones it uses now.
//
// A fixture on someone's disk names its boards, and renaming one here would
// otherwise stop that fixture loading with "no board profile named". The names
// on the left were ours; the names on the right are the ones the firmware
// release publishes, which is what an image is fetched by.
var renamedBoards = map[string]string{
	"RAK4631":       "RAK_4631",
	"Xiao_nRF52840": "Xiao_nrf52",
}

// BoardByName looks one up, case-insensitively.
func BoardByName(name string) (Board, error) {
	for from, to := range renamedBoards {
		if strings.EqualFold(from, name) {
			name = to
			break
		}
	}
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
	"Heltec_t114",
	"Heltec_t096",
	// Both were unreachable until they were named for the build that makes
	// their image, and both went straight through on the first probe after
	// that: build, boot, radio, tx, rx, flood.
	"Xiao_nrf52",
	"Heltec_mesh_solar",

	// The ESP32 side, added when the board probe agreed about all of them.
	//
	// The bar is what the six above cleared: the published image builds,
	// boots, brings its radio up, transmits unprompted and hears a neighbour.
	// It is deliberately not the flood column, which fails for most of this
	// list including boards that were on it from the start - that row is
	// measuring the repeater's own forwarding policy against a stimulus it
	// half-duplexes through, and it has never been the question of whether
	// the wiring is right.
	//
	// The T-Deck was the last of them and failed reception until the day the
	// converter, the analog bus and the front end were answered; it passes
	// now, which is what put it here rather than a wish to run wadamesh on
	// something.
	"Heltec_v3",
	"heltec_v4",
	"Heltec_WSL3",
	"Heltec_Wireless_Tracker",
	"LilyGo_T3S3_sx1262",
	"LilyGo_TDeck",
	"Ebyte_EoRa-S3",
	"Xiao_S3",
	"Xiao_S3_WIO",
	"RAK_3112",
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
			// No longer "there is no bus for the radio": the fork's S3 machine
			// now builds GPSPI2, offers the 49 GPIOs the part has rather than
			// the ESP32's 40, and can be given an octal PSRAM. An image gets as
			// far as ESP-IDF's own startup and asserts there, before MeshCore
			// runs - the same line on two different boards, one of which has no
			// PSRAM at all, so it is not that either.
			blocked[b.Name] = b.MCU + ": boots, then asserts inside ESP-IDF startup"
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
