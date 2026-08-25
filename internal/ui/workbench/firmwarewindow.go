// One build, in a window of its own.
//
// The library is a list and a list has to stay a list: role, version, size, a
// tick. Everything else about a build - where the file actually is, whether it
// is a whole flash image or half of one, what it has been told to do at reset,
// what the person who imported it wanted the next one to know - had nowhere to
// be shown and no way to be changed. It is shown here, beside the build it is
// about, in the place somebody is already looking when a build does not do
// what they expected.
//
// Every change goes through a verb. A window is not allowed to be a second way
// of editing the cache: it asks firmware.update and reads the result back out
// of the next snapshot, exactly as a script driving the same verb would.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

type firmwareWindowPanel struct {
	// role, version and board are which build this window is about. They are
	// written when a rename succeeds, so the window follows the build it was
	// opened on rather than emptying the moment it is renamed.
	role, version, board string

	OnDo Do

	// Layered and bar are the chrome a compositor without decorations needs,
	// on the same terms as a node window.
	Layered   bool
	bar       comp.TitleBar
	maximised bool

	built  bool
	list   widget.List
	name   comp.Field
	notes  comp.Field
	coproc comp.Check

	// roleWant and boardWant are the draft: what the chips and the board
	// list say, which is not the same as what the build is until apply. They
	// are separate from role and board above for exactly that reason - the
	// identity is how the window finds its build in the library, and writing
	// a half-made choice into it made the window lose the build the moment
	// somebody picked a different role.
	roleWant   string
	boardWant  string
	roleChips  map[string]*comp.Chip
	boardChips map[string]*comp.Chip
	boardBtn   comp.Button
	boardList  widget.List
	boardPick  bool

	apply   comp.Button
	revert  comp.Button
	useFor  comp.Button
	del     comp.Button
	confirm bool

	// seeded is the build the editors were last filled from. A window whose
	// build changes underneath it - renamed from a script, or its notes
	// edited elsewhere - refills rather than showing a stale draft; a window
	// somebody is typing in does not.
	seeded string
}

func (p *firmwareWindowPanel) build() {
	p.list.Axis = layout.Vertical
	p.boardList.Axis = layout.Vertical
	p.name.Label, p.name.Hint = "Name", "what the library calls this build"
	p.name.Editor.SingleLine = true
	p.notes.Label = "Notes"
	p.notes.Hint = "what the next person should know about this build"
	p.coproc.Label = "coprocessors enabled at reset"
	p.apply.Label, p.apply.Kind = "apply", comp.Primary
	p.revert.Label, p.revert.Kind = "revert", comp.Secondary
	p.useFor.Label, p.useFor.Kind = "use for this role", comp.Secondary
	p.del.Label, p.del.Kind = "delete", comp.Secondary
	p.boardBtn.Kind = comp.Secondary
	p.roleChips = map[string]*comp.Chip{}
	for _, r := range importRoles() {
		p.roleChips[r] = &comp.Chip{}
	}
	p.boardChips = map[string]*comp.Chip{}
	p.built = true
}

// row is the build this window is about, as the last library rebuild saw it.
func (p *firmwareWindowPanel) row(s *state.Snapshot) (state.FirmwareRow, bool) {
	if s == nil {
		return state.FirmwareRow{}, false
	}
	for i := range s.Library {
		r := s.Library[i]
		if r.Role == p.role && r.Version == p.version && r.Board == p.board {
			return r, true
		}
	}
	return state.FirmwareRow{}, false
}

// seed fills the editors from the build, when it is a different build from the
// one they were filled from.
func (p *firmwareWindowPanel) seed(r state.FirmwareRow) {
	key := buildWindowKey(r.Role, r.Version, r.Board) + "\x00" + r.Settings.Notes
	if p.seeded == key {
		return
	}
	p.seeded = key
	p.name.Editor.SetText(r.Version)
	p.notes.Editor.SetText(r.Settings.Notes)
	p.coproc.Bool.Value = r.Settings.CoprocAtReset
	p.roleWant, p.boardWant = r.Role, r.Board
}

func (p *firmwareWindowPanel) Draw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	if !p.built {
		p.build()
	}
	r, found := p.row(s)
	if !found {
		return p.missing(t, gtx)
	}
	p.seed(r)
	p.act(gtx, r)

	var kids []layout.FlexChild
	// The window's own chrome when nothing else gave it any: a layer-shell
	// window has no title bar but the one drawn here.
	if p.Layered {
		p.bar.Title, p.bar.Maximised = p.version, p.maximised
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.bar.Layout(t, gtx)
		}))
	}
	kids = append(kids,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(t, gtx, r)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			sections := []func(layout.Context) layout.Dimensions{
				func(gtx layout.Context) layout.Dimensions { return p.identity(t, gtx, r) },
				func(gtx layout.Context) layout.Dimensions { return p.howItRuns(t, gtx, r) },
				func(gtx layout.Context) layout.Dimensions { return p.facts(t, gtx, r) },
			}
			return comp.List(t, &p.list, len(sections),
				func(gtx layout.Context, i int) layout.Dimensions {
					return sections[i](gtx)
				})(gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.actions(t, gtx, r)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

// missing is what a window shows once its build has gone: deleted from under
// it, or renamed by something else. Saying so beats an empty window, and
// beats guessing which build was meant.
func (p *firmwareWindowPanel) missing(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	where := p.board
	if where == "" {
		where = "this machine"
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink,
				p.role+" "+p.version+" for "+where)),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
				"is no longer in the library - it has been deleted or renamed elsewhere")),
		)
	})
}

func (p *firmwareWindowPanel) header(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	where := "this machine"
	if r.Board != "" {
		where = r.Board + " (emulated)"
	}
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return roleIcon(t, gtx, r.Role)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, r.Version)),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, r.Role+" - "+where)),
				)
			}),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.stateChip(t, gtx, r)
			}),
		)
	})
}

// stateChip is the one thing worth saying at a glance: whether a board could
// start from this, or why not.
func (p *firmwareWindowPanel) stateChip(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	label, ink := "on disk", t.P.Good
	switch {
	case !r.OnDisk && r.Unavailable:
		label, ink = "not published for this machine", t.P.Warn
	case !r.OnDisk:
		label, ink = "not downloaded", t.P.Dim
	case r.Native:
		label = "runs on this machine"
	case r.Facts.Kind != "" && !r.Facts.Bootable:
		label, ink = r.Facts.Kind, t.P.Bad
	}
	return comp.Text(t, t.Sz.Caption, ink, label)(gtx)
}

func (p *firmwareWindowPanel) sectionTitle(t *theme.Theme, title, why string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: t.Sp.M, Bottom: t.Sp.S}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, title)),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, why)),
				)
			})
	}
}

// fact is one labelled line, the value in mono so a path can be read.
func fact(t *theme.Theme, label, value string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						w := gtx.Dp(unitDp(110))
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
						d := comp.Text(t, t.Sz.Caption, t.P.Faint, label)(gtx)
						d.Size.X = w
						return d
					}),
					layout.Flexed(1, comp.Mono(t, t.Sz.Caption, t.P.Dim, value)),
				)
			})
	}
}

func humanBytes(n int64) string {
	switch {
	case n <= 0:
		return "-"
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}

func strOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
