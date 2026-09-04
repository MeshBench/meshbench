// The companion's settings, as a view inside Client.
//
// This is the pane the rewrite lost. Everything here was reachable only from
// the command line afterwards, and path hash size was not reachable at all -
// the protocol had SetPathHashMode and nothing sent it. It lives inside
// Client rather than as a mode of its own: CLI and TCP hand the radio to
// whatever is on the other end of them, and a settings form only one of the
// three ways in can use does not need a tab all three share the row with.
//
// Two rules the layout is built around. Every box shows what the node said,
// not what was typed into it last session, so a field is a proposal and the
// line beneath it is the truth. And nothing is sent until Apply: a modem
// setting that half applies takes the node off the mesh, and a box that acts
// on every keystroke half applies constantly.
package workbench

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// pathHashChoices is what the operator may pick, in bytes.
//
// One, two or three. The firmware answers ERR_CODE_ILLEGAL_ARG to anything
// above mode 2, so a fourth choice would be a control that always fails.
var pathHashChoices = []int{1, 2, 3}

// radioPane is the settings form.
func (c *companionTab) radioPane(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	if !cs.Connected {
		return centreNote(t, gtx,
			"not connected - settings are read from the node, and there is\n"+
				"nothing to read them from. Connect first.")
	}
	rows := []layout.Widget{
		settingHeading(t, "Identity"),
		c.setting(t, "Name", &c.setName, cs.Name, "what this node advertises"),
		settingHeading(t, "Modem"),
		c.setting(t, "Frequency", &c.setFreq, freqShown(cs), "kHz"),
		c.setting(t, "Bandwidth", &c.setBW, kHzShown(cs.BWKHz), "kHz"),
		c.setting(t, "Spreading factor", &c.setSF, u8Shown(cs.SF), "5 to 12"),
		c.setting(t, "Coding rate", &c.setCR, u8Shown(cs.CR), "4/5 to 4/8"),
		c.setting(t, "TX power", &c.setTx, u8Shown(cs.TxDBm), txNote(cs)),
		settingHeading(t, "Paths"),
		func(gtx layout.Context) layout.Dimensions {
			return c.pathHashRow(t, gtx, cs)
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, modemWarning))
		},
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, comp.List(t, &c.radioList, len(rows),
			func(gtx layout.Context, i int) layout.Dimensions { return rows[i](gtx) })),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(t.Sp.S).Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint,
							"empty boxes are left alone, and Apply clears them", false)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.applyRadio.Layout(t, gtx)
						}),
					)
				})
		}),
	)
}

// modemWarning is the one thing about this form that can go badly wrong.
const modemWarning = "The modem settings decide who this node can hear at all. " +
	"Change them here and it stops matching its neighbours, which looks " +
	"exactly like a node that has gone quiet."

// setting is one labelled box with what the node currently holds beneath it.
func (c *companionTab) setting(t *theme.Theme, label string, f *comp.Field, now, note string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS, Bottom: t.Sp.XXS}.
			Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					comp.Fixed(gtx, 132, comp.Text(t, t.Sz.Caption, t.P.Dim, label)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(140)
						gtx.Constraints.Max.X = gtx.Dp(140)
						return f.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint,
						settingNote(now, note), false)),
				)
			})
	}
}

// settingNote is what sits beside the box: what the node holds, and what the
// units are.
func settingNote(now, note string) string {
	switch {
	case now == "" && note == "":
		return ""
	case now == "":
		return note
	case note == "":
		return "now " + now
	}
	return "now " + now + "  ·  " + note
}

func settingHeading(t *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Top: t.Sp.S, Bottom: t.Sp.XXS}.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Faint, strings.ToUpper(s)))
	}
}

// pathHashRow is the size selector, with what it costs beside it.
func (c *companionTab) pathHashRow(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				comp.Fixed(gtx, 132, comp.Text(t, t.Sz.Caption, t.P.Dim, "Hash size")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.hashPicker(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint,
					pathHashNote(cs), false)),
			)
		})
}

// hashPicker is the 1 / 2 / 3 segmented control, drawn wherever it is needed -
// the settings form and the composer both use it, and two controls for one
// preference that disagreed on screen would be worse than none.
func (c *companionTab) hashPicker(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	var kids []layout.FlexChild
	for _, n := range pathHashChoices {
		n := n
		ck := c.click(fmt.Sprintf("hash:%d", n))
		// Always the operator's own choice, defaulted to three: a control
		// that only sometimes shows a selection is the ambiguity, not the
		// fix for one.
		on := c.pathHash == n
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.XXS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return modeChip(t, gtx, strconv.Itoa(n), on)
					})
				})
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}

// pathHashNote says what the size costs and what it buys.
//
// Worth saying in the interface rather than the documentation: at three bytes
// the loop-detect thresholds the firmware indexes by size are all 1, so
// minimal, moderate and strict stop being different settings. Somebody
// tuning one while the other is at 3 is tuning nothing.
func pathHashNote(cs state.Companion) string {
	n := cs.PathHashBytes
	if n == 0 {
		return "the node has not said - firmware older than v10 does not report it"
	}
	per := fmt.Sprintf("%d byte per hop", n)
	if n > 1 {
		per = fmt.Sprintf("%d bytes per hop", n)
	}
	switch n {
	case 1:
		return per + "  ·  smallest packets, most false collisions between paths"
	case 3:
		return per + "  ·  loop detection is the same at minimal, moderate and strict"
	}
	return per
}

// freqShown is the frequency in the units the box takes, which is kHz - the
// same units the command wants, so nothing is converted between reading it
// and sending it back.
func freqShown(cs state.Companion) string {
	if cs.FreqKHz == 0 {
		return ""
	}
	return fmt.Sprintf("%d (%.3f MHz)", cs.FreqKHz, float64(cs.FreqKHz)/1000)
}

func kHzShown(v uint32) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(v), 10)
}

func u8Shown(v uint8) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(int(v))
}

// txNote carries the ceiling the radio reported, so a number that would be
// refused can be seen to be too big before it is sent.
func txNote(cs state.Companion) string {
	if cs.MaxTxDBm == 0 {
		return "dBm"
	}
	return fmt.Sprintf("dBm, up to %d", cs.MaxTxDBm)
}

// radioClicks handles Apply and Revert.
func (c *companionTab) radioClicks(gtx layout.Context, cs state.Companion) {
	for _, n := range pathHashChoices {
		if c.click(fmt.Sprintf("hash:%d", n)).Clicked(gtx) {
			c.pathHash = n
		}
	}
	if !c.applyRadio.Click.Clicked(gtx) && !c.radioSubmitted {
		return
	}
	params := map[string]any{}
	if v := fieldText(&c.setName); v != "" {
		params["name"] = v
	}
	for _, f := range []struct {
		key   string
		field *comp.Field
	}{
		{"freq_khz", &c.setFreq}, {"bw_khz", &c.setBW},
		{"sf", &c.setSF}, {"cr", &c.setCR}, {"tx_dbm", &c.setTx},
	} {
		// Anything unparseable is left out rather than sent as zero: a zero
		// in a modem field is not "unchanged", it is a value the firmware
		// rejects, and it fails the whole frame including the fields that
		// were fine.
		if v, err := strconv.ParseFloat(fieldText(f.field), 64); err == nil {
			params[f.key] = v
		}
	}
	// Only when it differs from what the node holds. Sending the node's own
	// value back is a write to flash for no change.
	if c.pathHash != cs.PathHashBytes {
		params["path_hash"] = float64(c.pathHash)
	}
	if len(params) == 0 {
		return
	}
	c.do("companion.configure", params)
	for _, f := range c.radioFields() {
		f.Editor.SetText("")
	}
}

func (c *companionTab) radioFields() []*comp.Field {
	return []*comp.Field{
		&c.setName, &c.setFreq, &c.setBW, &c.setSF, &c.setCR, &c.setTx,
	}
}

// buildRadio sets up the settings controls.
func (c *companionTab) buildRadio() {
	for _, f := range c.radioFields() {
		f.Editor.SingleLine, f.Editor.Submit = true, true
	}
	c.setName.Hint = "leave to keep"
	c.setFreq.Hint = "869525"
	c.setBW.Hint = "250"
	c.setSF.Hint = "11"
	c.setCR.Hint = "5"
	c.setTx.Hint = "22"
	c.applyRadio.Label, c.applyRadio.Kind = "Apply", comp.Primary
	c.radioList.Axis = layout.Vertical
}

// radioWidgets is every control here, for the audit.
func (c *companionTab) radioWidgets() ([]*comp.Field, []*comp.Button) {
	return c.radioFields(), []*comp.Button{&c.applyRadio}
}
