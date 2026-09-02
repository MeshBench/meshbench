package workbench

import (
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// setupStateInk maps a row's state to the word and the colour that say it.
//
// Only "needed" is warned about. A dependency nobody has asked for yet is not a
// fault, and a page of red rows on a fresh install teaches an operator that red
// means nothing. "Waiting on you" takes the accent instead, because it is the
// one state that is a question rather than a condition.
func setupStateInk(t *theme.Theme, s string) (string, colorNRGBA) {
	switch state.SetupState(s) {
	case state.SetupReady:
		return "ready", t.P.Good
	case state.SetupNeeded:
		return "needed", t.P.Warn
	case state.SetupUndecided:
		return "waiting on you", t.P.Accent
	case state.SetupBlocked:
		return "not on this platform", t.P.Dim
	case state.SetupMissing:
		return "not here", t.P.Dim
	default:
		return s, t.P.Dim
	}
}

// setupActionLabel is what the one button on a row offers to do. Named from the
// verb rather than carried in the row, so the wording is the interface's
// business and the check stays something a script reads.
func setupActionLabel(verb string) string {
	switch verb {
	case "resource.fetch":
		return "Fetch"
	case "terrain.allow", "update.allow":
		return "Allow"
	case "panel.open":
		return "Open Firmware"
	case "update.check":
		return "Check now"
	case "update.download":
		return "Download"
	case "update.reveal":
		return "Show me"
	default:
		return "Do it"
	}
}

// setupTurnOff is the one action whose sense flips with the row's state: a
// permission that is already granted is withdrawn by the same verb.
func setupTurnOff(r state.SetupRow) bool {
	if r.Verb != "terrain.allow" && r.Verb != "update.allow" {
		return false
	}
	on, _ := r.Params["on"].(bool)
	return !on
}

// group is one card: what the group is, the note true of all of it, then its
// rows.
func (p *setupPanel) group(t *theme.Theme, gtx layout.Context,
	g state.SetupGroup) layout.Dimensions {
	return layout.Inset{Bottom: t.Sp.M}.Layout(gtx, comp.Card(t, g.Name,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if g.Note == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint, g.Note))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rows0(t, gtx, g)
				}),
			)
		}))
}

func (p *setupPanel) rows0(t *theme.Theme, gtx layout.Context,
	g state.SetupGroup) layout.Dimensions {
	items := make([]layout.FlexChild, 0, len(g.Rows))
	for i := range g.Rows {
		r := g.Rows[i]
		items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.row(t, gtx, g.Name+"/"+r.Name, r)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}

// row is one dependency: what it is, what it costs, where it is, and the one
// thing that would change it.
func (p *setupPanel) row(t *theme.Theme, gtx layout.Context,
	key string, r state.SetupRow) layout.Dimensions {
	// Pooled only for a row that has an action. A widget built for a row that
	// draws no button is a control the audit finds, presses, and correctly
	// reports as reaching nothing.
	if r.Verb != "" {
		if w := p.rowFor(key); w.act.Click.Clicked(gtx) {
			p.do(r.Verb, r.Params)
		}
	}
	return layout.Inset{Bottom: t.Sp.M}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return setupRowHead(t, gtx, r)
				}),
				layout.Rigid(setupLine(t, t.Sz.Caption, t.P.Dim, r.What)),
				// Where the thing actually is, in mono: it is a path, and a
				// path is read character by character or not at all.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if r.Where == "" {
						return layout.Dimensions{}
					}
					return comp.Mono(t, t.Sz.Caption, t.P.Faint, r.Where)(gtx)
				}),
				// Do is content, not a tooltip. It is the whole answer for a
				// row nothing here can fix, and those are the rows people were
				// reading three documents to solve.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if r.Do == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Ink, r.Do))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rowAction(t, gtx, key, r)
				}),
			)
		})
}

// setupRowHead is the name, then the two things read across from it: what it
// costs, and what state it is in.
func setupRowHead(t *theme.Theme, gtx layout.Context, r state.SetupRow) layout.Dimensions {
	label, ink := setupStateInk(t, r.State)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, r.Name)),
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.Cost == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Right: t.Sp.M}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Dim, r.Cost))
		}),
		layout.Rigid(comp.Pill(t, ink, label)),
	)
}

func (p *setupPanel) rowAction(t *theme.Theme, gtx layout.Context,
	key string, r state.SetupRow) layout.Dimensions {
	if r.Verb == "" {
		return layout.Dimensions{}
	}
	w := p.rowFor(key)
	w.act.Label = setupActionLabel(r.Verb)
	if setupTurnOff(r) {
		w.act.Label = "Turn off"
	}
	// Held to its own width by a spacer taking the rest. A button laid out as
	// a rigid child of a vertical flex takes the whole card, and a full-width
	// Allow reads as a banner rather than as a control.
	return layout.Inset{Top: t.Sp.S}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return w.act.Layout(t, gtx)
				}),
				layout.Flexed(1, comp.Spacer),
			)
		})
}

// setupLine draws a caption only when there is one, so a row with nothing to
// add does not leave a blank line where a sentence was.
func setupLine(t *theme.Theme, size unit.Sp, c colorNRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if s == "" {
			return layout.Dimensions{}
		}
		return comp.Text(t, size, c, s)(gtx)
	}
}
