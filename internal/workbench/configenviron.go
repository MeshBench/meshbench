// The Buildings card: what stands in the way of the paths, and where to get
// it - either a directory of prepared tiles, or a database pulled live for
// the loaded map's area.
package workbench

import (
	"strings"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// wireEnvironSources arms the database dropdown. Picking a database is the
// action: it pulls for the current map and switches buildings on.
func (p *configPanel) wireEnvironSources() {
	p.envDir.Hint, p.envDir.Label = "a tile directory from tools/envgen", "Environment tiles"
	p.envDir.Editor.SingleLine = true
	p.loadEnv.Label, p.loadEnv.Kind = "load buildings", comp.Secondary
	p.dropEnv.Label, p.dropEnv.Kind = "bare earth", comp.Quiet
	p.envSrcDD.Label = "Building database"
	p.envSrcDD.Value = "download for the map's area"
	p.envSrcDD.OnOpen = func() {
		if p.choose == nil {
			return
		}
		p.choose("Pull building footprints from", []string{
			"Microsoft + OSM merged - footprints everywhere, OSM types and " +
				"materials override (the plan's shape)",
			"OpenStreetMap alone - live Overpass pull, tags carry heights and materials",
			"Microsoft alone - worldwide ML footprints, heights from imagery, no types",
		}, func(picked string) {
			if p.do == nil {
				return
			}
			source := "merged"
			switch {
			case strings.HasPrefix(picked, "OpenStreetMap"):
				source = "osm"
			case strings.HasPrefix(picked, "Microsoft alone"):
				source = "microsoft"
			}
			p.do("environ.fetch", map[string]any{"source": source})
		})
	}
}

func (p *configPanel) buildingsCard(t *theme.Theme, s *state.Snapshot) layout.Widget {
	return comp.Card(t, "Buildings", func(gtx layout.Context) layout.Dimensions {
		now := "bare earth - no environment loaded"
		if s.RFEnvironment != "" {
			now = "pricing buildings from " + s.RFEnvironment
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.envSrcDD.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"picking a database pulls footprints in patches around each "+
					"node - not the whole bounding box, which for a national "+
					"network is mostly empty ground - caches them like "+
					"terrain, and switches buildings on; a pull still too "+
					"large says so and points at tools/envgen")),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(p.fieldRow(t, &p.envDir, &p.loadEnv, now+". Tiles come "+
				"from tools/envgen over Microsoft/OSM footprints; buildings add "+
				"knife edges and wall loss to the path budget in both RF modes, "+
				"and missing tiles are counted, never mistaken for empty ground")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.dropEnv.Layout(t, gtx)
			}),
		)
	})
}
