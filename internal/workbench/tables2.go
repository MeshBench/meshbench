// The other two tables of P4: installed firmware, and past runs.
//
// Both read the same places on disk the rest of the tool writes to, rather
// than keeping a list of their own. A second inventory of what is installed
// is an inventory that goes stale the first time somebody uses the CLI.
package workbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// firmwarePanel is what is installed and could be run (6.x, P4).
// firmwarePanel is the library: every build, what runs it, and what can be
// done to it.
//
// A table with the actions on the rows, because the form it replaced asked
// somebody to type a role, a version and a path that were all on the screen
// already - and offered download, delete and use-for-role as three buttons
// that acted on whatever had been typed rather than on anything being looked
// at.
// buildKey names a row, and splitBuildKey reads it back. Keyed on what a build
// is rather than on its path, because a build that has not been downloaded has
// no path and still has to be a row.
func buildKey(r state.FirmwareRow) string {
	return r.Role + "\x00" + r.Version + "\x00" + r.Board
}

func splitBuildKey(k string) (role, version, board string) {
	parts := strings.SplitN(k, "\x00", 3)
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	return parts[0], parts[1], parts[2]
}

func findLibraryRow(s *state.Snapshot, key string) *state.FirmwareRow {
	for i := range s.Library {
		if buildKey(s.Library[i]) == key {
			return &s.Library[i]
		}
	}
	return nil
}

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
