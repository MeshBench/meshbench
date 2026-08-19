// Two runs, metric by metric.
package workbench

import (
	"fmt"
	"math"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// comparePanel compares the two most recent saved runs.
//
// The two most recent rather than a pair chosen in a dialog, because the
// common case is "I just changed something and ran it again" and making
// somebody pick two rows to find that out is friction for nothing. Choosing a
// different pair is a job for the runs table's selection, later.
type comparePanel struct {
	tb     comp.Table
	init   bool
	loaded bool
	rows   []comp.Row
	head   string
	save   comp.Button
	// OnSave asks the store to record the current run.
	OnSave func()
}

func (p *comparePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "metric", Width: 190, Sortable: true},
			{Title: "older", Width: 130, Right: true, Mono: true},
			{Title: "newer", Width: 130, Right: true, Mono: true},
			{Title: "change", Width: 130, Right: true, Mono: true, Sortable: true},
			{Title: "", Mono: true},
		}
		p.save.Label, p.save.Kind = "save this run", comp.Primary
		p.init = true
	}
	if !p.loaded {
		p.loaded = true
		p.reload()
	}
	if p.save.Click.Clicked(gtx) {
		if p.OnSave != nil {
			p.OnSave()
		}
		p.loaded = false
	}

	body := func(gtx layout.Context) layout.Dimensions {
		if len(p.rows) == 0 {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				p.head))
		}
		p.tb.SetRows(p.rows)
		return p.tb.Layout(t, gtx, nil)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, p.head)),
		layout.Flexed(1, body),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return p.save.Layout(t, gtx)
		}),
	)
}

func (p *comparePanel) reload() {
	runs := session.LoadRuns()
	p.rows = nil
	switch {
	case len(runs) == 0:
		p.head = "no saved runs yet - save one, change something, and save another"
		return
	case len(runs) == 1:
		p.head = "one saved run (" + runs[0].Name + ") - save another to compare"
		return
	}
	newer, older := runs[0], runs[1]
	p.head = fmt.Sprintf("%s (%s) against %s (%s)",
		newer.Name, newer.At, older.Name, older.At)
	if newer.Seed != older.Seed {
		// Said, not hidden. Two runs of different seeds differ for a reason
		// that has nothing to do with whatever was changed between them.
		p.head += fmt.Sprintf("   - different seeds (%d and %d), so a difference here is not evidence",
			newer.Seed, older.Seed)
	}
	for _, name := range session.MetricNames(newer, older) {
		a, aok := older.Metrics[name]
		b, bok := newer.Metrics[name]
		cells := []string{name, fmtMetric(a, aok), fmtMetric(b, bok), "", ""}
		if aok && bok {
			cells[3] = changeOf(a, b)
			cells[4] = spark(a, b)
		} else {
			// Present in one run and not the other: not a change of zero.
			cells[3] = "only in one"
		}
		p.rows = append(p.rows, comp.Row{Key: name, Cells: cells})
	}
}

func fmtMetric(v float64, ok bool) string {
	if !ok {
		return "-"
	}
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// changeOf is the difference, as a percentage where that means anything.
//
// A percentage of zero is not a percentage, and printing "+Inf%" is how a
// table stops being read.
func changeOf(a, b float64) string {
	d := b - a
	if a == 0 {
		return fmt.Sprintf("%+.0f", d)
	}
	return fmt.Sprintf("%+.1f%%", d/a*100)
}

// spark is a text indicator of direction, so the column survives being copied
// out and does not rely on colour alone.
func spark(a, b float64) string {
	switch {
	case b > a:
		return "up"
	case b < a:
		return "down"
	}
	return "same"
}
