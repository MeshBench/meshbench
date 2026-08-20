// Importing a live network.
//
// Through internal/provider and internal/scenario, whose rule is the one worth
// keeping in front of somebody importing: a record that cannot be placed is
// reported, not invented. Every source gives partial data, and the temptation
// at each gap is to fill it with something reasonable - which produces a
// scenario full of confident fiction that nothing downstream can tell apart
// from the real parts.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// importPanel fetches a deployment and says what it found before anything is
// committed to the scenario.
type importPanel struct {
	tb   comp.Table
	init bool
	// OnRemove takes one area back out of the study, so a study built from
	// several places can drop one without starting over.
	OnRemove func(name string)
	// areaBtns are the per-area buttons, pooled by name; armed is the one
	// asked to confirm, because removing one changes what a fetch keeps.
	areaBtns map[string]*comp.Button
	armed    string
}

func (p *importPanel) areaBtn(name string) *comp.Button {
	if p.areaBtns == nil {
		p.areaBtns = map[string]*comp.Button{}
	}
	b := p.areaBtns[name]
	if b == nil {
		b = &comp.Button{Kind: comp.Quiet}
		p.areaBtns[name] = b
	}
	return b
}

func (p *importPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "outcome", Width: 260},
			{Title: "count", Width: 100, Right: true, Mono: true},
			{Title: "what it means"},
		}
		p.init = true
	}

	body := func(gtx layout.Context) layout.Dimensions {
		if s == nil || s.Import == nil {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				"nothing fetched yet - describing an import changes nothing until it is committed"))
		}
		im := s.Import
		rows := []comp.Row{
			{Key: "found", Cells: []string{"records fetched",
				fmt.Sprintf("%d", im.Records), "what the deployment published"}},
			{Key: "usable", Cells: []string{"nodes importable",
				fmt.Sprintf("%d", im.Nodes), "placed well enough to simulate"}},
			{Key: "nopos", Cells: []string{"skipped, no position",
				fmt.Sprintf("%d", im.SkippedNoPosition),
				"reported rather than invented; a node with no position cannot be placed"}},
			{Key: "outside", Cells: []string{"skipped, outside the area",
				fmt.Sprintf("%d", im.SkippedOutside),
				"beyond the boundary and its RF margin"}},
			{Key: "uncertain", Cells: []string{"placed loosely",
				fmt.Sprintf("%d", im.Uncertain),
				"imported and flagged; any result involving them carries it"}},
			{Key: "participants", Cells: []string{"participants",
				fmt.Sprintf("%d", im.Participants),
				"outside the area but kept for their RF - dropping them makes the mesh behave better than reality"}},
		}
		p.tb.SetRows(rows)
		return p.tb.Layout(t, gtx, nil)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "import a live network")),
		// The URL is asked for once, in the action bar above, beside the
		// numbered buttons that use it. A second box here took the same
		// answer and reached nothing at all.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return importAreaNote(t, gtx, s)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.areaRow(t, gtx, s)
		}),
		layout.Flexed(1, body),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Warn,
			"this describes what an import would do; it does not change the network")),
	)
}

// importAreaNote says which study area the import will be narrowed to,
// before anything is fetched.
//
// Before this the import took whatever the deployment published and the only
// way to narrow it was to commit the lot and prune - which measures every
// pair of four hundred nodes against the terrain and the buildings, then
// throws most of the answer away.
func importAreaNote(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Areas) == 0 {
		return comp.OneLine(t, t.Sz.Caption, t.P.Warn,
			"no study area: every node the deployment publishes will be imported - "+
				"set one in the Boundary panel first to import a county rather than a country",
			false)(gtx)
	}
	// Named once each: a fixture that already carried an area and an area
	// accepted on top of it read as "Fife, Fife, Fife", which says nothing
	// three times.
	seen := map[string]bool{}
	names := make([]string, 0, len(s.Areas))
	for _, a := range s.Areas {
		if a.Name == "" || seen[a.Name] {
			continue
		}
		seen[a.Name] = true
		names = append(names, a.Name)
	}
	margin := s.MarginKm
	if margin <= 0 {
		margin = 30
	}
	return comp.OneLine(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
		"study area: %s, plus %g km of margin - nodes outside it are not imported, "+
			"and those within the margin come in as participants",
		strings.Join(names, ", "), margin), false)(gtx)
}

// areaRow lists the study's areas, each removable.
//
// Here rather than in a panel of its own: the area is chosen for an import,
// and a separate tab for it reads as an unrelated feature somebody has to
// know to visit first.
func (p *importPanel) areaRow(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Areas) == 0 {
		return layout.Dimensions{}
	}
	seen := map[string]bool{}
	kids := []layout.FlexChild{
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "remove:  ")),
	}
	for _, a := range s.Areas {
		name := a.Name
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		b := p.areaBtn(name)
		b.Label, b.Kind = name, comp.Quiet
		if p.armed == name {
			b.Label, b.Kind = "sure? "+name, comp.Destructive
		}
		if b.Click.Clicked(gtx) {
			if p.armed == name {
				p.armed = ""
				if p.OnRemove != nil {
					p.OnRemove(name)
				}
			} else {
				p.armed = name
			}
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return b.Layout(t, gtx) })
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}
