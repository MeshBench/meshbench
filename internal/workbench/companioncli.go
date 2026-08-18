// The command line, over the same claim the client holds.
//
// Its own file because the companion tab is at the file limit, and because
// this is the one mode that is not decoded state: meshcore-cli speaks the
// firmware's text console, so what arrives is lines, and the only structure
// this can add is picking out the echo from the failures.
package workbench

import (
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// cliPane is the command line, over the same session the client uses.
func (c *companionTab) cliPane(t *theme.Theme, gtx layout.Context, s *state.Snapshot, cs state.Companion) layout.Dimensions {
	lines := []string(nil)
	if s != nil {
		lines = s.Consoles[c.node]
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(lines) == 0 {
				return centreNote(t, gtx,
					"no output yet.\nType a command, or ? for the list.")
			}
			c.cliList.ScrollToEnd = true
			return comp.List(t, &c.cliList, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
				return comp.Mono(t, t.Sz.Data, cliInk(t, lines[i]), lines[i])(gtx)
			})(gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return c.cmd.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.runCmd.Layout(t, gtx)
					}),
				)
			})
		}),
	)
}

// cliInk picks out the echo and the failures, so a transcript can be read at
// a glance rather than word by word.
func cliInk(t *theme.Theme, line string) colorNRGBA {
	switch {
	case strings.HasPrefix(line, "> "):
		return t.P.Ink
	case strings.HasPrefix(line, "error:"), strings.HasPrefix(line, "no command"),
		strings.HasPrefix(line, "meshcore-cli has"):
		return t.P.Bad
	}
	return t.P.Dim
}
