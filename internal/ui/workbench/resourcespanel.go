// Resources: everything the application fetches at runtime that is not firmware.
//
// The Firmware Library's page shape, because it is the same question asked of
// different things - what is on this machine, how big is it, and may we go and
// get the rest. What it must not do is imply more certainty than it has: a size
// that was guessed says so, a licence that has to be read is offered rather
// than assumed, and a thing that is absent says why rather than showing zero.
package workbench

import (
	"fmt"
	"github.com/MeshBench/meshbench/internal/app/resource"
	"sort"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// resRowW is one row's widgets, pooled by key. Widget identity is address, so
// these are built once and kept - a button rebuilt each frame forgets it was
// pressed.
type resRowW struct {
	fetch, remove, licence comp.Button
}

type resourcesPanel struct {
	refreshBtn comp.Button
	list       widget.List
	rows       map[string]*resRowW

	// confirm holds the key of the row whose removal has been asked once.
	// Destructive things ask twice, in place, and never behind a modal.
	confirm string

	// Refresh and OnAction are wired by the panel list; a panel owns no store.
	Refresh  func()
	OnAction func(verb string, params map[string]any)

	asked bool
}

func (p *resourcesPanel) rowFor(key string) *resRowW {
	if p.rows == nil {
		p.rows = map[string]*resRowW{}
	}
	w, ok := p.rows[key]
	if !ok {
		w = &resRowW{}
		w.fetch.Kind = comp.Secondary
		w.licence.Kind = comp.Quiet
		w.licence.Label = "Licence"
		w.remove.Kind = comp.Destructive
		p.rows[key] = w
	}
	return w
}

// resourceKey is the identity a row's widgets are pooled by. Widget identity is
// address, so it has to be stable across frames and unique across rows.
func resourceKey(r state.ResourceRow) string {
	return r.Kind + "/" + r.Name + "/" + r.Version
}

func (p *resourcesPanel) do(verb string, params map[string]any) {
	if p.OnAction != nil {
		p.OnAction(verb, params)
	}
}

func (p *resourcesPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.asked && p.Refresh != nil {
		p.asked = true
		p.Refresh()
	}
	p.refreshBtn.Kind, p.refreshBtn.Label = comp.Secondary, "Rescan"
	if p.refreshBtn.Click.Clicked(gtx) && p.Refresh != nil {
		p.Refresh()
	}

	rows := append([]state.ResourceRow(nil), s.Resources...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		return rows[i].Name < rows[j].Name
	})

	p.list.Axis = layout.Vertical
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.header(t, gtx, rows)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(rows) == 0 {
					return p.empty(t, gtx)
				}
				return comp.List(t, &p.list, len(rows),
					func(gtx layout.Context, i int) layout.Dimensions {
						return p.row(t, gtx, rows[i])
					})(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"Sizes marked ~ are estimates, not measurements. "+
					"Nothing here is fetched without being asked.")),
		)
	})
}

func (p *resourcesPanel) header(t *theme.Theme, gtx layout.Context,
	rows []state.ResourceRow) layout.Dimensions {
	var onDisk int
	var total int64
	for _, r := range rows {
		// resource.State's own names. This tested "present", which nothing
		// emits, so the count was always zero and the header said nothing was
		// here while a fetched SoftDevice sat in the row beneath it.
		switch resource.State(r.State) {
		case resource.OnDisk, resource.InUse:
			onDisk++
			total += r.Bytes
		}
	}
	// "0 of 1, -" put a dash where a size belongs, which reads as a broken
	// number rather than as nothing being here yet.
	sub := "none of them on this machine yet"
	if onDisk > 0 {
		sub = fmt.Sprintf("%d of %d here, %s on disk", onDisk, len(rows), siBytes(total))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, "Resources")),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
					"What this build downloads that is not firmware - "+sub+".")),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.refreshBtn.Layout(t, gtx)
		}),
	)
}

func (p *resourcesPanel) empty(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Dim, "Nothing to list yet.")),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"Rescan asks the cache what is there. An empty list means the cache is "+
					"empty, not that this build downloads nothing.")),
		)
	})
}
