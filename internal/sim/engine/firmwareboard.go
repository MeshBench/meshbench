package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Turning a board's profile into a machine.
//
// One declaration decides both what the interface draws and what the emulator
// is wired with, which is the whole reason it is a declaration: two lists that
// have to agree eventually do not, and a board whose profile said one
// controller while its firmware used another looked exactly like a board with
// no radio fitted.

// emulatedBackend builds the emulator side for a node that names a board.
//
// Everything it needs is already recorded: the board catalogue says where the
// radio sits on that hardware, and the firmware cache says where the image is.
// Failing here rather than later matters, because a wrong pin does not announce
// itself - it produces a driver reporting no chip, which reads as a broken
// emulator.
// runSeed is the run's own seed, which the node mixes into its radio's noise.
//
// Passed in rather than derived from the node's name alone: a name is stable
// across every run and every scenario, so seeding from it would give a node the
// same identity for ever and make two scenarios that differ only by seed
// produce the same keys. The rule is same seed, same scenario, same result -
// which means a different seed has to give a different answer.
func emulatedBackend(spec scenario.Node, allowUnverified bool, runSeed uint64) (*emulated.EmulatedNode, error) {
	board, err := hw.BoardByName(spec.Firmware.Board)
	if err != nil {
		return nil, err
	}
	if !allowUnverified && !hw.EmulationSupported(board.Name) {
		// Named with the way out of it. The gate is a curation claim - has
		// anybody watched this board's own image boot - and an operator who
		// wants to be the one doing the watching had no way to say so from
		// the refusal alone.
		return nil, fmt.Errorf("%s has no verified emulation wiring: nobody has "+
			"watched its own image boot here yet. Run the board probe for it "+
			"from the Bench view, or switch on unwatched wiring to run it "+
			"anyway and be the one who finds out", board.Name)
	}
	if board.QEMU == nil && board.Renode == nil {
		return nil, fmt.Errorf("%s names no emulator", board.Name)
	}

	// The image format follows the MCU, not a preference: the ESP32 boards
	// publish a merged .bin and the nRF52 ones a .uf2, and neither emulator
	// will take the other's.
	format := "bin"
	if board.Renode != nil {
		format = "uf2"
	}

	cache := firmware.DefaultCacheDir()
	// A companion is one application built for several transports; the role and
	// the transport are separate settings of it, not four role names. Resolve
	// them together so a node written the old way (role companion_radio_usb)
	// and one written the new way (role companion_radio, transport usb) name
	// the same image. Only the USB build is reachable here: its client arrives
	// over the serial port, which an emulator has, and Bluetooth is not
	// something we model - so a companion with no transport of its own is run
	// as the USB one.
	role, transport := spec.Firmware.Companion()
	// A BLE companion waits for a phone over Bluetooth, and there is none here.
	// Left to the image lookup it would fail with "no image in the cache",
	// which sends the operator downloading a build that could never run; say
	// what is actually wrong and what to use instead. Checked on the resolved
	// transport so a node written either way - the composite role, or the
	// transport field - is caught.
	if role == scenario.RoleCompanionRadio && transport == "ble" {
		return nil, fmt.Errorf(
			"%s is a Bluetooth companion (companion_radio_ble); the emulator models "+
				"no Bluetooth, so nothing could reach it - use companion_radio_usb instead",
			spec.Name)
	}
	if role == scenario.RoleCompanionRadio && transport == "" {
		transport = "usb"
	}
	img := emulated.BoardImage{
		Board:     board.Name,
		Role:      string(role),
		Version:   spec.Firmware.Version,
		Format:    format,
		Transport: transport,
	}
	src := emulated.BoardImagePath(cache, img)
	if _, err := os.Stat(src); err != nil {
		// Not where a download would have put it, so ask what the cache
		// actually holds.
		//
		// A downloaded image is named by convention and this found it. An
		// imported one is named after the label its importer chose, which is
		// the whole point of importing - so computing the path and giving up
		// meant a build you could see in the library, pin to a node, and never
		// run. The refusal even told you to download it, which is the one thing
		// that would not have helped.
		found := ""
		for _, in := range firmware.ListInstalled(cache) {
			if in.Board != img.Board || in.Version != img.Version {
				continue
			}
			// The role as the node names it, or that plus the transport a
			// published companion carries.
			if in.Role == img.Role || in.Role == img.Role+"_"+img.Transport {
				found = in.Path
				break
			}
		}
		if found == "" {
			return nil, fmt.Errorf("no %s image for %s %s in the cache - download "+
				"one from the firmware library, or import your own",
				board.Name, spec.Firmware.Role, spec.Firmware.Version)
		}
		src = found
	}

	dir := firmware.NodeWorkDir(spec.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Which emulator, and what only that emulator needs. Everything a board
	// declares - its buttons, its meter, its card, its receiver, its display -
	// is wired below this, once, for both: a part that reached the guest under
	// one machine and not the other is exactly the difference this window
	// exists to hide, and it was here for a year because the Renode branch
	// returned before any of it ran.
	var node *emulated.EmulatedNode
	if board.Renode != nil {
		node = &emulated.EmulatedNode{
			Emulator: emulated.Renode,
			Image:    src,
			// Published nRF52 images are linked above a Nordic SoftDevice,
			// which is fetched rather than bundled and so may not be here yet.
			// The refusal names it, because the alternative is a node that
			// boots into a fill pattern and looks like a broken emulator.
			SoftDeviceDir: emulated.SoftDeviceDir(),
			Platform:      board.Renode.Platform,
			SPIBase:       board.Renode.SPIBase,
			NssPort:       board.Renode.NssPort,
			NssPin:        board.Renode.NssPin,
			IrqPort:       board.Renode.IrqPort,
			IrqPin:        board.Renode.IrqPin,
			// Where this board's firmware puts Serial, as it is for the ESP32
			// boards below. On this emulator it decides whether the node has a
			// console at all, because nothing here models a USB device.
			ConsoleOnUSB: board.Renode.ConsoleOnUSB,
			IdleHighPins: idleHigh(board.Renode.IdleHighPins),
			NodeName:     spec.Name,
			// As on the QEMU node below: the name alone collides, because
			// every board the probe measures is called "bc-under-test".
			BoardName: board.Name,
			Position:  emulated.LatLon{Lat: spec.Position.Lat, Lon: spec.Position.Lon},
			RunSeed:   runSeed,
			Dir:       dir,
		}
		return withParts(node, board, spec, firmware.LoadBuildSettings(src))
	}

	// Padded once per node, beside its own working directory: QEMU takes only
	// 2, 4, 8 or 16 MB images and the size has to match the image header.
	//
	// Kept between runs, as a board's flash is. Rewritten every start, an
	// emulated node lost its identity, preferences and contacts each time it
	// was restarted - so a node configured over its console reverted the
	// moment somebody stopped and started it, and both arms of a comparison
	// began factory-fresh whether or not that was the intention. A different
	// build still gets a fresh chip: that is what reflashing a board is.
	// What has been decided about this build, read from beside the image
	// rather than from the board: the same hardware runs one image that needs
	// the coprocessors up at reset and another that would be flattered by it.
	set := firmware.LoadBuildSettings(src)

	padded := filepath.Join(dir, "flash.bin")
	if _, err := emulated.PadImageKeeping(src, padded); err != nil {
		return nil, err
	}

	node = &emulated.EmulatedNode{
		Emulator:   emulated.QEMU,
		Image:      padded,
		Machine:    board.QEMU.Machine,
		SPI:        board.QEMU.SPI,
		NSS:        board.QEMU.NSS,
		PSRAMMB:    board.QEMU.PSRAMMB,
		PSRAMOctal: board.QEMU.PSRAMOctal,
		FEM:        board.QEMU.FEM,
		Busy:       board.QEMU.Busy,
		DIO1:       board.QEMU.DIO1,
		NodeName:   spec.Name,
		// With the name and the run's seed, these decide the radio's noise and
		// so the identity the firmware generates. The name alone collides:
		// every board the probe measures is called "bc-under-test".
		BoardName: board.Name,
		Position:  emulated.LatLon{Lat: spec.Position.Lat, Lon: spec.Position.Lon},
		RunSeed:   runSeed,
		Dir:       dir,
		// Where this board's firmware puts Serial. On a board built with
		// ARDUINO_USB_CDC_ON_BOOT that is the USB peripheral, not UART0, and
		// a console handed to the wrong one is a board that boots and then
		// appears to say nothing.
		ConsoleOnUSB: board.QEMU.ConsoleOnUSB,
		// And what has been decided about this particular build.
		CoprocAtReset: set.CoprocAtReset,
	}
	return withParts(node, board, spec, set)
}

// gpsEpoch is the date and time the first sentence carries.
//
// Fixed rather than the wall clock, because determinism is a feature here: the
// same scenario run twice has to put out the same bytes, and a receiver
// stamping the real time would make every capture differ from every other. The
// date is arbitrary; that it never moves is not.
var gpsEpoch = time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

// meterReading is a converter channel and what it should read.
type meterReading struct {
	channel int
	raw     uint16
}

// MeterModelled reports whether this board's meter reaches a converter that is
// modelled, and which channel it lands on.
//
// Exported so the interface can ask the engine rather than keeping a second
// copy of which pins are converter inputs. It had one for a day and said "no
// converter modelled" about a board whose converter is modelled - a false
// claim in the one column that exists to be trusted.
func MeterModelled(board hw.Board) (channel int, ok bool) {
	m, have := batteryMeter(board)
	return m.channel, have
}

// batteryMeter is what the board's cell puts on its converter, at a full
// charge.
//
// The number the firmware sees rather than a voltage, because the arithmetic
// between the two is the board's - its divider against the converter's range -
// and it is declared beside the pin for that reason. A board that declares no
// meter gets no reading, which leaves the channel at nothing: honest about a
// cell nobody has measured, and still enough that the converter answers rather
// than leaving a firmware waiting for it.
func batteryMeter(board hw.Board) (meterReading, bool) {
	p := board.Hardware
	if p == nil {
		return meterReading{}, false
	}
	// The pin to channel mapping below is the ESP32-S3's, and the nRF52's is
	// a different set of pins reached through a different converter, so the
	// two are picked apart here rather than one of them being handed the
	// other's channel in silence.
	switch {
	case strings.EqualFold(board.MCU, "nRF52840"):
		return nrf52Meter(board)
	case !strings.EqualFold(board.MCU, "ESP32-S3"):
		return meterReading{}, false
	}
	for _, part := range p.PartsOfKind(hw.Meter) {
		if part.Pin == hw.PinNone || part.FullScaleMV <= 0 {
			continue
		}
		ch, ok := adc1Channel(part.Pin)
		if !ok {
			continue
		}
		// The emulated part now carries ADC calibration (BLK_VERSION_MAJOR=1
		// in eFuse BLK2), so the firmware converts the raw through ESP-IDF's
		// non-linear V1 curve rather than a linear default. Encode the raw as
		// the inverse of that exact curve, evaluated at the voltage the divider
		// puts on the pin, so the firmware reads the true cell voltage back.
		// The divider ratio is FullScaleMV/adc1Atten3FullScaleMV - the cell
		// voltage that fills the converter, over the pin voltage that does.
		pinMV := board.Battery.VoltageAt(1) * 1000 * adc1Atten3FullScaleMV / float64(part.FullScaleMV)
		return meterReading{channel: ch, raw: adc1Atten3RawForVoltage(pinMV)}, true
	}
	return meterReading{}, false
}

// adc1Channel is which of the first converter's channels a pin is, on the
// ESP32-S3: its ten inputs are GPIO 1 to GPIO 10, in order. A pin outside that
// is on the second converter or on none, and neither is modelled - so it is
// reported as no channel rather than as channel zero, which would put a
// reading on somebody else's pin.
func adc1Channel(pin int) (int, bool) {
	if pin < 1 || pin > 10 {
		return 0, false
	}
	return pin - 1, true
}

// adc1Atten3FullScaleMV is the pin voltage that fills ADC1 at 12 dB (atten 3)
// on the ESP32-S3 - the nominal full scale the board's FullScaleMV divider was
// chosen against. The battery meter is the only ADC input modelled and it is on
// this attenuation, so it is the only curve that needs inverting.
const adc1Atten3FullScaleMV = 3300.0

// adc1Atten3Voltage is ESP-IDF's calibration-V1 curve for ADC1 at atten 3 with
// the factory-baseline eFuse - the init codes and cal points that all-zero
// diffs produce, which is exactly what the emulated eFuse presents once
// BLK_VERSION_MAJOR is 1. It maps a raw converter reading to millivolts as
// esp_adc_cal_raw_to_voltage does on the part, so inverting it round-trips the
// firmware's own arithmetic and the reported voltage is unchanged. Transcribed
// from esp-idf (v5.5) components/esp_adc/esp32s3/curve_fitting_coefficients.c
// and components/efuse/esp32s3/esp_efuse_rtc_calib.c.
func adc1Atten3Voltage(raw float64) float64 {
	// First step: the linear reference through (digi 900, 850 mV).
	x := raw * 850.0 / 900.0
	// Second step: the reading-error polynomial for atten 3, five terms, with
	// the signs the part records them under.
	adcErr := -0.644403418269478 -
		0.0644334888647536*x +
		0.0001297891447611*x*x -
		7.0769718e-8*x*x*x +
		1.3515e-11*x*x*x*x
	return x - adcErr
}

// adc1Atten3RawForVoltage inverts adc1Atten3Voltage: the raw the converter must
// return for the firmware to read pinMV back. The curve is monotonic, so a
// bisection settles to the nearest code well inside one LSB.
func adc1Atten3RawForVoltage(pinMV float64) uint16 {
	lo, hi := 0.0, 4095.0
	if pinMV <= adc1Atten3Voltage(lo) {
		return 0
	}
	if pinMV >= adc1Atten3Voltage(hi) {
		return 4095
	}
	for i := 0; i < 24; i++ {
		mid := (lo + hi) / 2
		if adc1Atten3Voltage(mid) < pinMV {
			lo = mid
		} else {
			hi = mid
		}
	}
	return uint16(math.Round((lo + hi) / 2))
}

// idleHigh carries the board profile's idle-high pins across the layer
// boundary: firmware/emulated cannot import firmware/board, so the two describe
// a pin with the same shape and this converts between them.
func idleHigh(pins []hw.GPIOPin) []emulated.GPIOPin {
	if len(pins) == 0 {
		return nil
	}
	out := make([]emulated.GPIOPin, 0, len(pins))
	for _, p := range pins {
		out = append(out, emulated.GPIOPin{Port: p.Port, Pin: p.Pin})
	}
	return out
}

// listenInputs opens the channel a board's presses travel down, in whichever
// form the machine underneath can reach.
func listenInputs(board hw.Board, path string) (*peripheral.ButtonSender, error) {
	if board.Renode != nil {
		return peripheral.ListenButtonsTCP()
	}
	return peripheral.ListenButtons(path)
}
