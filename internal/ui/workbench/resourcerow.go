package workbench

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// resourceStateInk maps a row's state to the one colour that says it.
//
// Absent is Dim rather than Bad: a thing nobody has asked for yet is not a
// fault, and a red row for it teaches an operator to ignore red rows.
func resourceStateInk(t *theme.Theme, s string) (string, colorNRGBA) {
	switch s {
	case "present":
		return "on disk", t.P.Good
	case "fetching":
		return "fetching", t.P.Accent
	case "failed":
		return "failed", t.P.Bad
	default:
		return "not fetched", t.P.Dim
	}
}

// size says what it knows and marks what it does not.
//
// An estimate presented as a measurement is exactly what a page about disk
// usage must not do, so a guess carries a tilde and an absent thing is a dash
// rather than a zero.
func resourceSize(r state.ResourceRow) string {
	if r.Bytes <= 0 {
		return "-"
	}
	if r.Estimated {
		return "~" + siBytes(r.Bytes)
	}
	return siBytes(r.Bytes)
}

func (p *resourcesPanel) row(t *theme.Theme, gtx layout.Context,
	r state.ResourceRow) layout.Dimensions {
	key := resourceKey(r)
	w := p.rowFor(key)
	present := r.State == "present"

	if w.fetch.Click.Clicked(gtx) {
		p.do("resource.fetch", map[string]any{"name": r.Name, "version": r.Version})
	}
	if w.licence.Click.Clicked(gtx) {
		p.do("resource.licence", map[string]any{"name": r.Name, "version": r.Version})
	}
	if w.remove.Click.Clicked(gtx) {
		if p.confirm == key {
			p.confirm = ""
			p.do("resource.remove", map[string]any{"name": r.Name, "version": r.Version})
		} else {
			p.confirm = key
		}
	}
	label, ink := resourceStateInk(t, r.State)
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, comp.Text(t, t.Sz.Body, t.P.Ink,
							fmt.Sprintf("%s %s", r.Name, r.Version))),
						layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, resourceSize(r))),
						layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
						layout.Rigid(comp.Text(t, t.Sz.Caption, ink, label)),
					)
				}),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, r.Kind)),
				// Why is content, not a tooltip: it is the row's answer to
				// "and why is it in that state", and a state without one is
				// how an operator concludes the application is stuck.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if r.Why == "" {
						return layout.Dimensions{}
					}
					return comp.Text(t, t.Sz.Caption, t.P.Faint, r.Why)(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rowActions(t, gtx, w, key, present, r.Auto)
				}),
			)
		})
}

func (p *resourcesPanel) rowActions(t *theme.Theme, gtx layout.Context,
	w *resRowW, key string, present, auto bool) layout.Dimensions {
	// Labels and disabled states are re-stated each frame; the widgets they
	// belong to are not, so a press survives the redraw that follows it.
	w.fetch.Label = "Fetch"
	if present {
		w.fetch.Label = "Re-fetch"
	}
	w.remove.Label = "Remove"
	if p.confirm == key {
		w.remove.Label = "Remove - sure?"
	}
	w.remove.Disabled = !present
	w.remove.Reason = ""
	if !present {
		w.remove.Reason = "nothing on disk to remove"
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.fetch.Layout(t, gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.licence.Layout(t, gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.remove.Layout(t, gtx) }),
		layout.Flexed(1, layout.Spacer{}.Layout),
		// Said rather than offered. Nothing here can fetch itself yet - the
		// SoftDevice deliberately will not, because it is somebody else's
		// licensed binary and the terms should arrive where a person sees
		// them. A tickbox that cannot be ticked is worse than a sentence.
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, autoLabel(auto))),
	)
}

func autoLabel(auto bool) string {
	if auto {
		return "fetched automatically"
	}
	return "only when asked"
}
