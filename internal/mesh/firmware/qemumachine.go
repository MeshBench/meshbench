// The machine this board is, said as one string.
//
// QEMU takes a machine and its properties as a single comma-separated
// argument, so everything a board declares - where its radio sits, what it has
// on its buses, how much RAM is fitted - is composed here rather than scattered
// through the launch. Only what the board actually declares is passed: a
// property with a zero value is a pin the machine would wire to GPIO 0, which
// is a strapping pin on this part and not a spare one.
package firmware

import (
	"fmt"
	"strconv"
	"strings"
)

// machineString is the -machine argument for this node, radioAt being where
// the radio device should look for the model.
func (e *EmulatedNode) machineString(radioAt string) string {
	machine := fmt.Sprintf("%s,radio-path=%s,radio-spi=%d,radio-nss=%d,radio-busy=%d",
		e.Machine, radioAt, e.SPI, e.NSS, e.Busy)
	// Only when the board records one. Without it the machine leaves the line
	// unwired, and the firmware never learns a packet arrived - it reads a
	// received packet solely from the interrupt this pin raises.
	if e.DIO1 != 0 {
		machine += fmt.Sprintf(",radio-dio1=%d", e.DIO1)
	}
	// Only when the board has one. Left off, the machine leaves the line
	// unwired, which is what a board with no module should look like - as
	// opposed to one whose module is permanently switched off.
	if e.FEM != 0 {
		machine += fmt.Sprintf(",radio-fem=%d", e.FEM)
	}
	if e.PSRAMOctal {
		machine += ",psram-octal=on"
	}
	// A firmware whose exception handler reaches for the floating point unit
	// before anything has enabled it dies in a loop nothing can be seen past.
	// Asked for by environment rather than by board, because it is a property
	// of the firmware being looked at rather than of the hardware.
	if CoprocAtReset() {
		machine += ",cp-at-reset=on"
	}
	if e.ButtonPath != "" {
		// The path on its own, because more than the buttons listen on it: a
		// board with a meter and no button still has something to say.
		machine += ",input-path=" + e.ButtonPath
		if e.KbdAddr != 0 || e.TouchAddr != 0 {
			machine += fmt.Sprintf(",kbd-addr=%d,touch-addr=%d", e.KbdAddr, e.TouchAddr)
		}
		if len(e.ButtonPins) > 0 {
			pins := make([]string, len(e.ButtonPins))
			for i, p := range e.ButtonPins {
				pins[i] = strconv.Itoa(p)
			}
			// Doubled, because this is one value inside a comma separated
			// list and a bare comma would end it: a board with two buttons
			// refused to start at all until it was.
			machine += ",input-pins=" + strings.Join(pins, ",,")
		}
	}
	if e.CardPath != "" {
		machine += fmt.Sprintf(",card-cs=%d", e.CardCS)
	}
	if e.BatRaw != 0 {
		machine += fmt.Sprintf(",bat-adc-channel=%d,bat-adc-raw=%d",
			e.BatChannel, e.BatRaw)
	}
	// The display, on the same terms as the radio: only when the board has
	// one and only when something is listening.
	if e.PanelPath != "" {
		addr := e.PanelAddr
		if addr == 0 {
			addr = 0x3C
		}
		machine += fmt.Sprintf(",panel-path=%s,panel-addr=%d,panel-offset=%d",
			e.PanelPath, addr, e.PanelOffset)
		if e.PanelCS != 0 {
			machine += fmt.Sprintf(",panel-cs=%d,panel-dc=%d,panel-w=%d,panel-h=%d",
				e.PanelCS, e.PanelDC, e.PanelWidth, e.PanelHgt)
		}
	}
	return machine
}
