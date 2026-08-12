// Importing a live network (6.21).
//
// Through internal/provider and internal/scenario, whose rule is the one worth
// keeping in front of somebody importing: a record that cannot be placed is
// reported, not invented. Every source gives partial data, and the temptation
// at each gap is to fill it with something reasonable - which produces a
// scenario full of confident fiction that nothing downstream can tell apart
// from the real parts.
package main

import (
	"context"
	"fmt"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// importPanel fetches a deployment and says what it found before anything is
// committed to the scenario.
type importPanel struct {
	url   comp.Field
	fetch comp.Button
	tb    comp.Table
	init  bool
	// OnFetch asks the store to import from this URL.
	OnFetch func(url string)
}

func (p *importPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.url.Label = "CoreScope deployment"
		p.url.Hint = "https://example.compute.oarc.uk"
		p.fetch.Label, p.fetch.Kind = "fetch and describe", comp.Primary
		p.tb.Cols = []comp.Column{
			{Title: "outcome", Width: 260},
			{Title: "count", Width: 100, Right: true, Mono: true},
			{Title: "what it means"},
		}
		p.init = true
	}
	if p.fetch.Click.Clicked(gtx) && p.OnFetch != nil {
		p.OnFetch(p.url.Editor.Text())
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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.url.Layout(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return p.fetch.Layout(t, gtx)
		}),
		layout.Flexed(1, body),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Warn,
			"this describes what an import would do; it does not change the network")),
	)
}

// importFrom fetches a deployment and summarises what an import would produce.
//
// Describes rather than commits. Replacing the loaded network with a fetched
// one is a destructive act on somebody's work, and the useful half of an
// import panel is the part that tells you what you are about to get.
func importFrom(ctx context.Context, url string) (*state.Import, error) {
	if url == "" {
		return nil, fmt.Errorf("no deployment URL")
	}
	cs := &provider.CoreScope{BaseURL: url}
	records, err := cs.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	res, err := scenario.Import(records, scenario.ImportOptions{
		DefaultBoard: "RAK4631",
		// The band this project works in. Stated rather than defaulted,
		// because no source publishes a modem configuration and a wrong one
		// makes every imported link wrong in the same direction. EU/UK
		// narrow, 4/8, which is what the shipped fixtures were built on -
		// and leaving the coding rate at zero is rejected rather than
		// defaulted, which is how this was caught.
		Radio:            scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62.5e3, SpreadFactor: 8, CodingRate: 4},
		MaxUncertaintyKm: 1,
	})
	if err != nil {
		return nil, err
	}
	return &state.Import{
		URL: url, Records: len(records), Nodes: len(res.Nodes),
		SkippedNoPosition: res.SkippedNoPosition,
		SkippedOutside:    res.SkippedOutside,
		Uncertain:         res.Uncertain, Participants: res.Participants,
	}, nil
}
