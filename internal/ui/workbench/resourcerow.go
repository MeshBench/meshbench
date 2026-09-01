package workbench

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// resourceStateInk maps a row's state to the one colour that says it.
//
// Absent is Dim rather than Bad: a thing nobody has asked for yet is not a
// fault, and a red row for it teaches an operator to ignore red rows. Needed is
// the exception, because it is the one state that stops a scenario running.
//
// The names are resource.State's own. They used to be "present", "fetching" and
// "failed", which the provider has never emitted, so every row fell through to
// the default and a SoftDevice sitting on disk and feeding the simulation was
// labelled "not fetched" - the panel disagreeing with the directory beside it.
func resourceStateInk(t *theme.Theme, s string) (string, colorNRGBA) {
	switch resource.State(s) {
	case resource.OnDisk:
		return "on disk", t.P.Good
	case resource.InUse:
		return "in use", t.P.Good
	case resource.Needed:
		return "needed", t.P.Warn
	case resource.Unavailable:
		return "unavailable", t.P.Dim
	case resource.Available:
		return "not fetched", t.P.Dim
	default:
		return s, t.P.Dim
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

// shareBar draws how much of the cache one row accounts for.
//
// On a page about disk usage the useful number is the proportion, not the
// size: one row here is nearly all of it and the rest are rounding error, and
// four sizes in four different units do not say that at a glance.
func shareBar(t *theme.Theme, frac float64) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		// Fixed width, not the width available. Stretched across the card a
		// bar this thin reads as a rule between sections rather than as a
		// measurement of anything.
		h, w := gtx.Dp(t.Sp.S), gtx.Dp(shareBarWidth)
		if max := gtx.Constraints.Max.X; w > max {
			w = max
		}
		// The radius is the bar's own height: cornerRadius clamps it to half
		// the shorter side, which is exactly a capsule.
		comp.RoundRect(gtx, image.Point{X: w, Y: h}, t.Sp.S, t.P.Sunk)
		fill := int(float64(w) * frac)
		// A share too small to draw still gets a mark. A bar that renders as
		// nothing reads as none, and none is not what a tenth of a percent
		// of seven gigabytes means.
		if min := gtx.Dp(t.Sp.XS); fill < min {
			fill = min
		}
		comp.RoundRect(gtx, image.Point{X: fill, Y: h}, t.Sp.S, t.P.Accent)
		return layout.Dimensions{Size: image.Point{X: w, Y: h}}
	}
}

// shareBarWidth is the gauge's length. Wide enough that a few percent is a
// visible sliver rather than the minimum mark.
const shareBarWidth = unit.Dp(240)

// sharePct is the share as words, keeping a real but tiny share distinct from
// nothing at all.
func sharePct(frac float64) string {
	switch pc := frac * 100; {
	case pc >= 10:
		return fmt.Sprintf("%.0f%% of the cache", pc)
	case pc >= 0.1:
		return fmt.Sprintf("%.1f%% of the cache", pc)
	default:
		return "under 0.1% of the cache"
	}
}

func (p *resourcesPanel) row(t *theme.Theme, gtx layout.Context,
	r state.ResourceRow) layout.Dimensions {
	key := resourceKey(r)
	w := p.rowFor(key)
	// On disk or feeding the simulation: either way the file is here, which is
	// what the buttons below care about. Tested against "present", a name the
	// provider does not use, Remove was offered on nothing and refused on
	// everything.
	st := resource.State(r.State)
	present := st == resource.OnDisk || st == resource.InUse

	if w.fetch.Click.Clicked(gtx) {
		p.do("resource.fetch", map[string]any{
			"name": r.Name, "version": r.Version, "kind": r.Kind})
	}
	if w.licence.Click.Clicked(gtx) {
		// Pressed again it closes, and closing is a verb of its own rather
		// than a flag the panel keeps to itself. Both halves of a toggle
		// should be things a script can do.
		verb := "resource.licence"
		if p.licenceShown(r) {
			verb = "resource.licence.hide"
		}
		p.do(verb, map[string]any{
			"name": r.Name, "version": r.Version, "kind": r.Kind})
	}
	if w.remove.Click.Clicked(gtx) {
		if p.confirm == key {
			p.confirm = ""
			p.do("resource.remove", map[string]any{
				"name": r.Name, "version": r.Version, "kind": r.Kind})
		} else {
			p.confirm = key
		}
	}

	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, comp.Card(t, "",
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rowHead(t, gtx, r)
				}),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, rowMeta(r))),
				// Why is content, not a tooltip: it is the row's answer to
				// "and why is it in that state", and a state without one is
				// how an operator concludes the application is stuck.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if r.Why == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Dim, r.Why))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rowShare(t, gtx, r, present)
				}),
				layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.rowActions(t, gtx, w, key, present, r)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.licenceBox(t, gtx, r)
				}),
			)
		}))
}

// rowHead is the name, then the two facts read across from it: how big, and
// what state it is in.
func (p *resourcesPanel) rowHead(t *theme.Theme, gtx layout.Context,
	r state.ResourceRow) layout.Dimensions {
	label, ink := resourceStateInk(t, r.State)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, r.Name)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// A version is compared down a column, so it is mono - and a row
			// that has none says nothing rather than trailing a space.
			if r.Version == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: t.Sp.S}.Layout(gtx,
				comp.Mono(t, t.Sz.Caption, t.P.Dim, r.Version))
		}),
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(comp.Mono(t, t.Sz.Data, t.P.Ink, resourceSize(r))),
		layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
		layout.Rigid(comp.Pill(t, ink, label)),
	)
}

// rowShare is the proportion bar, drawn only for bytes that were counted.
//
// An estimate has no place on it: the bar is a comparison of what is actually
// on the disk, and a guess drawn beside measurements would be read as one.
func (p *resourcesPanel) rowShare(t *theme.Theme, gtx layout.Context,
	r state.ResourceRow, present bool) layout.Dimensions {
	if !present || r.Estimated || p.onDisk <= 0 || r.Bytes <= 0 {
		return layout.Dimensions{}
	}
	frac := float64(r.Bytes) / float64(p.onDisk)
	return layout.Inset{Top: t.Sp.S}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(shareBar(t, frac)),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, sharePct(frac))),
				layout.Flexed(1, comp.Spacer),
			)
		})
}

// rowMeta is the kind and how it got here, on one line. The second half used to
// float alone at the far right of the row, a caption with nothing to attach to.
func rowMeta(r state.ResourceRow) string {
	return r.Kind + " - " + autoLabel(r.Auto)
}

func (p *resourcesPanel) rowActions(t *theme.Theme, gtx layout.Context,
	w *resRowW, key string, present bool, r state.ResourceRow) layout.Dimensions {
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
	// A cache that fills itself has nothing to ask for out of context. The
	// button says so rather than sitting there accepting a press that would
	// do nothing - which is how an operator concludes downloading is broken.
	w.fetch.Disabled = !r.Fetchable
	w.fetch.Reason = ""
	if !r.Fetchable {
		w.fetch.Reason = "fills itself as the map is used"
		// Except where the row has its own answer. An emulator with no build
		// for this machine is not a cache, and telling somebody it fills
		// itself as the map is used explains nothing about the thing in front
		// of them.
		if resource.State(r.State) == resource.Unavailable && r.Why != "" {
			w.fetch.Reason = r.Why
		}
	}
	w.licence.Label = "Licence"
	if p.licenceShown(r) {
		w.licence.Label = "Hide licence"
	}
	w.licence.Disabled = !r.Licensed
	w.licence.Reason = ""
	if !r.Licensed {
		w.licence.Reason = "no terms recorded"
		if r.Fetchable && !present {
			w.licence.Reason = "terms arrive with the file"
		}
	}
	w.remove.Disabled = !present
	w.remove.Reason = ""
	if !present {
		w.remove.Reason = "nothing on disk"
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.fetch.Layout(t, gtx) }),
		layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.licence.Layout(t, gtx) }),
		// Remove sits away from the two safe actions rather than beside them.
		// Destructive things should not be the next button along.
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.remove.Layout(t, gtx) }),
	)
}

func autoLabel(auto bool) string {
	if auto {
		return "fetched automatically"
	}
	return "only when asked"
}
