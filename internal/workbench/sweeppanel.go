// The sweep: defining a matrix of arms, choosing who sends, and reading the
// result back.
//
// The biggest of the action panels by some way, because a sweep is the one
// thing here that takes a plan rather than a button - arms cross, seeds
// multiply, and the estimate has to say what that will cost before anybody
// starts it.
package workbench

import (
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
	"github.com/MeshBench/meshbench/internal/session"
)

// sweepControls defines an A/B experiment: arms, seeds, sender, timing.
type sweepControls struct {
	// addArm and sender are dropdowns rather than boxes because both are a
	// choice from a list this machine already knows: the firmware library, and
	// the companions in the scenario. Typing a version was the old shape and it
	// was wrong twice over - a misremembered tag defines an arm that cannot
	// start, and nothing in the panel said which tags existed.
	addArm comp.Dropdown
	sender comp.Dropdown

	// arms are the versions chosen, in the order chosen. An arm is one column
	// of the comparison, so this is what "define" turns into experiment.vary.
	// armRm is one remove button per arm, pooled by version: a widget rebuilt
	// each frame is a widget that never registers a press.
	armRm map[string]*comp.Button

	// The companions that will originate - sendersNow() below, off the live
	// snapshot, not cached here. A list, not one name: a single originator
	// makes every seed of an arm return the same numbers, so the seed cannot
	// bound the noise and no difference between arms has anything to be
	// called larger than.
	senderRm  map[string]*comp.Button
	allSend   comp.Button
	noneSend  comp.Button
	spreadFld comp.Field
	bytesFld  comp.Field

	// scopeFld is the region every sender originates under.
	//
	// A box rather than a dropdown, because the regions a scenario's repeaters
	// hold are theirs and not a list this panel knows: they are inferred from
	// captured traffic, and a menu of the ones we happen to have seen would be
	// wrong the first time somebody imports a mesh we have not.
	//
	// It has to be here at all because a sweep that sends unscoped is carried
	// by a different set of repeaters and measures a different network, with
	// nothing at either end reporting it.
	scopeFld comp.Field

	// vary crosses a parameter across the arms already defined. This is the
	// gesture the whole panel exists for: three path hash sizes by two
	// firmware versions is six arms, and without it the matrix can only ever
	// compare builds.
	varyDD   comp.Dropdown
	varyName string // the VaryParams name, or set:<setting>
	varyVals comp.Field
	addArms  comp.Button

	// Long lists scroll. Thirty-five companions pushed the panels below them
	// off the bottom of the window, and there was no way to reach the end of
	// the list or anything under it.
	armScroll, sendScroll, panelScroll widget.List

	seeds  comp.Field
	runFor comp.Field
	// snap is the latest snapshot, kept because a dropdown's chooser runs
	// after the frame that opened it.
	snap   *state.Snapshot
	define comp.Button
	start  comp.Button
	stop   comp.Button
	export comp.Button
	// choose opens the shell's chooser, the one way anything picks from a list.
	choose func(title string, opts []string, pick func(string))
	// askText is for a setting no list could hold: the firmware has more
	// switches than this build knows the names of, and the interesting one is
	// usually the one nobody enumerated.
	askText func(title, hint, initial string, got func(string))
	do      Do
	built   bool
}

func (c *sweepControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.seeds.Hint = "seeds, e.g. 1 2 3 4"
		c.runFor.Hint = "run for, seconds"
		for _, f := range []*comp.Field{&c.seeds, &c.runFor} {
			f.Editor.SingleLine = true
		}
		c.addArm.Label, c.addArm.Value = "arms: repeater firmware to compare", "add an arm..."
		c.sender.Label, c.sender.Value = "sender", "choose a companion..."
		c.armRm = map[string]*comp.Button{}
		c.senderRm = map[string]*comp.Button{}
		c.allSend.Label, c.allSend.Kind = "all", comp.Quiet
		c.noneSend.Label, c.noneSend.Kind = "none", comp.Quiet
		c.spreadFld.Hint = "fired N s apart (0 = together)"
		c.bytesFld.Hint = "message size, bytes (0 = a short label)"
		c.scopeFld.Hint = "scope, e.g. sco (blank = unscoped)"
		c.spreadFld.Editor.SingleLine = true
		c.bytesFld.Editor.SingleLine = true
		c.scopeFld.Editor.SingleLine = true
		c.varyDD.Label, c.varyDD.Value = "vary a parameter across arms", "choose a parameter..."
		c.varyVals.Hint = "values, comma separated"
		c.varyVals.Editor.SingleLine = true
		c.addArms.Label, c.addArms.Kind = "add arms", comp.Secondary
		c.armScroll.Axis, c.sendScroll.Axis = layout.Vertical, layout.Vertical
		c.panelScroll.Axis = layout.Vertical
		c.varyDD.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := make([]string, 0, len(session.VaryParams)+1)
			for _, p := range session.VaryParams {
				opts = append(opts, p.Label)
			}
			// Anything the enumerated list does not cover. The AGC reset
			// interval is the example that matters: it is what makes the
			// 1.17.1 gain fault reachable, and nothing here has a field for it.
			opts = append(opts, anySetting)
			c.choose("Vary which parameter?", opts, func(picked string) {
				if picked == anySetting {
					c.choose("Which firmware setting?",
						append([]string{customSetting}, commonSettings...),
						func(name string) {
							if name == customSetting {
								if c.askText == nil {
									return
								}
								c.askText("Firmware setting", "as the CLI spells it, e.g. rxdelay.base", "",
									func(typed string) {
										typed = strings.TrimSpace(strings.TrimPrefix(typed, "set "))
										if typed == "" {
											return
										}
										c.varyName, c.varyDD.Value = "set:"+typed, typed
										c.varyVals.Editor.SetText("")
									})
								return
							}
							c.varyName = "set:" + name
							c.varyDD.Value = name
							c.varyVals.Editor.SetText("")
						})
					return
				}
				for _, p := range session.VaryParams {
					if p.Label == picked {
						c.varyName, c.varyDD.Value = p.Name, p.Label
						c.varyVals.Editor.SetText(p.Defaults)
					}
				}
			})
		}
		c.define.Label, c.define.Kind = "define", comp.Secondary
		c.start.Label, c.start.Kind = "run it", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Destructive
		c.export.Label, c.export.Kind = "export", comp.Quiet
		c.addArm.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := repeaterBuilds(c.snap)
			if len(opts) == 0 {
				if c.do != nil {
					c.do("ui.said", "no repeater firmware in the library to compare; "+
						"download one from the Firmware panel first")
				}
				return
			}
			c.choose("Which firmware is this arm?", opts, func(picked string) {
				if c.do != nil {
					c.do("experiment.vary", map[string]any{
						"parameter": "repeater_version", "values": []any{picked}})
				}
			})
		}
		c.sender.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := companionsIn(c.snap)
			if len(opts) == 0 {
				if c.do != nil {
					c.do("ui.said", "no companion in this network to originate a message")
				}
				return
			}
			c.choose("Who sends?", opts, func(picked string) {
				now := c.sendersNow()
				for _, n := range now {
					if n == picked {
						return
					}
				}
				var next []any
				for _, n := range now {
					next = append(next, n)
				}
				next = append(next, picked)
				if c.do != nil {
					c.do("experiment.senders", map[string]any{"senders": next})
				}
			})
		}
		c.built = true
	}
	// The dropdowns are opened from a callback that runs outside this frame, so
	// the snapshot they read has to be kept rather than captured.
	c.snap = s
	if c.define.Click.Clicked(gtx) && c.do != nil {
		// Said aloud when nothing was defined.
		//
		// Every branch below is guarded on its box holding something, so
		// pressing this with the boxes empty did nothing at all and reported
		// nothing at all - which is indistinguishable from a button that is
		// not connected.
		asked := 0
		var ss []any
		for _, v := range splitFields(fieldText(&c.seeds)) {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				ss = append(ss, n)
			}
		}
		if len(ss) > 0 {
			asked++
			c.do("experiment.seeds", map[string]any{"seeds": ss})
		}

		if v, ok := num(&c.spreadFld); ok {
			asked++
			c.do("experiment.define", map[string]any{"spread_ms": v * 1000})
		}
		if v, ok := num(&c.bytesFld); ok && v > 0 {
			asked++
			c.do("experiment.define", map[string]any{"bytes": v})
		}
		// Sent even when blank, because clearing it is a real instruction:
		// "send unscoped" has to be reachable once a scope has been set.
		asked++
		c.do("experiment.define", map[string]any{
			"scope": strings.TrimSpace(fieldText(&c.scopeFld))})
		if v, ok := num(&c.runFor); ok {
			asked++
			c.do("experiment.base", map[string]any{"run_for_ms": v * 1000})
		}
		if asked == 0 {
			c.do("ui.said", "nothing to define: add an arm to compare, "+
				"the seeds to run each on, who sends, or how long a cell runs")
		}
	}
	if c.addArms.Click.Clicked(gtx) && c.do != nil {
		var vs []any
		for _, v := range strings.Split(fieldText(&c.varyVals), ",") {
			if v = strings.TrimSpace(v); v != "" {
				vs = append(vs, v)
			}
		}
		switch {
		case c.varyName == "":
			c.do("ui.said", "choose a parameter to vary first")
		case len(vs) == 0:
			c.do("ui.said", "no values to cross: type them comma separated")
		default:
			c.do("experiment.vary", map[string]any{
				"parameter": c.varyName, "values": vs})
		}
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.start", nil)
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.stop", nil)
	}
	if c.export.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.export", nil)
	}
	// The definition lives in the session, not here: the control socket defines
	// arms too, and a panel holding its own copy showed "no arms yet" over a
	// session with four. Removing one asks for the list without it.
	arms := c.armsNow()
	for i := range arms {
		b := c.armRm[arms[i]]
		if b == nil || !b.Click.Clicked(gtx) {
			continue
		}
		var keep []any
		for j, a := range arms {
			if j != i {
				keep = append(keep, a)
			}
		}
		if c.do != nil {
			c.do("experiment.define", map[string]any{"arms": armObjs(keep)})
		}
		break
	}

	sendersNow := c.sendersNow()
	for i := range sendersNow {
		b := c.senderRm[sendersNow[i]]
		if b == nil || !b.Click.Clicked(gtx) {
			continue
		}
		var keep []any
		for j, n := range sendersNow {
			if j != i {
				keep = append(keep, n)
			}
		}
		if c.do != nil {
			c.do("experiment.senders", map[string]any{"senders": keep})
		}
		break
	}
	if c.allSend.Click.Clicked(gtx) && c.do != nil {
		var all []any
		for _, n := range companionsIn(c.snap) {
			all = append(all, n)
		}
		c.do("experiment.senders", map[string]any{"senders": all})
	}
	if c.noneSend.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.senders", map[string]any{"senders": []any{}})
	}

	bar := actionBar{
		fields:  []*comp.Field{&c.seeds, &c.runFor, &c.spreadFld, &c.bytesFld, &c.scopeFld},
		buttons: []*comp.Button{&c.define, &c.start, &c.stop, &c.export},
		note: "a message is originated by a companion, so the sender has to be " +
			"one; two seeds that agree exactly are one draw repeated, not a spread",
	}
	// The whole panel scrolls.
	//
	// It is a column of a dozen controls in a fixed-width rail, and the ones
	// that matter most - define, run it, and the estimate saying what run it is
	// about to cost - are at the bottom. Without this they are simply not on
	// the screen, and nothing indicates that there is more.
	sections := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.addArm.Layout(t, gtx)
			})
		},
		func(gtx layout.Context) layout.Dimensions { return c.armList(t, gtx) },
		// Crossing, under the list it crosses onto.
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.varyDD.Layout(t, gtx)
			})
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return c.varyVals.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.addArms.Layout(t, gtx)
					}),
				)
			})
		},
		comp.Text(t, t.Sz.Caption, t.P.Faint,
			"crossed onto the arms above: three values by two arms is six"),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return c.sender.Layout(t, gtx) })
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.allSend.Layout(t, gtx)
						}),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.noneSend.Layout(t, gtx)
						}),
					)
				})
		},
		func(gtx layout.Context) layout.Dimensions { return c.senderList(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return bar.layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return c.estimate(t, gtx) },
	}
	sections = append(sections, func(gtx layout.Context) layout.Dimensions {
		return layout.Spacer{Height: t.Sp.M}.Layout(gtx)
	})
	return comp.List(t, &c.panelScroll, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
		return sections[i](gtx)
	})(gtx)
}
