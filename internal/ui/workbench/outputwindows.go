// One node's one log, in a window of its own.
//
// The Output tab shows one source at a time because a tab is one pane. What
// people actually do while a board is misbehaving is watch its screen and two
// of its logs together - what the board printed beside what the emulator said
// about running it - and that needs windows, not tabs.
//
// Keyed by node and source together, so a second request for the same log
// raises the window that is already out there rather than opening another copy
// of it.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

type outputWindows struct {
	*windowSet
}

func newOutputWindows() *outputWindows {
	return &outputWindows{windowSet: newWindowSet()}
}

// outputWindowKey identifies one log: a node and one of its voices.
func outputWindowKey(node, source string) string { return node + "\x00" + source }

func (w *outputWindows) openFor(node, source string,
	newTheme func() *theme.Theme, st *state.Store, do Do) {
	key := outputWindowKey(node, source)
	if !w.claim(key) {
		return
	}
	p := &outputWindowPanel{node: node, OnDo: do}
	p.out.source, p.out.noPop = source, true
	title := "MeshBench - " + node + " " + sourceLabel(source)
	go runPopout(w.windowSet, key, title, popoutSize{760, 520}, p, newTheme, st)
}

// outputWindowPanel is one log, drawn on its own.
//
// It reuses the Output tab's own pane rather than a second implementation of
// it: the source buttons, the search, the follow toggle and the footer all
// behave identically, because they are the same code.
type outputWindowPanel struct {
	node string
	OnDo Do

	out outputPane

	Layered   bool
	bar       comp.TitleBar
	maximised bool

	asked string
}

func (p *outputWindowPanel) setLayered(on bool)       { p.Layered = on }
func (p *outputWindowPanel) titleBar() *comp.TitleBar { return &p.bar }
func (p *outputWindowPanel) setMaximised(on bool)     { p.maximised = on }

func (p *outputWindowPanel) Draw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	o := &p.out
	if !o.built {
		o.build()
	}
	// Its own subscription, so a window left open on the emulator's log keeps
	// being refreshed while the tab that opened it is showing something else.
	askOutputOnce(p.OnDo, &p.asked, p.node, o.source)
	p.clicks(gtx)

	lines, total, note, path := o.readFrom(p.node, s)
	shown := filterLines(lines, fieldText(&o.search))

	var kids []layout.FlexChild
	if p.Layered {
		p.bar.Title, p.bar.Maximised = p.node+" "+sourceLabel(o.source), p.maximised
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.bar.Layout(t, gtx)
		}))
	}
	kids = append(kids,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, p.node)),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
							sourceWhat(o.source))),
					)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return o.head(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return o.body(t, gtx, shown, note)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return o.foot(t, gtx, len(shown), total, path)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

// clicks runs the pane's own controls inside this window.
func (p *outputWindowPanel) clicks(gtx layout.Context) {
	o := &p.out
	if o.pauseBtn.Click.Clicked(gtx) {
		o.follow = !o.follow
	}
	o.pauseBtn.Label = "pause"
	if !o.follow {
		o.pauseBtn.Label = "follow"
	}
	for i := range o.srcBtns {
		if !o.srcBtns[i].Click.Clicked(gtx) {
			continue
		}
		// Switching source in a popped-out window changes what this window
		// is, rather than opening another: somebody who wanted both open has
		// the button in the tab for that.
		o.source = outputSources[i].key
		o.follow = true
	}
}

// sourceLabel and sourceWhat are one log's short name and its one-line
// description, from the same list the buttons are built from.
func sourceLabel(source string) string {
	for _, s := range outputSources {
		if s.key == source {
			return s.label
		}
	}
	return source
}

func sourceWhat(source string) string {
	for _, s := range outputSources {
		if s.key == source {
			return s.what
		}
	}
	return ""
}
