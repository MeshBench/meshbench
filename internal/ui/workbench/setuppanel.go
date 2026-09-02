// Setup: what this machine has, what it has not, and what to do about each.
//
// The one page a new install needs and the one it did not have. Everything on
// it was already discoverable - the firmware library knew its cache was empty,
// the terrain switch knew nobody had answered it, and the emulator toolchain
// announced itself by a node failing to start - but each was discovered by
// walking into it, one at a time, in the middle of doing something else.
//
// So it is a report, not a wizard. Nothing here does anything on its own: each
// row says what it costs and offers the one action that would fix it, and the
// rows nothing can fix say the steps in words instead of pretending there is a
// button.
package workbench

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// setupRowW is one row's widget, pooled by key. Widget identity is address, so
// a button rebuilt each frame forgets it was pressed.
type setupRowW struct{ act comp.Button }

type setupPanel struct {
	recheckBtn comp.Button
	list       widget.List
	rows       map[string]*setupRowW

	// Refresh and OnAction are wired by the panel list; a panel owns no store.
	Refresh  func()
	OnAction func(verb string, params map[string]any)

	asked bool
	// lastUpdate is the update answer the page has already drawn, so one that
	// arrives while Setup is open lands on it. Both the check and the download
	// run on workers and finish long after the page was built; without this the
	// row went on saying "none has answered yet" underneath a status line that
	// had just named the release.
	lastUpdate string
}

func (p *setupPanel) rowFor(key string) *setupRowW {
	if p.rows == nil {
		p.rows = map[string]*setupRowW{}
	}
	w, ok := p.rows[key]
	if !ok {
		w = &setupRowW{}
		w.act.Kind = comp.Secondary
		p.rows[key] = w
	}
	return w
}

func (p *setupPanel) do(verb string, params map[string]any) {
	if p.OnAction != nil {
		p.OnAction(verb, params)
	}
}

func (p *setupPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.asked && p.Refresh != nil {
		p.asked = true
		p.Refresh()
	}
	if u := s.Update.Checked + s.Update.Staged; u != p.lastUpdate && p.Refresh != nil {
		p.lastUpdate = u
		p.Refresh()
	}
	p.recheck(gtx)

	groups := s.Setup
	p.list.Axis = layout.Vertical
	return layout.UniformInset(t.Sp.L).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.header(t, gtx, groups)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(groups) == 0 {
					return layout.Dimensions{}
				}
				return p.cards(t, gtx, groups)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(groups) == 0 {
					return p.empty(t, gtx)
				}
				return comp.List(t, &p.list, len(groups),
					func(gtx layout.Context, i int) layout.Dimensions {
						return p.group(t, gtx, groups[i])
					})(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"Sizes are stated before anything is fetched, and nothing on "+
					"this page downloads until it is pressed.")),
		)
	})
}

// recheck labels the one button the page itself owns and fires it. Called from
// both draws, because a control drawn by one and wired by the other is a
// control the audit correctly reports as dead.
func (p *setupPanel) recheck(gtx layout.Context) {
	p.recheckBtn.Kind, p.recheckBtn.Label = comp.Secondary, "Check again"
	if p.recheckBtn.Click.Clicked(gtx) && p.Refresh != nil {
		p.Refresh()
	}
}

// auditDraw is every row this panel can draw, in two columns and without the
// counters or the footnote.
//
// The real page scrolls and the audit's sweep cannot: a list taller than the
// window reads as controls nobody can click, and it would be the audit that was
// wrong. Columns are the cheap axis here, the same answer the Configuration
// page came to - the sweep costs nothing extra for a wider layout and a great
// deal for a taller one.
func (p *setupPanel) auditDraw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	p.recheck(gtx)
	groups := s.Setup
	half := (len(groups) + 1) / 2
	column := func(from, to int) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			items := make([]layout.FlexChild, 0, to-from)
			for i := from; i < to && i < len(groups); i++ {
				g := groups[i]
				items = append(items, layout.Rigid(
					func(gtx layout.Context) layout.Dimensions {
						return p.group(t, gtx, g)
					}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		}
	}
	return layout.UniformInset(t.Sp.L).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.header(t, gtx, groups)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, column(0, half)),
					layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
					layout.Flexed(1, column(half, len(groups))),
				)
			}),
		)
	})
}

// setupTally is what the cards count, taken once per frame.
type setupTally struct {
	ready, needed, undecided, missing, blocked int
}

func tallyOf(groups []state.SetupGroup) setupTally {
	var s setupTally
	for _, g := range groups {
		for _, r := range g.Rows {
			switch state.SetupState(r.State) {
			case state.SetupReady:
				s.ready++
			case state.SetupNeeded:
				s.needed++
			case state.SetupUndecided:
				s.undecided++
			case state.SetupBlocked:
				s.blocked++
			default:
				s.missing++
			}
		}
	}
	return s
}

// cards are the four answers somebody opens this page for: is anything
// stopping me, is anything waiting on me, what is here already, and what can
// this machine simply not have.
func (p *setupPanel) cards(t *theme.Theme, gtx layout.Context,
	groups []state.SetupGroup) layout.Dimensions {
	s := tallyOf(groups)
	cells := []layout.Widget{
		comp.StatCell(t, "Ready", itoa(s.ready), "here, and a node would find it"),
		comp.StatCell(t, "Blocking", itoa(s.needed),
			"something in this session is waiting on it"),
		comp.StatCell(t, "Waiting on you", itoa(s.undecided),
			"a question nothing has answered on your behalf"),
		comp.StatCell(t, "Not here", itoa(s.missing+s.blocked), notHereCaption(s)),
	}
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return comp.CellGrid(t, gtx, 150, cells)
	})(gtx)
}

// notHereCaption keeps "nobody has fetched it" apart from "this platform
// cannot have it". They add up to the same number and only one of them is
// something an operator can act on.
func notHereCaption(s setupTally) string {
	if s.blocked > 0 {
		return itoa(s.blocked) + " of them cannot be had on this platform at all"
	}
	return "absent, and nothing needs it yet"
}

func (p *setupPanel) header(t *theme.Theme, gtx layout.Context,
	groups []state.SetupGroup) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, "Setup")),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
					setupSubtitle(tallyOf(groups)))),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.recheckBtn.Layout(t, gtx)
		}),
	)
}

// setupSubtitle says the verdict in a sentence, because a row of counters is
// not an answer to "can I run this yet".
func setupSubtitle(s setupTally) string {
	switch {
	case s.needed > 0:
		return "something in this session cannot run until the rows below are seen to."
	case s.undecided > 0:
		return "nothing is broken; one thing is waiting to be told what it may do."
	default:
		return "everything this session needs is on this machine."
	}
}

func (p *setupPanel) empty(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Dim, "Not checked yet.")),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"Check again reads the caches and the tool lookup. It asks the "+
					"disk, never the network.")),
		)
	})
}
