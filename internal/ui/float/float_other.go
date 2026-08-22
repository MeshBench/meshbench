//go:build !windows && !darwin

package float

import "gioui.org/app"

// Linux and the BSDs: a Wayland session can have the overlay ask, and an X11
// one has no ask a client can make without an X connection of its own. The
// ask is the machine's preference - it buys pinning at the price of a window
// with no decoration of the compositor's - and the spot is only used where
// the ask succeeded.
func above(spot Spot, keep bool) []app.Option {
	if keep && onWayland() {
		return []app.Option{app.LayerShell(app.LayerShellConfig{
			Layer:     app.LayerOverlay,
			Keyboard:  app.KeyboardOnDemand,
			Anchor:    app.EdgeTop | app.EdgeLeft,
			MarginTop: spot.Top, MarginLeft: spot.Left,
		})}
	}
	return nil
}
