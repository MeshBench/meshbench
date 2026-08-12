// The other two tables of P4: installed firmware, and past runs.
//
// Both read the same places on disk the rest of the tool writes to, rather
// than keeping a list of their own. A second inventory of what is installed
// is an inventory that goes stale the first time somebody uses the CLI.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// firmwarePanel is what is installed and could be run (6.x, P4).
type firmwarePanel struct {
	tb     comp.Table
	init   bool
	loaded bool
	rows   []comp.Row
}

func (p *firmwarePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "version", Width: 190, Mono: true, Sortable: true},
			{Title: "role", Width: 170, Sortable: true},
			{Title: "target", Width: 140, Sortable: true},
			{Title: "size", Width: 96, Right: true, Mono: true, Sortable: true},
			{Title: "path", Mono: true},
		}
		p.init = true
	}
	// Read once. The catalogue changes when somebody installs firmware, which
	// is not something that happens between two frames, and stat-ing a
	// directory sixty times a second to find that out would be absurd.
	if !p.loaded {
		p.loaded = true
		p.rows = firmwareRows()
	}
	p.tb.SetRows(p.rows)
	if len(p.rows) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no firmware installed - meshbench firmware install"))
	}
	return p.tb.Layout(t, gtx, nil)
}

func firmwareRows() []comp.Row {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	installed := firmware.ListInstalled(filepath.Join(cache, "meshcoresim", "firmware"))
	rows := make([]comp.Row, 0, len(installed))
	for _, f := range installed {
		target := f.Board
		if f.Native {
			// Native builds carry no board, and printing an empty cell there
			// reads as missing data rather than as the answer.
			target = "this machine"
		}
		rows = append(rows, comp.Row{
			Key: f.Path,
			Cells: []string{
				f.Version, f.Role, target,
				fmt.Sprintf("%.1f MB", float64(f.Bytes)/(1<<20)),
				f.Path,
			},
		})
	}
	return rows
}

// runsPanel is every run with its build checksum (6.13).
//
// The checksum is the point. Three arms of a study once returned identical
// numbers and the question - was the firmware actually different - could not
// be answered from the results, only from the provenance beside them.
type runsPanel struct {
	tb     comp.Table
	init   bool
	loaded bool
	rows   []comp.Row
}

func (p *runsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "when", Width: 160, Mono: true, Sortable: true},
			{Title: "run", Width: 200, Sortable: true},
			{Title: "seed", Width: 80, Right: true, Mono: true, Sortable: true},
			{Title: "build", Width: 140, Mono: true},
			{Title: "result"},
		}
		p.tb.SortCol, p.tb.SortDesc, p.init = 0, true, true
	}
	if !p.loaded {
		p.loaded = true
		p.rows = runRows()
	}
	p.tb.SetRows(p.rows)
	if len(p.rows) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no runs recorded yet - a run appears here once one has been saved"))
	}
	return p.tb.Layout(t, gtx, nil)
}

// runRows reads the run records the CLI writes.
func runRows() []comp.Row {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	dir := filepath.Join(cache, "meshcoresim", "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var rows []comp.Row
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var r struct {
			Name    string `json:"name"`
			At      string `json:"at"`
			Seed    uint64 `json:"seed"`
			Build   string `json:"build"`
			Outcome string `json:"outcome"`
		}
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		when := r.At
		if ts, err := time.Parse(time.RFC3339, r.At); err == nil {
			when = ts.Format("2006-01-02 15:04:05")
		}
		rows = append(rows, comp.Row{
			Key:   e.Name(),
			Cells: []string{when, r.Name, fmt.Sprintf("%d", r.Seed), r.Build, r.Outcome},
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Cells[0] > rows[j].Cells[0] })
	return rows
}
