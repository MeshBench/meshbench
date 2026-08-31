// The Firmware Library, to Alex's mock: a page, not a spreadsheet.
//
// A header that says what the page is for with the actions beside it, counted
// tabs for the three views, search and a role filter, and generous rows - a
// tinted role icon, the version, how it runs, its size, a tick for on disk,
// who uses it, a bordered use-for-role control and a delete that asks twice.
// The footer says how much is shown and what the marks mean. Deliberately no
// "Latest" mark: version names here include branches and study builds, and a
// chip that guesses wrong crowns somebody's experiment - Alex's call.
package workbench

import (
	"image"
	"image/color"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Small aliases that keep the drawing code readable.
type colorNRGBA = color.NRGBA

func unitDp(v int) unit.Dp              { return unit.Dp(v) }
func imagePtXY(x, y int) image.Point    { return image.Pt(x, y) }
func imageRectPt(d int) image.Rectangle { return image.Rect(0, 0, d, d) }

type fwRowW struct {
	use, act widget.Clickable
	// open is the row itself: a press on the role or the version opens the
	// build's own window. The two buttons keep their own areas, so the
	// clickable covers only the cells that are not already controls.
	open widget.Clickable
}

type firmwarePanel struct {
	tabAll, tabDisk, tabMachine, tabBoards comp.Chip
	tab                                    int
	search                                 comp.Field
	filterBtn                              comp.Chip
	roleFilter                             string
	importBtn                              comp.Button
	refreshBtn                             comp.Button
	scanBtn                                comp.Button
	list                                   widget.List
	rows                                   map[string]*fwRowW
	confirm                                string
	built                                  bool
	asked                                  bool
	// OnAction sends a verb about one build; Refresh asks for the library
	// again; OnImport asks for a path and a role; choose opens the chooser.
	OnAction func(verb string, params map[string]any)
	Refresh  func()
	OnImport func()
	choose   func(title string, opts []string, pick func(string))
}

func (p *firmwarePanel) build() {
	p.importBtn.Label, p.importBtn.Kind = "import...", comp.Secondary
	p.refreshBtn.Label, p.refreshBtn.Kind = "refresh", comp.Secondary
	p.scanBtn.Label, p.scanBtn.Kind = "scan builds", comp.Primary
	p.search.Hint = "search builds..."
	p.search.Editor.SingleLine = true
	p.rows = map[string]*fwRowW{}
	p.list.Axis = layout.Vertical
	p.built = true
}

// The column grid, one place: label and width per column, zero meaning "the
// rest".
var fwCols = []struct {
	label string
	width int
}{
	{"Role", 210}, {"Version", 250}, {"Runs as", 140}, {"Size", 70},
	{"On disk", 70}, {"Used by", 90}, {"Use for role", 150}, {"Actions", 70},
}

func (p *firmwarePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
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
	if p.tabBoards.Click.Clicked(gtx) {
		p.tab = 3
	}
	if p.importBtn.Click.Clicked(gtx) && p.OnImport != nil {
		p.OnImport()
	}
	if p.refreshBtn.Click.Clicked(gtx) && p.Refresh != nil {
		p.Refresh()
	}
	if p.scanBtn.Click.Clicked(gtx) {
		if p.OnAction != nil {
			p.OnAction("firmware.rescan", nil)
		}
		if p.Refresh != nil {
			p.Refresh()
		}
	}
	if p.filterBtn.Click.Clicked(gtx) && p.choose != nil && s != nil {
		seen := map[string]bool{}
		opts := []string{"every role"}
		for i := range s.Library {
			if r := s.Library[i].Role; !seen[r] {
				seen[r] = true
				opts = append(opts, r)
			}
		}
		p.choose("Only this role", opts, func(picked string) {
			if picked == "every role" {
				picked = ""
			}
			p.roleFilter = picked
		})
	}
	if s == nil {
		return layout.Dimensions{}
	}

	nAll, nDisk, nMachine, nBoards := 0, 0, 0, 0
	for i := range s.Library {
		nAll++
		if s.Library[i].OnDisk {
			nDisk++
		}
		// A build with no board runs as this machine; one with a board is a
		// published image for real hardware, run under emulation.
		if s.Library[i].Board == "" {
			nMachine++
		} else {
			nBoards++
		}
	}

	want := strings.ToLower(fieldText(&p.search))
	shown := make([]state.FirmwareRow, 0, len(s.Library))
	for i := range s.Library {
		r := s.Library[i]
		if p.tab == 1 && !r.OnDisk {
			continue
		}
		if p.tab == 2 && r.Board != "" {
			continue
		}
		if p.tab == 3 && r.Board == "" {
			continue
		}
		if p.roleFilter != "" && r.Role != p.roleFilter {
			continue
		}
		if want != "" && !strings.Contains(strings.ToLower(r.Role), want) &&
			!strings.Contains(strings.ToLower(r.Version), want) &&
			!strings.Contains(strings.ToLower(r.Board), want) {
			continue
		}
		shown = append(shown, r)
	}
	sort.Slice(shown, func(i, j int) bool {
		if shown[i].Role != shown[j].Role {
			return shown[i].Role < shown[j].Role
		}
		return shown[i].Version < shown[j].Version
	})

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.tabRow(t, gtx, nAll, nDisk, nMachine, nBoards)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.colHeads(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(shown) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
					"nothing here - other tabs may hold builds, and scan builds asks the catalogue"))
			}
			return comp.List(t, &p.list, len(shown), func(gtx layout.Context, i int) layout.Dimensions {
				return p.row(t, gtx, s, shown[i])
			})(gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.footer(t, gtx, len(shown), nAll)
		}),
	)
}

// borderedAction is the mock's per-row control: a bordered rounded box that
// reads as pressable without shouting like a primary button.
func borderedAction(t *theme.Theme, gtx layout.Context, ck *widget.Clickable,
	label string, line, ink colorNRGBA) layout.Dimensions {
	return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if ck.Hovered() {
			ink = t.P.Ink
		}
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S,
		}.Layout(gtx, comp.Text(t, t.Sz.Caption, ink, label))
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 5, theme.Alpha(t.P.Sunk, 0.6))
		comp.Border(gtx, dims.Size, 5, 1, line)
		call.Add(gtx.Ops)
		return dims
	})
}
