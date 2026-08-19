//go:build !windows && !darwin

package float

import "gioui.org/app"

// Linux: under Wayland a client cannot ask - the compositor decides, and a
// KWin window rule ("keep above others" for io.github.meshbench.meshbench)
// is the supported answer. Under X11 an EWMH message could do it, but that
// needs its own X connection beside Gio's; not worth it until somebody on
// X11 asks.
func keep(app.ViewEvent) bool { return false }
