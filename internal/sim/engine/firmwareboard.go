package engine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
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
func emulatedBackend(spec scenario.Node, allowUnverified bool) (*firmware.EmulatedNode, error) {
	board, err := scenario.BoardByName(spec.Firmware.Board)
	if err != nil {
		return nil, err
	}
	if !allowUnverified && !scenario.EmulationSupported(board.Name) {
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
	img := firmware.BoardImage{
		Board:   board.Name,
		Role:    string(spec.Firmware.Role),
		Version: spec.Firmware.Version,
		Format:  format,
	}
	// A companion publishes a BLE build and a USB one. Only the USB build is
	// reachable here: its client arrives over the serial port, which an
	// emulator has, and Bluetooth is not something we model.
	if img.Role == "companion_radio" {
		img.Transport = "usb"
	}
	src := firmware.BoardImagePath(cache, img)
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

	if board.Renode != nil {
		return &firmware.EmulatedNode{
			Emulator: firmware.Renode,
			Image:    src,
			// Published nRF52 images are linked above a Nordic SoftDevice,
			// which is fetched rather than bundled and so may not be here yet.
			// The refusal names it, because the alternative is a node that
			// boots into a fill pattern and looks like a broken emulator.
			SoftDeviceDir: firmware.SoftDeviceDir(),
			Platform:      board.Renode.Platform,
			SPIBase:       board.Renode.SPIBase,
			NssPort:       board.Renode.NssPort,
			NssPin:        board.Renode.NssPin,
			IrqPort:       board.Renode.IrqPort,
			IrqPin:        board.Renode.IrqPin,
			NodeName:      spec.Name,
			Dir:           dir,
		}, nil
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
	// the coprocessors up at reset and another that would be flattered by it,
	// and one that drives a different SPI controller entirely.
	set := firmware.LoadBuildSettings(src)

	padded := filepath.Join(dir, "flash.bin")
	if _, err := firmware.PadImageKeeping(src, padded); err != nil {
		return nil, err
	}

	node := &firmware.EmulatedNode{
		Emulator:   firmware.QEMU,
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
		Dir:        dir,
		// Where this board's firmware puts Serial. On a board built with
		// ARDUINO_USB_CDC_ON_BOOT that is the USB peripheral, not UART0, and
		// a console handed to the wrong one is a board that boots and then
		// appears to say nothing.
		ConsoleOnUSB: board.QEMU.ConsoleOnUSB,
		// And what has been decided about this particular build.
		CoprocAtReset: set.CoprocAtReset,
	}
	// Which controller this firmware drives, where it is not the one the
	// board's own image uses. The peripherals move with it: they are one bus
	// in copper, and it is the firmware that decides which controller the
	// matrix puts on those pins.
	if set.SPIController != 0 {
		node.SPI = set.SPIController
	}

	// The board's buttons, from the same declaration everything else comes
	// from. Only the ones it actually has: a board that declares none, or
	// declares one as absent, gets no channel rather than a channel nothing
	// can move.
	if p := board.Hardware; p != nil {
		var pins []int
		for _, part := range p.PartsOfKind(scenario.Button) {
			if part.Pin != scenario.PinNone {
				pins = append(pins, part.Pin)
			}
		}
		// A trackball's directions are buttons as far as the machine is
		// concerned - four lines the guest reads, moved from outside. It is
		// the firmware that decides an edge on one of them means a step.
		for _, part := range p.PartsOfKind(scenario.Ball) {
			for _, pin := range part.Pins {
				if pin != scenario.PinNone {
					pins = append(pins, pin)
				}
			}
		}
		// A keyboard, a touch panel and the cell's own divider travel the same
		// channel, so the channel exists if the board has any of them.
		var kbd, touch uint8
		for _, part := range p.PartsOfKind(scenario.Keys) {
			kbd = part.Addr
		}
		for _, part := range p.PartsOfKind(scenario.Touch) {
			touch = part.Addr
		}
		meter, hasMeter := batteryMeter(board)
		if len(pins) > 0 || kbd != 0 || touch != 0 || hasMeter {
			bs, err := firmware.ListenButtons(filepath.Join(dir, "buttons.sock"))
			if err != nil {
				return nil, fmt.Errorf("engine: listening for %s's inputs: %w", spec.Name, err)
			}
			node.Buttons = bs
			node.ButtonPath = bs.Path()
			node.ButtonPins = pins
			node.KbdAddr, node.TouchAddr = kbd, touch
			if hasMeter {
				node.BatChannel, node.BatRaw = meter.channel, meter.raw
			}
		}
	}

	// The card slot, where the board has one and the node has a card in it.
	//
	// A slot is not a fitted card: the board can only say the slot exists, and
	// whether this particular node has storage is the scenario's business -
	// except where the firmware insists, which it can, because a build that
	// keeps its settings on a card boots into nothing without one.
	if p := board.Hardware; p != nil {
		for _, part := range p.PartsOfKind(scenario.Card) {
			if part.Pin == scenario.PinNone {
				continue
			}
			if !spec.HasCard(true, set.CardRequired) {
				break
			}
			// The node's own, beside its sockets and its logs, unless it was
			// handed one somewhere else - which is how a card is shared
			// between nodes or prepared in advance.
			card := spec.CardFile
			if card == "" {
				card = filepath.Join(dir, "card.img")
			}
			if err := os.MkdirAll(filepath.Dir(card), 0o755); err != nil {
				return nil, fmt.Errorf("engine: %s's card: %w", spec.Name, err)
			}
			if err := firmware.MakeCard(card); err != nil {
				return nil, fmt.Errorf("engine: %s's card: %w", spec.Name, err)
			}
			node.CardPath, node.CardCS = card, part.Pin
			break
		}
	}

	// The receiver, where the board has one. Fed from the node's own position
	// rather than from a log, so there is one place the node is and both the
	// channel and the handheld read it.
	if p := board.Hardware; p != nil && len(p.PartsOfKind(scenario.GPS)) > 0 {
		g, err := firmware.ListenGPS(filepath.Join(dir, "gps.sock"),
			spec.Position.Lat, spec.Position.Lon, spec.HeightAGLm, gpsEpoch)
		if err != nil {
			return nil, fmt.Errorf("engine: listening for %s's receiver: %w", spec.Name, err)
		}
		node.GPS = g
		node.GPSPath = g.Path()
	}

	// The display, from the same declaration the machine's wiring comes from.
	// Only where the board says it has one and only where the controller is
	// one we model: a board whose screen we cannot draw shows nothing, which
	// is what it does today and is honest about it.
	if p := board.Hardware; p != nil && p.Screen != nil &&
		(p.Screen.Bus == scenario.BusI2C || p.Screen.Bus == scenario.BusSPI) {
		ln, err := firmware.ListenPanel(filepath.Join(dir, "panel.sock"))
		if err != nil {
			return nil, fmt.Errorf("engine: listening for %s's display: %w", spec.Name, err)
		}
		node.Panel = ln
		node.PanelPath = ln.Path()
		node.PanelAddr = p.Screen.Addr
		// An SH1106 is an SSD1306 whose columns start two to the right. Not a
		// detail: the whole picture slides sideways without it, which reads as
		// a driver fault rather than a wrong constant.
		if p.Screen.Controller == "SH1106" {
			node.PanelOffset = 2
		}
		// A colour panel goes on the radio's controller instead, and needs
		// its own select and the command/data line to be told apart from it.
		if p.Screen.Bus == scenario.BusSPI {
			node.PanelCS, node.PanelDC = p.Screen.CS, p.Screen.DC
			node.PanelWidth, node.PanelHgt = p.Screen.WidthPx, p.Screen.HeightPx
		}
	}
	return node, nil
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

// batteryMeter is what the board's cell puts on its converter, at a full
// charge.
//
// The number the firmware sees rather than a voltage, because the arithmetic
// between the two is the board's - its divider against the converter's range -
// and it is declared beside the pin for that reason. A board that declares no
// meter gets no reading, which leaves the channel at nothing: honest about a
// cell nobody has measured, and still enough that the converter answers rather
// than leaving a firmware waiting for it.
func batteryMeter(board scenario.Board) (meterReading, bool) {
	p := board.Hardware
	if p == nil {
		return meterReading{}, false
	}
	// The pin to channel mapping below is the ESP32-S3's. Every board that
	// declares a meter is one today, and a board that is not would be given
	// somebody else's channel in silence - so it is refused rather than
	// guessed at, and whoever adds the first non-S3 meter finds out here.
	if !strings.EqualFold(board.MCU, "ESP32-S3") {
		return meterReading{}, false
	}
	for _, part := range p.PartsOfKind(scenario.Meter) {
		if part.Pin == scenario.PinNone || part.FullScaleMV <= 0 {
			continue
		}
		ch, ok := adc1Channel(part.Pin)
		if !ok {
			continue
		}
		raw := board.Battery.VoltageAt(1) * 1000 * 4096 / float64(part.FullScaleMV)
		return meterReading{channel: ch, raw: uint16(math.Max(0, math.Min(4095, raw)))}, true
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
