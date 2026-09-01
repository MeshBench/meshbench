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
	"sort"

	"github.com/MeshBench/meshbench/internal/app/resource"

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
	// where takes somebody to the page that can get this, for a row that
	// cannot get it itself. A pointer, made only for the rows that name such
	// a page: a button that exists on every row and is drawn on one is a
	// control nothing can click, which the panel audit counts as a fault.
	where *comp.Button
}

type resourcesPanel struct {
	refreshBtn comp.Button
	list       widget.List
	rows       map[string]*resRowW

	// onDisk is what the rows add up to this frame, so each can say what
	// share of it it is. Render state, recomputed before the list draws.
	onDisk int64

	// licence is the terms the store last read, and the row they belong to.
	// Which row is open is a fact about the world here rather than about the
	// panel, so that opening one is something a script can do.
	licence state.LicenceText

	// confirm holds the key of the row whose removal has been asked once.
	// Destructive things ask twice, in place, and never behind a modal.
	confirm string

	// Refresh and OnAction are wired by the panel list; a panel owns no store.
	Refresh  func()
	OnAction func(verb string, params map[string]any)

	asked bool
}

func (p *resourcesPanel) rowFor(r state.ResourceRow) *resRowW {
	if p.rows == nil {
		p.rows = map[string]*resRowW{}
	}
	key := resourceKey(r)
	w, ok := p.rows[key]
	if !ok {
		w = &resRowW{}
		w.fetch.Kind = comp.Secondary
		w.licence.Kind = comp.Quiet
		w.licence.Label = "Licence"
		w.remove.Kind = comp.Destructive
		p.rows[key] = w
	}
	// Made for the rows that name a page and no others, so the panel audit
	// never meets a button nothing draws.
	if r.HowToPanel != "" && w.where == nil {
		w.where = &comp.Button{Kind: comp.Secondary}
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

	sum := totalsOf(rows)
	p.onDisk = sum.bytes
	p.licence = s.Licence

	p.list.Axis = layout.Vertical
	return layout.UniformInset(t.Sp.L).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.header(t, gtx, rows, sum)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(rows) == 0 {
					return layout.Dimensions{}
				}
				return p.cards(t, gtx, rows, sum)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(rows) == 0 {
					return p.empty(t, gtx)
				}
				return comp.List(t, &p.list, len(rows),
					func(gtx layout.Context, i int) layout.Dimensions {
						return p.row(t, gtx, rows[i])
					})(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"Sizes marked ~ are estimates, not measurements. "+
					"Nothing here is fetched without being asked.")),
		)
	})
}

// totals is what the page says about itself, counted once per frame.
type totals struct {
	onDisk  int
	bytes   int64
	auto    int64
	asked   int64
	largest state.ResourceRow
}

func totalsOf(rows []state.ResourceRow) totals {
	var s totals
	for _, r := range rows {
		// resource.State's own names. This tested "present", which nothing
		// emits, so the count was always zero and the header said nothing was
		// here while a fetched SoftDevice sat in the row beneath it.
		switch resource.State(r.State) {
		case resource.OnDisk, resource.InUse:
		default:
			continue
		}
		s.onDisk++
		s.bytes += r.Bytes
		if r.Auto {
			s.auto += r.Bytes
		} else {
			s.asked += r.Bytes
		}
		if r.Bytes > s.largest.Bytes {
			s.largest = r
		}
	}
	return s
}

// cards are the four questions this page exists to answer: how much is here,
// what is most of it, and how much of that arrived without anybody deciding.
func (p *resourcesPanel) cards(t *theme.Theme, gtx layout.Context,
	rows []state.ResourceRow, s totals) layout.Dimensions {
	cells := []layout.Widget{
		comp.StatCell(t, "On disk", siOrDash(s.bytes),
			fmt.Sprintf("%d of %d listed here", s.onDisk, len(rows))),
		comp.StatCell(t, "Largest", siOrDash(s.largest.Bytes), largestCaption(s)),
		comp.StatCell(t, "Filled itself", siOrDash(s.auto),
			"cached as the map was used"),
		comp.StatCell(t, "Asked for", siOrDash(s.asked),
			"fetched deliberately, licence and all"),
	}
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return comp.CellGrid(t, gtx, 150, cells)
	})(gtx)
}

// siOrDash keeps nothing distinct from zero: a page about disk usage that
// prints "0 B" for a cache it has not measured is lying about the disk.
func siOrDash(b int64) string {
	if b <= 0 {
		return "-"
	}
	return siBytes(b)
}

func largestCaption(s totals) string {
	if s.largest.Name == "" {
		return "nothing measured yet"
	}
	return s.largest.Name
}

func (p *resourcesPanel) header(t *theme.Theme, gtx layout.Context,
	rows []state.ResourceRow, s totals) layout.Dimensions {
	// The counting moved to the cards below. The subtitle says what the page
	// is; the numbers say what is in it, and saying both twice is how a header
	// and its own contents start disagreeing.
	sub := "What this build downloads that is not firmware."
	if len(rows) > 0 && s.onDisk == 0 {
		sub = "What this build downloads that is not firmware - " +
			"none of it on this machine yet."
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, "Resources")),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, sub)),
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
