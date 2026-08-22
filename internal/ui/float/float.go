// Package float keeps a window above the others, where the platform allows a
// client to ask for that at all.
//
// macOS and Windows have always-on-top as a property of a normal window, so
// Above asks for it when the window is made, and that is the whole story.
//
// Linux is the one place the ask has a price, so it is a preference: on by
// default, settable with ui.keep_above, and only meaningful under Wayland.
// There, no client may ask a normal window to stay above others - the
// compositor decides, which is what a KWin window rule is for - so Above asks
// for the one thing a client can: a wlr-layer-shell surface on the overlay
// layer (our Gio fork), which the compositor stacks above every normal window
// for as long as it is mapped. The price is what the protocol forbids the
// compositor from giving such a surface: no title bar, no taskbar entry, no
// minimise. The window draws its own title bar (comp.TitleBar) whose close
// returns the panel to the main window; and it is placed by margins, so the
// bar's drag moves it. On compositors without the protocol (GNOME's Mutter,
// notably) Gio falls back to a normal window and reports that in
// ConfigEvent.LayerShell, and the window keeps the raised-not-pinned
// behaviour it always had.
//
// X11 has an EWMH message for this that needs an X connection beside Gio's;
// not worth it until somebody on X11 asks.
package float

import (
	"os"

	"gioui.org/app"
	"gioui.org/unit"
)

// cascade counts the layer-shell windows this process has placed, so each
// takes the next stagger slot rather than the one before it.
var cascade int

// Spot is where a layer window sits: its distances from the top and left of
// the output, which is what a drag of its title bar changes.
type Spot struct {
	Top, Left unit.Dp
}

// NextSpot is where the next layer window goes: staggered down and right in
// steps of a row, so two opened together do not land exactly on one another.
func NextSpot() Spot {
	m := unit.Dp(24 + 32*(cascade%8))
	cascade++
	return Spot{Top: m, Left: m}
}

// Above returns the window options that ask for a window at spot to stay
// above the others. Pass them when the window is first made: none of the
// platforms can grant the ask later.
//
// keep is the Linux preference (ui.keep_above, on by default): false means
// the compositor's own windows, which can fall behind the main one.
// Everywhere else always-on-top is a property of a normal window that costs
// nothing, so the ask is unconditional and keep is ignored. Whether it was
// granted is still the platform's to say - on Linux, ConfigEvent's
// LayerShell field reports it.
func Above(spot Spot, keep bool) []app.Option { return above(spot, keep) }

// Move returns the option that places a layer window at spot: margins are
// what position one, because no compositor will drag it for the client.
func Move(spot Spot) app.Option {
	return app.LayerShellPlace(app.EdgeTop|app.EdgeLeft, spot.Top, 0, 0, spot.Left)
}

// Maximise returns the option that fills the output: anchored to all four
// edges with no margins, the layer-shell spelling of maximise.
func Maximise() app.Option {
	return app.LayerShellPlace(app.EdgeTop|app.EdgeBottom|app.EdgeLeft|app.EdgeRight,
		0, 0, 0, 0)
}

// onWayland reports whether the session looks like Wayland, which is the
// best a client can do before Gio connects: a stale WAYLAND_DISPLAY falls
// through to X11 inside Gio, and the layer-shell ask is simply ignored there.
func onWayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" }
