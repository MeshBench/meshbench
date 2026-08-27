// What a build has been told to do, where it lives, and what its controls do.
//
// Split from the identity half only for length: together they were past the
// limit, and the seam is the natural one - above is what the build is called,
// here is how it is run and where it actually sits.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// howItRuns is the settings kept beside the build: what the emulator is told
// before this particular image starts.
func (p *firmwareWindowPanel) howItRuns(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	if r.Native {
		return layout.Dimensions{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(p.sectionTitle(t, "How it is run",
			"kept beside the image, so it follows this build and not the board")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.coproc.LayoutSwitch(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.S}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint,
					"The part resets with its coprocessors off and the firmware decides "+
						"which tasks may use them. A firmware whose exception handler saves "+
						"floating point state before anything has enabled them traps inside "+
						"its own vector, loops, and hides everything behind it. On, that is "+
						"visible - and a firmware that genuinely mismanages the register is "+
						"flattered, so leave it off unless a board looks dead."))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.card.LayoutSwitch(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.S}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint,
					"A firmware that keeps its settings on a card boots into nothing "+
						"without one. On, every node running this build is given a card "+
						"whatever its own slot was set to - which is better than a boot "+
						"failure minutes into a run, in a message that never mentions "+
						"cards."))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.notes.Layout(t, gtx)
		}),
	)
}

// facts is where the build actually is and what it actually contains.
func (p *firmwareWindowPanel) facts(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	image := "-"
	if r.Facts.Kind != "" {
		image = r.Facts.Kind
		if r.Facts.FlashMB > 0 {
			image += fmt.Sprintf(", for a %d MB chip", r.Facts.FlashMB)
		}
	} else if r.Native {
		image = "an executable for this machine"
	}
	used := "no nodes"
	if r.InUse == 1 {
		used = "1 node"
	} else if r.InUse > 1 {
		used = fmt.Sprintf("%d nodes", r.InUse)
	}
	modified := "-"
	if !r.Modified.IsZero() {
		modified = r.Modified.Format("2006-01-02 15:04:05")
	}
	settings := "-"
	if r.Path != "" {
		settings = firmware.SettingsPath(r.Path)
		if r.Settings == (firmware.BuildSettings{}) {
			settings += "  (nothing decided yet, so not written)"
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(p.sectionTitle(t, "Where it is",
			"what is in the cache is the only thing that decides what a node can run")),
		layout.Rigid(fact(t, "file", strOr(r.Path, "not on this machine"))),
		layout.Rigid(fact(t, "settings", settings)),
		layout.Rigid(fact(t, "size", humanBytes(r.Bytes))),
		layout.Rigid(fact(t, "modified", modified)),
		layout.Rigid(fact(t, "image", image)),
		layout.Rigid(fact(t, "used by", used)),
	)
}

// actions is the row along the bottom: what can be done to the build itself.
func (p *firmwareWindowPanel) actions(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	p.del.Label = "delete"
	if p.confirm {
		p.del.Label = "sure?"
	}
	changed := p.changed(r)
	p.apply.Label = "apply"
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.OnDisk {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.useFor.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !r.OnDisk {
					return layout.Dimensions{}
				}
				return p.del.Layout(t, gtx)
			}),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !changed {
					return comp.Text(t, t.Sz.Caption, t.P.Faint, "no unsaved changes")(gtx)
				}
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.revert.Layout(t, gtx) })
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !changed {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions { return p.apply.Layout(t, gtx) })
			}),
		)
	})
}

// renaming reports whether the identity in the editors differs from the build.
func (p *firmwareWindowPanel) renaming(r state.FirmwareRow) bool {
	return !r.Native && (strings.TrimSpace(fieldText(&p.name)) != r.Version ||
		p.roleWant != r.Role || p.boardWant != r.Board)
}

// changed reports whether anything at all is unsaved.
func (p *firmwareWindowPanel) changed(r state.FirmwareRow) bool {
	return p.renaming(r) ||
		p.coproc.Bool.Value != r.Settings.CoprocAtReset ||
		p.card.Bool.Value != r.Settings.CardRequired ||
		fieldText(&p.notes) != r.Settings.Notes
}

// act runs the controls. Every one of them goes through a verb.
func (p *firmwareWindowPanel) act(gtx layout.Context, r state.FirmwareRow) {
	if p.revert.Click.Clicked(gtx) {
		// Re-seed from the build by forgetting what the editors were filled
		// from, rather than by writing them here twice.
		p.seeded = ""
		p.roleWant, p.boardWant = r.Role, r.Board
		p.confirm = false
	}
	if p.apply.Click.Clicked(gtx) && p.OnDo != nil {
		name := strings.TrimSpace(fieldText(&p.name))
		params := map[string]any{
			"version": r.Version, "role": r.Role, "board": r.Board,
			"label": name, "new_role": p.roleWant, "new_board": p.boardWant,
			"coproc_at_reset": p.coproc.Bool.Value,
			"card_required":   p.card.Bool.Value,
			"notes":           fieldText(&p.notes),
		}
		// A board of "" is the host build, and an absent new_board means
		// "leave it alone" - so it has to be said in a way that survives the
		// difference. The verb reads an empty string as unchanged, which is
		// right for every case except moving a board image to the host, and
		// that is not a move anybody makes: the two are different kinds of
		// file entirely.
		p.OnDo("firmware.update", params)
		// Follow the build to its new name. If the verb refuses, the toast
		// says why and the window finds no such row - which is recoverable,
		// because reverting puts the old identity back.
		//
		// The editors are not cleared here. The seed key is built from the
		// identity, so a rename refills them by itself when the renamed build
		// appears in the next snapshot; clearing them as well would drop the
		// draft in the window between the ask and the answer.
		p.role, p.version, p.board = p.roleWant, name, p.boardWant
		p.confirm = false
	}
	if p.useFor.Click.Clicked(gtx) && p.OnDo != nil {
		p.OnDo("firmware.set", map[string]any{
			"role": r.Role, "version": r.Version,
		})
	}
	if p.del.Click.Clicked(gtx) {
		// Deleting a build the scenario is using leaves those nodes unable to
		// start, and the failure arrives at play rather than here - so the
		// second press is the destructive one, exactly as in the library.
		if !p.confirm {
			p.confirm = true
		} else if p.OnDo != nil {
			p.confirm = false
			p.OnDo("firmware.delete", map[string]any{
				"role": r.Role, "version": r.Version,
				"board": r.Board, "path": r.Path,
			})
		}
	}
}

// auditDraw lays the window out in two columns, so the control audit can
// reach every control at once.
//
// The window itself scrolls its sections, and a control below the fold is a
// control the audit would press at zero size and call dead. Two columns rather
// than one tall one because stacked flat the sections are taller than the
// audit's canvas, and the action row at the bottom fell off it - which is
// exactly the failure this is meant to catch, arriving as a false one.
func (p *firmwareWindowPanel) auditDraw(t *theme.Theme, gtx layout.Context,
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
	column := func(kids ...layout.Widget) layout.FlexChild {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			var children []layout.FlexChild
			for _, k := range kids {
				children = append(children, layout.Rigid(k))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	}
	return layout.Flex{}.Layout(gtx,
		column(
			func(gtx layout.Context) layout.Dimensions { return p.header(t, gtx, r) },
			func(gtx layout.Context) layout.Dimensions { return p.identity(t, gtx, r) },
			func(gtx layout.Context) layout.Dimensions { return p.actions(t, gtx, r) },
		),
		column(
			func(gtx layout.Context) layout.Dimensions { return p.howItRuns(t, gtx, r) },
			func(gtx layout.Context) layout.Dimensions { return p.facts(t, gtx, r) },
		),
	)
}
