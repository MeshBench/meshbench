// Wiring a board's declared parts into whichever machine is running it.
//
// One function for both emulators, deliberately. What a board carries is a fact
// about the board, and the interface draws it from the same declaration, so a
// part that reached the guest under QEMU and quietly did not under Renode is a
// window telling the truth about one machine and not the other. It was, until
// the Renode path was made to come through here.
package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/emulated/peripheral"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// withParts hangs everything the board declares off the node, and hands it back.
func withParts(node *emulated.EmulatedNode, board hw.Board, spec scenario.Node,
	set firmware.BuildSettings) (*emulated.EmulatedNode, error) {

	// The board's buttons, from the same declaration everything else comes
	// from. Only the ones it actually has: a board that declares none, or
	// declares one as absent, gets no channel rather than a channel nothing
	// can move.
	if p := board.Hardware; p != nil {
		var pins []int
		for _, part := range p.PartsOfKind(hw.Button) {
			if part.Pin != hw.PinNone {
				pins = append(pins, part.Pin)
			}
		}
		// A trackball's directions are buttons as far as the machine is
		// concerned - four lines the guest reads, moved from outside. It is
		// the firmware that decides an edge on one of them means a step.
		for _, part := range p.PartsOfKind(hw.Ball) {
			for _, pin := range part.Pins {
				if pin != hw.PinNone {
					pins = append(pins, pin)
				}
			}
		}
		// A keyboard, a touch panel and the cell's own divider travel the same
		// channel, so the channel exists if the board has any of them.
		var kbd, touch uint8
		for _, part := range p.PartsOfKind(hw.Keys) {
			kbd = part.Addr
		}
		for _, part := range p.PartsOfKind(hw.Touch) {
			touch = part.Addr
		}
		meter, hasMeter := batteryMeter(board)
		if len(pins) > 0 || kbd != 0 || touch != 0 || hasMeter {
			// Renode cannot dial a socket file, so its boards get the same
			// channel on a loopback port. Which one a board uses is decided
			// here and nowhere else: everything above and below this line is
			// the same for both machines.
			bs, err := listenInputs(board, filepath.Join(node.Dir, "buttons.sock"))
			if err != nil {
				return nil, fmt.Errorf("engine: listening for %s's inputs: %w", spec.Name, err)
			}
			node.Buttons = bs
			node.ButtonPath = bs.Path()
			node.ButtonPort = bs.Port()
			node.ButtonPins = pins
			node.KbdAddr, node.TouchAddr = kbd, touch
			if hasMeter {
				node.BatChannel, node.BatRaw, node.HasMeter = meter.channel, meter.raw, true
				// Held for the emulator to collect when it connects. QEMU is
				// also told at machine creation and hears it twice, which
				// costs nothing; Renode has no other way to be told at all.
				bs.Preset(meter.channel, meter.raw)
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
		for _, part := range p.PartsOfKind(hw.Card) {
			if part.Pin == hw.PinNone {
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
				card = filepath.Join(node.Dir, "card.img")
			}
			if err := os.MkdirAll(filepath.Dir(card), 0o755); err != nil {
				return nil, fmt.Errorf("engine: %s's card: %w", spec.Name, err)
			}
			if err := emulated.MakeCard(card); err != nil {
				return nil, fmt.Errorf("engine: %s's card: %w", spec.Name, err)
			}
			node.CardPath, node.CardCS = card, part.Pin
			break
		}
	}

	// The receiver, where the board has one. Fed from the node's own position
	// rather than from a log, so there is one place the node is and both the
	// channel and the handheld read it.
	if p := board.Hardware; p != nil && len(p.PartsOfKind(hw.GPS)) > 0 {
		g, err := peripheral.ListenGPS(filepath.Join(node.Dir, "gps.sock"),
			spec.Position.Lat, spec.Position.Lon, spec.HeightAGLm, gpsEpoch)
		if err != nil {
			return nil, fmt.Errorf("engine: listening for %s's receiver: %w", spec.Name, err)
		}
		node.GPS = g
		node.GPSPath = g.Path()
	}

	// The display, from the same declaration the machine's wiring comes from.
	// Only where something inside the machine will connect to the other end:
	// a listener nobody dials is a panel that waits for ever, and the window
	// above it would report a board that had drawn nothing as one that chose
	// not to.
	if p := board.Hardware; ScreenModelled(board) {
		ln, err := listenFrames(board, filepath.Join(node.Dir, "panel.sock"))
		if err != nil {
			return nil, fmt.Errorf("engine: listening for %s's display: %w", spec.Name, err)
		}
		node.Panel = ln
		node.PanelPath = ln.Path()
		node.PanelPort = ln.Port()
		node.PanelAddr = p.Screen.Addr
		// An SH1106 is an SSD1306 whose columns start two to the right. Not a
		// detail: the whole picture slides sideways without it, which reads as
		// a driver fault rather than a wrong constant.
		if p.Screen.Controller == "SH1106" {
			node.PanelOffset = 2
		}
		// A colour panel goes on the radio's controller instead, and needs
		// its own select and the command/data line to be told apart from it.
		if p.Screen.Bus == hw.BusSPI {
			node.PanelCS, node.PanelDC = p.Screen.CS, p.Screen.DC
			node.PanelWidth, node.PanelHgt = p.Screen.WidthPx, p.Screen.HeightPx
		}
	}
	return node, nil
}

// ScreenModelled reports whether the machine that would run this board has a
// display model to put behind its declared screen.
//
// Beside the wiring rather than in the interface, and for the reason the meter
// learned: the window's Observed column is the one a person trusts, and a copy
// of this rule kept somewhere else went wrong within a day. A board whose panel
// nothing draws must read as "no display modelled" and never as "silent", which
// says the firmware chose not to draw.
//
// QEMU has both buses. Renode has the SPI panels - an ST7789 and an ST7735,
// sharing the radio's controller and told apart by chip select, which is how
// they are wired in copper - and not the I2C one: its TWIM model answers an
// address with a NACK because until now no board under it had anything on that
// bus to answer.
func ScreenModelled(board hw.Board) bool {
	p := board.Hardware
	if p == nil || p.Screen == nil {
		return false
	}
	if board.Renode != nil {
		// The generated platform description connects the display's select and
		// command lines by naming one port, so a board whose panel sat on the
		// other port would be wired wrong rather than unsupported. Both boards
		// that carry one have it on the radio's port; this is the check that
		// says so rather than a comment claiming it.
		return p.Screen.Bus == hw.BusSPI &&
			p.Screen.CS != 0 && p.Screen.DC != 0 &&
			onSamePort(p.Screen.CS, board.Renode.NssPort) &&
			onSamePort(p.Screen.DC, board.Renode.NssPort)
	}
	return p.Screen.Bus == hw.BusI2C || p.Screen.Bus == hw.BusSPI
}

// onSamePort reports whether a flat pin is on the port Renode calls name.
// P0.x is x and P1.x is 32+x, the numbering every profile here uses.
func onSamePort(pin int, name string) bool {
	if name == "gpio1" {
		return pin >= 32
	}
	return pin < 32
}
