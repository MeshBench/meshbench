// Regression scenarios: "will this stay fixed?" - a directory of portable
// cases, each pinning a fixture, seeds and firmware, run on real firmware
// and checked against assertions with a tolerance band derived from the
// spread they were captured with.
package workbench

import (
	"fmt"
	"image/color"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

type regressionsPanel struct {
	bar    actionBar
	dir    comp.Field
	run    comp.Button
	export comp.Button
	tb     comp.Table
	init   bool
	seq    uint64
	shown  bool
	do     Do
}

func (p *regressionsPanel) build() {
	p.dir.Hint = "directory of regression cases (blank: the default)"
	p.dir.Editor.SingleLine = true
	p.run.Label, p.run.Kind = "run all", comp.Primary
	p.export.Label, p.export.Kind = "export from sweep", comp.Secondary
	p.bar.fields = []*comp.Field{&p.dir}
	p.bar.buttons = []*comp.Button{&p.run, &p.export}
	p.bar.note = "a case run reads real firmware, so 41 scenarios takes about as long as 41 short sweeps"
	p.tb.Cols = []comp.Column{
		{Title: "scenario", Width: 190, Sortable: true},
		{Title: "seeds", Width: 60, Right: true, Mono: true},
		{Title: "verdict", Width: 80, Sortable: true},
		{Title: "detail"},
	}
	p.tb.SortCol = 0
	p.init = true
}

func (p *regressionsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if p.run.Click.Clicked(gtx) && p.do != nil {
		params := map[string]any{}
		if d := fieldText(&p.dir); d != "" {
			params["dir"] = d
		}
		p.do("regression.run_dir", params)
	}
	if p.export.Click.Clicked(gtx) && p.do != nil {
		p.do("regression.export", nil)
	}

	if !p.shown || s.Seq != p.seq {
		rows := make([]comp.Row, 0, len(s.Regressions))
		for _, r := range s.Regressions {
			rows = append(rows, comp.Row{
				Key: r.Name,
				Cells: []string{
					r.Name, fmt.Sprintf("%d", r.Seeds), r.Verdict, r.Detail,
				},
				Tint: regressionTint(t, r.Verdict),
			})
		}
		p.tb.SetRows(rows)
		p.seq, p.shown = s.Seq, true
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dir := s.RegressionsDir
			if dir == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Caption, t.P.Faint, "last run: "+dir, false))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.Regressions) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"run all, or export a case from the Sweep panel first"))
			}
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}

// regressionTint colours the row swatch: green for a pass, amber for a
// stochastic metric flagged outside its band, red for a hard failure or a
// case that could not be loaded at all.
func regressionTint(t *theme.Theme, verdict string) [4]uint8 {
	var c color.NRGBA
	switch verdict {
	case "pass":
		c = t.P.Good
	case "flag":
		c = t.P.Warn
	case "fail", "error":
		c = t.P.Bad
	default:
		return [4]uint8{}
	}
	return [4]uint8{c.R, c.G, c.B, c.A}
}
