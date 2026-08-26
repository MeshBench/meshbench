// The sections of a firmware window, and what its controls do.
//
// Split from the window itself only for length: the file above is the window,
// this is what is in it.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// identity is what the build is called, and what it claims to be for.
func (p *firmwareWindowPanel) identity(t *theme.Theme, gtx layout.Context,
	r state.FirmwareRow) layout.Dimensions {

	why := "the name the library lists it under, and the name a node pins. " +
		"Renaming moves the file and repoints anything using it"
	if r.Native {
		why = "a build for this machine is named after the host it runs on, " +
			"so there is nothing here to change"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(p.sectionTitle(t, "Identity", why)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.Native {
				return layout.Dimensions{}
			}
			return p.name.Layout(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.Native {
				return fact(t, "role", r.Role)(gtx)
			}
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.roleRow(t, gtx) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if r.Native {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.boardRow(t, gtx) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// What a rename would take with it, said before it happens rather
			// than as a surprise afterwards: a node pinned to the old name
			// would otherwise fail at its next start with "no image in the
			// cache", about a build sitting in the library under a new name.
			if r.InUse == 0 || !p.renaming(r) {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Warn,
					fmt.Sprintf("%d nodes run this build and will be repointed to the new name",
						r.InUse)))
		}),
	)
}

// roleRow is the role, as chips: there are four and the runner only matches
// these, so a free-text role is a build the runner would never select.
func (p *firmwareWindowPanel) roleRow(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	kids := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Dp(unitDp(110))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
			d := comp.Text(t, t.Sz.Caption, t.P.Faint, "role")(gtx)
			d.Size.X = w
			return d
		}),
	}
	for _, role := range importRoles() {
		role := role
		chip := p.roleChips[role]
		if chip.Click.Clicked(gtx) {
			p.roleWant = role
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return chip.Layout(t, gtx, shortRole(role), "", p.roleWant == role, t.P.Accent)
				})
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}

// shortRole is a role with the common prefix dropped, so four of them fit on
// one line without the window being a metre wide.
func shortRole(role string) string {
	switch role {
	case "companion_radio_usb":
		return "companion (usb)"
	case "companion_radio":
		return "companion"
	case "simple_repeater":
		return "repeater"
	case "simple_room_server":
		return "room server"
	}
	return role
}

// boardRow is which hardware the image is for: a button that opens the list
// in place, because the window has no shared chooser to post to and forty
// boards is not a row of chips.
func (p *firmwareWindowPanel) boardRow(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	p.boardBtn.Label = strOr(p.boardWant, hostBuildChoice)
	if p.boardBtn.Click.Clicked(gtx) {
		p.boardPick = !p.boardPick
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					w := gtx.Dp(unitDp(110))
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
					d := comp.Text(t, t.Sz.Caption, t.P.Faint, "board")(gtx)
					d.Size.X = w
					return d
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.boardBtn.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !p.boardPick {
				return layout.Dimensions{}
			}
			return p.boardChoices(t, gtx)
		}),
	)
}

func (p *firmwareWindowPanel) boardChoices(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	boards := importBoards()
	gtx.Constraints.Max.Y = gtx.Dp(unitDp(180))
	return layout.Inset{Top: t.Sp.XS, Left: t.Sp.M}.Layout(gtx,
		comp.List(t, &p.boardList, len(boards), func(gtx layout.Context, i int) layout.Dimensions {
			name := boards[i]
			if p.boardOpt(name).Click.Clicked(gtx) {
				p.boardWant = name
				if name == hostBuildChoice {
					p.boardWant = ""
				}
				p.boardPick = false
			}
			chip := p.boardOpt(name)
			on := p.boardWant == name || (p.boardWant == "" && name == hostBuildChoice)
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return chip.Layout(t, gtx, name, "", on, t.P.Accent)
				})
		}))
}

// boardOpt keeps one chip per board name, made on first sight: Gio identifies
// a widget by its address, so a chip built fresh each frame is a different
// widget every frame and never registers a press.
func (p *firmwareWindowPanel) boardOpt(name string) *comp.Chip {
	if p.boardChips[name] == nil {
		p.boardChips[name] = &comp.Chip{}
	}
	return p.boardChips[name]
}
