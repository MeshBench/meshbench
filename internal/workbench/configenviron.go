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
			"OpenStreetMap - live Overpass pull, tags carry heights and materials",
			"Microsoft Global ML footprints - worldwide, heights from imagery",
		}, func(picked string) {
			if p.do == nil {
				return
			}
			source := "osm"
			if strings.HasPrefix(picked, "Microsoft") {
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
				"picking a database pulls footprints for the loaded map's "+
					"area, caches them like terrain, and switches buildings "+
					"on; a pull too large for a live download says so and "+
					"points at tools/envgen")),
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
