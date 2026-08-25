// Every firmware window that is open, and the one goroutine each of them runs.
//
// The same shape as nodeWindows and for the same reason: a window belongs to
// its own event loop, so another goroutine asking for one either opens it or
// leaves a wish for the loop to pick up. Nothing here knows what a firmware
// window looks like.
package workbench

import (
	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/float"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// firmwareWindows tracks which builds have a window, so a second request
// raises rather than opening a duplicate.
type firmwareWindows struct {
	*windowSet
}

func newFirmwareWindows() *firmwareWindows {
	return &firmwareWindows{windowSet: newWindowSet()}
}

// buildWindowKey identifies a build across all three of its names.
//
// All three, because a label alone is not unique: the same image imported for
// two boards, or one label carrying a repeater and a companion, would share a
// window and each would show the other's settings.
func buildWindowKey(role, version, board string) string {
	return role + "\x00" + version + "\x00" + board
}

func (w *firmwareWindows) openFor(role, version, board string,
	newTheme func() *theme.Theme, st *state.Store, do Do) {
	key := buildWindowKey(role, version, board)
	if !w.claim(key) {
		return
	}
	go func() {
		defer w.release(key)
		th := newTheme()
		p := &firmwareWindowPanel{role: role, version: version, board: board, OnDo: do}
		spot := float.NextSpot()
		win := new(app.Window)
		win.Option(append([]app.Option{
			app.Title("MeshBench - " + version),
			app.Size(unit.Dp(760), unit.Dp(680)),
		}, float.Above(spot, keepAbove(st))...)...)
		win.Perform(system.ActionRaise)
		var chrome *layerChrome
		var ops op.Ops
		for {
			switch e := win.Event().(type) {
			case app.ConfigEvent:
				if e.Config.LayerShell && chrome == nil {
					p.Layered, chrome = true, newLayerChrome(spot)
				}
				if chrome != nil {
					chrome.screens(e.Config.Output, e.Config.Outputs)
				}
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				if w.wantsRaise(key) {
					if chrome != nil {
						if opts := chrome.recall(float.NextSpot()); len(opts) > 0 {
							win.Option(opts...)
						}
					} else {
						win.Perform(system.ActionRaise)
					}
				}
				gtx := app.NewContext(&ops, e)
				comp.Fill(gtx, th.P.Ground)
				if chrome != nil {
					chrome.frame(e)
				}
				layout.UniformInset(th.Sp.M).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return p.Draw(th, gtx, st.Snapshot())
					})
				if chrome != nil {
					opts, close := chrome.update(&p.bar)
					p.maximised = chrome.maximised
					if close {
						win.Perform(system.ActionClose)
					} else if len(opts) > 0 {
						win.Option(opts...)
					}
				}
				e.Frame(gtx.Ops)
				win.Invalidate()
			}
		}
	}()
}
