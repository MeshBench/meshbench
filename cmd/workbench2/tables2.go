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
	tb comp.Table
	// The three views of the library, as tabs with counts.
	tabAll, tabDisk, tabMachine comp.Chip
	tab                         int
	search                      comp.Field
	importBtn                   comp.Button
	refreshBtn                  comp.Button
	init                        bool
	asked                       bool
	confirm                     string
	// OnAction sends a verb about one build.
	OnAction func(verb string, params map[string]any)
	// Refresh asks for the library again, after something changed it.
	Refresh func()
	// OnImport asks for a path and imports the build there.
	OnImport func()
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
		p.importBtn.Label, p.importBtn.Kind = "import...", comp.Secondary
		p.refreshBtn.Label, p.refreshBtn.Kind = "refresh", comp.Secondary
		p.search.Hint = "search builds"
		p.init = true
	}
	// Asked for once, and again whenever something changes it: reading a
	// directory sixty times a second to find out whether a download finished
	// is not how that question gets answered.
	if !p.asked && p.Refresh != nil {
		p.asked = true
		p.Refresh()
	}
	if p.tabAll.Click.Clicked(gtx) {
		p.tab = 0
	}
	if p.tabDisk.Click.Clicked(gtx) {
		p.tab = 1
	}
	if p.tabMachine.Click.Clicked(gtx) {
		p.tab = 2
	}
	if p.importBtn.Click.Clicked(gtx) && p.OnImport != nil {
		p.OnImport()
	}
	if p.refreshBtn.Click.Clicked(gtx) && p.Refresh != nil {
		p.Refresh()
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

	// The three counts, before filtering, so a tab says what it holds.
	nAll, nDisk, nMachine := 0, 0, 0
	for i := range s.Library {
		nAll++
		if s.Library[i].OnDisk {
			nDisk++
		}
		if s.Library[i].Board == "" {
			nMachine++
		}
	}
	// The newest version per role gets the swatch the mock gives a "Latest"
	// chip - the table draws strings, so the mark is the row's tint and the
	// legend says so.
	latest := map[string]string{}
	for i := range s.Library {
		r := s.Library[i]
		if laxVersionLess(latest[r.Role], r.Version) {
			latest[r.Role] = r.Version
		}
	}

	rows := make([]comp.Row, 0, len(s.Library))
	for i := range s.Library {
		r := s.Library[i]
		if p.tab == 1 && !r.OnDisk {
			continue
		}
		if p.tab == 2 && r.Board != "" {
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
		row := comp.Row{Key: buildKey(r), Cells: []string{
			r.Role, r.Version, runsAs, disk, used, "use for this role", act,
		}}
		if latest[r.Role] == r.Version {
			g := t.P.Good
			row.Tint = [4]uint8{g.R, g.G, g.B, 255}
		}
		rows = append(rows, row)
	}
	p.tb.SetRows(rows)

	chip := func(c *comp.Chip, label string, n, tab int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return c.Layout(t, gtx, label, fmt.Sprintf("%d", n), p.tab == tab, t.P.Accent)
				})
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(comp.SectionTitle(t, "Firmware Library")),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
							"the builds this machine can run; what is in the cache is "+
								"the only thing that decides what a node can run")),
					)
				}),
				layout.Flexed(1, comp.Spacer),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return p.importBtn.Layout(t, gtx)
						})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.refreshBtn.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						chip(&p.tabAll, "All builds", nAll, 0),
						chip(&p.tabDisk, "On disk only", nDisk, 1),
						chip(&p.tabMachine, "This machine only", nMachine, 2),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = gtx.Dp(200)
							return p.search.LayoutEditor(t, gtx, &p.tb.Filter)
						}),
					)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(rows) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
					"nothing published for this machine and nothing downloaded"))
			}
			return p.tb.Layout(t, gtx, nil)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
							fmt.Sprintf("showing %d of %d builds", len(rows), nAll))),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(comp.Dot(t.P.Good, gtx.Dp(3))),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
							"  the newest for its role    delete asks twice")),
					)
				})
		}),
	)
}

// laxVersionLess orders versions well enough to mark the newest: numeric
// runs compare as numbers, so v1.9 sits below v1.17.
func laxVersionLess(a, b string) bool {
	if a == "" {
		return b != ""
	}
	ai, bi := 0, 0
	for ai < len(a) && bi < len(b) {
		ca, cb := a[ai], b[bi]
		if isDigit(ca) && isDigit(cb) {
			ja, jb := ai, bi
			for ja < len(a) && isDigit(a[ja]) {
				ja++
			}
			for jb < len(b) && isDigit(b[jb]) {
				jb++
			}
			na, nb := a[ai:ja], b[bi:jb]
			// Compare as numbers: longer digit runs are bigger, equal-length
			// runs compare lexically.
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			ai, bi = ja, jb
			continue
		}
		if ca != cb {
			return ca < cb
		}
		ai++
		bi++
	}
	return len(a)-ai < len(b)-bi
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

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
