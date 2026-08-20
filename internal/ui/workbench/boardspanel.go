// The hardware capability matrix: not can this board run, but which
// capabilities does it actually demonstrate on it - build, boot, radio, tx,
// rx, flood, fem, power - each measured rather than asserted, and untested
// kept visibly distinct from failed.
package workbench

import (
	"fmt"
	"image/color"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// boardCapCols is the matrix's own column order and short labels, matching
// boardcheck.Capabilities exactly - board, then one column per capability,
// then why not.
var boardCapCols = []struct{ key, label string }{
	{"build", "build"}, {"boot", "boot"}, {"radio", "radio"}, {"tx", "tx"},
	{"rx", "rx"}, {"flood", "flood"}, {"fem", "fem"}, {"power", "power"},
}

type boardsPanel struct {
	bar   actionBar
	board comp.Field
	vers  comp.Field
	probe comp.Button
	refr  comp.Button
	tb    comp.Table
	init  bool
	seq   uint64
	shown bool
	do    Do
}

func (p *boardsPanel) build() {
	p.board.Hint = "board name, e.g. Generic_E22_sx1262"
	p.vers.Hint = "board image version, e.g. v1.17.0 (blank: v1.17.0)"
	for _, f := range []*comp.Field{&p.board, &p.vers} {
		f.Editor.SingleLine = true
	}
	p.probe.Label, p.probe.Kind = "probe this board", comp.Primary
	p.refr.Label, p.refr.Kind = "refresh", comp.Secondary
	p.bar.fields = []*comp.Field{&p.board, &p.vers}
	p.bar.buttons = []*comp.Button{&p.probe, &p.refr}
	p.bar.note = "a probe is one real emulator boot, driven the same way the engine's own " +
		"live tests are - untested stays untested until it has actually run"
	p.tb.Cols = append([]comp.Column{{Title: "board", Width: 170, Sortable: true}},
		func() []comp.Column {
			cols := make([]comp.Column, 0, len(boardCapCols)+1)
			for _, c := range boardCapCols {
				cols = append(cols, comp.Column{Title: c.label, Width: 60, Right: true, Mono: true})
			}
			return append(cols, comp.Column{Title: "why not"})
		}()...)
	p.init = true
}

func (p *boardsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if p.probe.Click.Clicked(gtx) && p.do != nil {
		board := fieldText(&p.board)
		if board == "" {
			p.do("ui.said", "name a board to probe, or pick one from the matrix below")
		} else {
			params := map[string]any{"board": board}
			if v := fieldText(&p.vers); v != "" {
				params["version"] = v
			}
			p.do("board.probe", params)
		}
	}
	if p.refr.Click.Clicked(gtx) && p.do != nil {
		params := map[string]any{}
		if v := fieldText(&p.vers); v != "" {
			params["version"] = v
		}
		p.do("board.matrix", params)
	}

	if !p.shown || s.Seq != p.seq {
		rows := make([]comp.Row, 0, len(s.BoardMatrix))
		for _, b := range s.BoardMatrix {
			cells := make([]string, 0, len(b.Cells)+2)
			cells = append(cells, b.Board)
			whyNot := ""
			worst := "passed"
			for _, c := range b.Cells {
				cells = append(cells, symbolFor(c.State))
				if c.State == "failed" && whyNot == "" {
					whyNot = c.Detail
				}
				worst = worseCapState(worst, c.State)
			}
			if b.Stale {
				if whyNot != "" {
					whyNot += " (stale: re-probe)"
				} else {
					whyNot = "cached from a different emulator build - re-probe to confirm"
				}
			}
			cells = append(cells, whyNot)
			rows = append(rows, comp.Row{Key: b.Board, Cells: cells, Tint: capStateTint(t, worst)})
		}
		p.tb.SetRows(rows)
		p.seq, p.shown = s.Seq, true
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			ver := s.BoardMatrixVersion
			if ver == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Caption, t.P.Faint,
					fmt.Sprintf("measured against %s  ·  ✓ passed  ✗ failed  ?  untested  – n/a", ver), false))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.BoardMatrix) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"refresh to see every board, measured or not"))
			}
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}

func symbolFor(state string) string {
	switch state {
	case "passed":
		return "✓"
	case "failed":
		return "✗"
	case "n/a":
		return "–"
	default:
		return "?"
	}
}

// worseCapState ranks n/a and untested below passed but above failed is
// worse than everything - a board's row is only as good as its worst cell.
func worseCapState(current, next string) string {
	rank := map[string]int{"passed": 0, "n/a": 0, "untested": 1, "failed": 2}
	if rank[next] > rank[current] {
		return next
	}
	return current
}

func capStateTint(t *theme.Theme, worst string) [4]uint8 {
	var c color.NRGBA
	switch worst {
	case "passed":
		c = t.P.Good
	case "failed":
		c = t.P.Bad
	case "untested":
		return [4]uint8{}
	default:
		return [4]uint8{}
	}
	return [4]uint8{c.R, c.G, c.B, c.A}
}
