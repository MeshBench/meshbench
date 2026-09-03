// Package qemu builds the QEMU machine for an emulated board: the one
// comma-separated -machine argument that says where the radio sits and what
// the board has on its buses, composed from the board's own wiring.
package qemu

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is a board's QEMU wiring: everything the -machine argument is built
// from. An EmulatedNode fills one from its own fields.
type Config struct {
	Machine                       string
	SPI, NSS, Busy, DIO1, FEM     int
	PSRAMOctal, CoprocAtReset     bool
	ButtonPath                    string
	ButtonPins                    []int
	KbdAddr, TouchAddr            uint8
	CardPath                      string
	CardCS                        int
	BatChannel                    int
	BatRaw                        uint16
	PanelPath                     string
	PanelAddr                     uint8
	PanelOffset, PanelCS, PanelDC int
	PanelWidth, PanelHgt          int
}

// EnvCoprocAtReset brings the emulated coprocessors up enabled, which the part
// does not do.
//
// For one firmware only, and it is a lie the machine tells: CPENABLE resets to
// zero on this architecture and a firmware decides which of its tasks may use
// the floating point unit. A firmware whose exception handler saves floating
// point state before anything has enabled it takes a CoprocessorDisabled trap
// inside an exception vector, which is fatal and loops for ever - and hides
// everything behind it, so the board looks like it simply stopped.
//
// Off by default, because a firmware that genuinely mismanages that register
// would be flattered by it. On, it is a way to see what happens next.
const EnvCoprocAtReset = "MESHBENCH_QEMU_COPROC_AT_RESET"

// CoprocAtReset reports whether that has been asked for.
func CoprocAtReset() bool {
	v := strings.TrimSpace(os.Getenv(EnvCoprocAtReset))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// Machine is the -machine argument for this board, bridge being the host:port
// where the engine is listening for this node.
//
// The chip itself is not named here. It is a library the emulator loads, found
// through MESHBENCH_RADIO_LIB, because which chip model a node runs is a
// property of the installation rather than of the board being emulated.
func (c Config) Arg(bridge string) string {
	machine := fmt.Sprintf("%s,radio-bridge=%s,radio-spi=%d,radio-nss=%d,radio-busy=%d",
		c.Machine, bridge, c.SPI, c.NSS, c.Busy)
	// Only when the board records one. Without it the machine leaves the line
	// unwired, and the firmware never learns a packet arrived - it reads a
	// received packet solely from the interrupt this pin raises.
	if c.DIO1 != 0 {
		machine += fmt.Sprintf(",radio-dio1=%d", c.DIO1)
	}
	// Only when the board has one. Left off, the machine leaves the line
	// unwired, which is what a board with no module should look like - as
	// opposed to one whose module is permanently switched off.
	if c.FEM != 0 {
		machine += fmt.Sprintf(",radio-fem=%d", c.FEM)
	}
	if c.PSRAMOctal {
		machine += ",psram-octal=on"
	}
	// A firmware whose exception handler reaches for the floating point unit
	// before anything has enabled it dies in a loop nothing can be seen past.
	// Asked for by the build rather than by the board, because it is a
	// property of the firmware being looked at rather than of the hardware -
	// the same board runs an image that needs it and one that does not. The
	// environment forces it on for everything, for a script that is looking
	// rather than configuring.
	if c.CoprocAtReset || CoprocAtReset() {
		machine += ",cp-at-reset=on"
	}
	if c.ButtonPath != "" {
		// The path on its own, because more than the buttons listen on it: a
		// board with a meter and no button still has something to say.
		machine += ",input-path=" + c.ButtonPath
		if c.KbdAddr != 0 || c.TouchAddr != 0 {
			machine += fmt.Sprintf(",kbd-addr=%d,touch-addr=%d", c.KbdAddr, c.TouchAddr)
		}
		if len(c.ButtonPins) > 0 {
			pins := make([]string, len(c.ButtonPins))
			for i, p := range c.ButtonPins {
				pins[i] = strconv.Itoa(p)
			}
			// Doubled, because this is one value inside a comma separated
			// list and a bare comma would end it: a board with two buttons
			// refused to start at all until it was.
			machine += ",input-pins=" + strings.Join(pins, ",,")
		}
	}
	if c.CardPath != "" {
		machine += fmt.Sprintf(",card-cs=%d", c.CardCS)
	}
	if c.BatRaw != 0 {
		machine += fmt.Sprintf(",bat-adc-channel=%d,bat-adc-raw=%d",
			c.BatChannel, c.BatRaw)
	}
	// The display, on the same terms as the radio: only when the board has
	// one and only when something is listening.
	if c.PanelPath != "" {
		addr := c.PanelAddr
		if addr == 0 {
			addr = 0x3C
		}
		machine += fmt.Sprintf(",panel-path=%s,panel-addr=%d,panel-offset=%d",
			c.PanelPath, addr, c.PanelOffset)
		if c.PanelCS != 0 {
			machine += fmt.Sprintf(",panel-cs=%d,panel-dc=%d,panel-w=%d,panel-h=%d",
				c.PanelCS, c.PanelDC, c.PanelWidth, c.PanelHgt)
		}
	}
	return machine
}
