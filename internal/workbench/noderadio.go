// The Radio tab: what this node's chip has actually been set to.
//
// Everywhere else in the interface reports what a node is *meant* to be. The
// board profile says 22 dBm and a noise figure of 6; the scenario says SF8 on
// 869.618 MHz. None of that is a claim about the firmware running on the node,
// and MeshCore 1.17.1 fixed two faults where the two came apart: receive gain
// reverting to a compiled default after an AGC reset, and a transmit-enable
// line that never went high. Neither logged anything, and the firmware's own
// CLI went on reporting the setting the operator chose.
//
// So this reads the chip. Every value here came off the SPI the driver talks
// to, over the bridge, and is shown as the chip holds it - a register as a
// register - because the question is "is this node set to what I think it is",
// and a value translated on the way cannot answer it.
package workbench

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// radioRow is one line of the tab: what it is called, what it says, and the
// note that makes the value mean something.
type radioRow struct {
	label, value, why string
	// warn marks a value worth looking at twice rather than one that is wrong;
	// this panel reports, it does not diagnose.
	warn bool
}

func (p *nodeWindowPanel) radio(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	var st state.NodeStat
	for i := range s.Stats {
		if s.Stats[i].Name == p.node {
			st = s.Stats[i]
		}
	}
	r := st.Radio

	if !st.Running {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"this node is not running, so its radio has nothing to report"))
	}
	if !r.Reported {
		// Said, not drawn as zeroes. A chip reporting nothing and a chip set to
		// nothing look identical in a table of numbers.
		return layout.Center.Layout(gtx, comp.OneLine(t, t.Sz.Body, t.P.Dim,
			"this node's build does not report its radio yet - rebuild it from "+
				"meshcore-native and start it again", false))
	}

	rows := []radioRow{
		{label: "mode", value: radioMode(r.Mode),
			why: "what the chip is doing right now"},
		{label: "frequency", value: fmt.Sprintf("%.3f MHz", float64(r.FreqHz)/1e6),
			why: "as SetRfFrequency left it"},
		{label: "bandwidth", value: fmt.Sprintf("%.2f kHz", float64(r.BandwidthHz)/1000)},
		{label: "spreading factor", value: fmt.Sprintf("SF%d", r.SF)},
		{label: "coding rate", value: fmt.Sprintf("4/%d", r.CR)},
		{label: "preamble", value: fmt.Sprintf("%d symbols", r.PreambleSyms)},
	}

	gain := radioRow{label: "receive gain",
		value: fmt.Sprintf("%#02x  power saving", r.GainReg),
		why: "register 0x08AC. MeshCore re-applies the compile-time " +
			"SX126X_RX_BOOSTED_GAIN on every AGC reset, so a runtime change to " +
			"this is undone without anything saying so"}
	if r.Boosted {
		gain.value = fmt.Sprintf("%#02x  boosted", r.GainReg)
	} else {
		// Not an error - most boards ship this way - but it is the value the
		// 1.17.1 fault leaves behind, and worth a second look.
		gain.warn = true
	}
	rows = append(rows, gain)

	tx := radioRow{label: "transmit power",
		value: fmt.Sprintf("%d dBm", r.TxPowerDBm),
		why:   "what SetTxParams asked the PA for, not what leaves the antenna"}
	if r.TxPowerDBm == -128 {
		tx.value, tx.warn = "not set", true
		tx.why = "the firmware has not called SetTxParams; the board's own figure is standing in"
	}
	rows = append(rows, tx)

	fem := radioRow{label: "front-end module"}
	switch r.FemAtTx {
	case 0:
		fem.value = "not answered"
		fem.why = "this node has not transmitted, or has no module wired"
	case 1:
		fem.value = "out of circuit at transmit"
		fem.warn = true
		fem.why = "the enable line was low when this node last transmitted - on a " +
			"board with a module that is the whole of its gain, missing"
	default:
		fem.value = "in circuit at transmit"
		fem.why = "the enable line was high when this node last transmitted"
	}
	if r.FemLive {
		fem.value += "  (line high now)"
	}
	rows = append(rows, fem,
		radioRow{label: "IRQ mask", value: fmt.Sprintf("%#04x", r.IRQMask),
			why: "what the firmware allowed to raise DIO1"},
		radioRow{label: "IRQ flags", value: fmt.Sprintf("%#04x", r.IRQFlags),
			why: "what is raised now; a flag that sticks is a node that stops transmitting"},
		radioRow{label: "IRQ reads", value: fmt.Sprintf("%d, %d found the air busy",
			st.IRQReads, st.BusyReads),
			why: "how often the driver asked, and how often it was told to wait"},
		radioRow{label: "busy time", value: fmt.Sprintf("%d ms", st.BusyMs)},
	)

	// Vertical, said out loud. A list left on the zero value lays its rows out
	// across the window, which put the whole tab on one line.
	p.radioScroll.Axis = layout.Vertical

	lines := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Caption, t.P.Dim,
					"read off the chip over the bridge, not from the board profile", false))
		},
	}
	for _, row := range rows {
		row := row
		lines = append(lines, func(gtx layout.Context) layout.Dimensions {
			return p.radioLine(t, gtx, row)
		})
	}
	return comp.List(t, &p.radioScroll, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
		return lines[i](gtx)
	})(gtx)
}

func (p *nodeWindowPanel) radioLine(t *theme.Theme, gtx layout.Context, r radioRow) layout.Dimensions {
	value := t.P.Ink
	if r.warn {
		value = t.P.Warn
	}
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						w := gtx.Dp(unit.Dp(150))
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
						d := comp.OneLine(t, t.Sz.Body, t.P.Dim, r.label, false)(gtx)
						d.Size.X = w
						return d
					}),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Data, value, r.value, true)),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.why == "" {
					return layout.Dimensions{}
				}
				// The reason is content, not decoration: it is why the number
				// above is worth having on the screen.
				return layout.Inset{Left: unit.Dp(150), Top: t.Sp.XS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Faint, r.why))
			}),
		)
	})
}

// radioMode names what the chip is doing. The numbers are the chip's own.
func radioMode(m uint8) string {
	switch m {
	case 1:
		return "1  receiving"
	case 2:
		return "2  transmitting"
	case 3:
		return "3  channel activity detection"
	}
	return "0  standby"
}
