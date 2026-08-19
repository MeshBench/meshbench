// Firmware from the node table: what a node is running, what it could run,
// and the script it is told at boot.
package workbench

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// firmwarePicker offers every installed build for the selected node.
//
// It says which node it would change, because a row of version buttons with
// nothing naming the target is a control somebody presses and then has to go
// and check.
// firmwareList is the open dropdown for one node's firmware cell.
//
// A list rather than a row of buttons, because the number of installed builds
// is whatever somebody has installed - nine already overflowed a row, and a
// control that works until you install a tenth is not a control.
func (p *nodeViewPanel) firmwareList(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.pickFor == "" {
			return comp.Text(t, t.Sz.Caption, t.P.Faint,
				"click a firmware cell to change what that node runs")(gtx)
		}
		if p.closePick.Label == "" {
			p.closePick.Label, p.closePick.Kind = "cancel", comp.Quiet
		}
		if p.closePick.Click.Clicked(gtx) {
			p.pickFor, p.pickFilter.Editor = "", widget.Editor{}
			return layout.Dimensions{}
		}
		for i := range p.buildBtns {
			if p.buildBtns[i].Click.Clicked(gtx) && p.OnFirmware != nil {
				p.OnFirmware(p.pickFor, p.builds[i])
				p.pickFor = ""
				return layout.Dimensions{}
			}
		}

		// Which builds survive the box.
		//
		// Thirty-nine are installed on this machine. A horizontal row of
		// thirty-nine buttons is not a control, and it put cancel where the
		// first click lands - so choosing a build closed the list instead.
		want := strings.ToLower(strings.TrimSpace(p.pickFilter.Editor.Text()))
		shown := p.shownBuilds[:0]
		for i := range p.builds {
			if want == "" || strings.Contains(strings.ToLower(p.builds[i]), want) {
				shown = append(shown, i)
			}
		}
		p.shownBuilds = shown

		p.buildList.Axis = layout.Vertical
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink,
						"What should "+p.pickFor+" run?")),
					layout.Flexed(1, comp.Spacer),
					btn(t, &p.closePick),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				p.pickFilter.Hint = fmt.Sprintf("filter %d builds", len(p.builds))
				p.pickFilter.Editor.SingleLine = true
				return p.pickFilter.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(shown) == 0 {
					return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption,
						t.P.Faint, "nothing matches that"))
				}
				return comp.List(t, &p.buildList, len(shown),
					func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.buildBtns[shown[i]].Layout(t, gtx)
							})
					})(gtx)
			}),
		)
	}
}

// installedBuilds is the native builds this machine has, newest name last.
func installedBuilds() []string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range firmware.ListInstalled(filepath.Join(cache, "meshcoresim", "firmware")) {
		if f.Native {
			out = append(out, f.Version)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// cpuTime prints processor time at a scale somebody can read.
//
// Milliseconds while a node has barely run, then seconds - rather than a
// percentage, which for fifty nodes ticking over is fifty readings of 0.3% and
// tells nobody which one has done the work.
func cpuTime(ms int64) string {
	switch {
	case ms <= 0:
		return "-"
	case ms < 1000:
		return fmt.Sprintf("%d ms", ms)
	case ms < 60000:
		return fmt.Sprintf("%.2f s", float64(ms)/1000)
	}
	return fmt.Sprintf("%dm %02ds", ms/60000, (ms%60000)/1000)
}

// provisioningScript shows what a node is sent before a run.
//
// In the console's own voice, monospaced and in order, because the point is
// that it is the same text somebody would type - a script they can copy into a
// terminal and watch fail there, rather than a description of what the
// application does somewhere they cannot see.
func provisioningScript(t *theme.Theme, s *state.Snapshot) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Provisioning) == 0 {
			return layout.Dimensions{}
		}
		kids := []layout.FlexChild{
			layout.Rigid(comp.SectionTitle(t, s.ProvisioningNode+" is told, at boot:")),
		}
		for _, l := range s.Provisioning {
			l := l
			col := t.P.Ink
			if l.Comment {
				col = t.P.Faint
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// A comment is already prose, so it takes the whole width
				// rather than being cut off by a column sized for commands.
				if l.Comment {
					return comp.OneLine(t, t.Sz.Data, col, l.Command+"  -  "+l.Why, false)(gtx)
				}
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(230)
						gtx.Constraints.Max.X = gtx.Dp(230)
						return comp.Mono(t, t.Sz.Data, col, l.Command)(gtx)
					}),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, l.Why, false)),
				)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	}
}

// firmwareOverlay draws the build list over the panel, centred.
func (p *nodeViewPanel) firmwareOverlay(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	comp.FillRect(gtx, gtx.Constraints.Max, theme.Alpha(t.P.Ground, 0.86))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(520), gtx.Dp(420)
		if w > gtx.Constraints.Max.X {
			w = gtx.Constraints.Max.X
		}
		if h > gtx.Constraints.Max.Y {
			h = gtx.Constraints.Max.Y
		}
		gtx.Constraints.Min = image.Pt(w, h)
		gtx.Constraints.Max = image.Pt(w, h)
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, p.firmwareList(t))
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 6, t.P.Panel)
		comp.Border(gtx, dims.Size, 6, 1, t.P.Rule)
		call.Add(gtx.Ops)
		return dims
	})
}

// OpenFirmware and OpenMenu show a node's controls without a click.
//
// Scriptable for the same reason the search box and the pop-out are: a control
// that only opens under a hand cannot be captured, and a thing nobody can
// screenshot is a thing nobody checks. This is now the third time that has
// bitten, so it goes in with the control rather than after it.
func (p *nodeViewPanel) OpenFirmware(node string) { p.pickFor = node }
