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
type firmwarePanel struct {
	tb      comp.Table
	onDisk  comp.Check
	native  comp.Check
	init    bool
	asked   bool
	confirm string
	// OnAction sends a verb about one build.
	OnAction func(verb string, params map[string]any)
	// Refresh asks for the library again, after something changed it.
	Refresh func()
}

func (p *firmwarePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "role", Width: 170, Sortable: true},
			{Title: "version", Width: 210, Mono: true, Sortable: true},
			{Title: "runs as", Width: 150, Sortable: true},
			{Title: "on disk", Width: 110, Right: true, Mono: true, Sortable: true},
			{Title: "used by", Width: 100, Right: true, Mono: true, Sortable: true},
			{Title: "", Width: 130, Menu: true},
			{Title: "", Width: 110, Menu: true},
		}
		p.onDisk.Label = "on disk only"
		p.native.Label = "this machine only"
		p.init = true
	}
	// Asked for once, and again whenever something changes it: reading a
	// directory sixty times a second to find out whether a download finished
	// is not how that question gets answered.
	if !p.asked && p.Refresh != nil {
		p.asked = true
		p.Refresh()
	}
	for _, c := range []*comp.Check{&p.onDisk, &p.native} {
		c.Bool.Update(gtx)
	}
	if s == nil {
		return layout.Dimensions{}
	}

	p.tb.OnCell = func(key string, col int) {
		role, version, board := splitBuildKey(key)
		switch col {
		case 5:
			if p.OnAction != nil {
				p.OnAction("firmware.set", map[string]any{
					"role": role, "version": version,
				})
			}
		case 6:
			row := findLibraryRow(s, key)
			switch {
			case row == nil:
			case !row.OnDisk:
				if p.OnAction != nil {
					p.OnAction("firmware.download", map[string]any{
						"role": role, "version": version, "board": board,
					})
				}
			case p.confirm != key:
				// Deleting a build the scenario is using leaves those nodes
				// unable to start, and the failure arrives at play rather
				// than here, so the second press is the destructive one.
				p.confirm = key
			default:
				p.confirm = ""
				if p.OnAction != nil {
					p.OnAction("firmware.delete", map[string]any{
						"role": role, "version": version, "board": board,
					})
				}
			}
		}
	}

	rows := make([]comp.Row, 0, len(s.Library))
	for i := range s.Library {
		r := s.Library[i]
		if p.onDisk.Bool.Value && !r.OnDisk {
			continue
		}
		if p.native.Bool.Value && r.Board != "" {
			continue
		}
		runsAs := "this machine"
		if r.Board != "" {
			// A board image is emulated hardware: one emulator per node, in
			// real time, which is a different proposition from a host build
			// and should never read as the same thing.
			runsAs = r.Board + " (emulated)"
		}
		disk := "not here"
		if r.OnDisk {
			disk = fmt.Sprintf("%.1f MB", float64(r.Bytes)/(1<<20))
		}
		used := "-"
		if r.InUse > 0 {
			used = fmt.Sprintf("%d nodes", r.InUse)
		}
		act := "download"
		if r.OnDisk {
			act = "delete"
			if p.confirm == buildKey(r) {
				act = "sure?"
			}
		}
		rows = append(rows, comp.Row{Key: buildKey(r), Cells: []string{
			r.Role, r.Version, runsAs, disk, used, "use for this role", act,
		}})
	}
	p.tb.SetRows(rows)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.onDisk.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.native.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
					fmt.Sprintf("%d builds", len(rows)))),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(rows) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
					"nothing published for this machine and nothing downloaded"))
			}
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}

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
