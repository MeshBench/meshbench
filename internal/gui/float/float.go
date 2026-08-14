// Package float keeps a window above the others, where the platform allows a
// client to ask for that at all.
//
// Gio can raise a window and has no always-on-top, for a reason: under
// Wayland no client may request it - the compositor decides, which is what a
// KWin window rule is for. So this is done per platform through the native
// handle Gio exposes in its ViewEvent, and Keep reports honestly whether it
// did anything:
//
//	macOS    NSWindow.level = NSFloatingWindowLevel
//	Windows  SetWindowPos(hwnd, HWND_TOPMOST, ...)
//	X11      not yet (an EWMH client message; needs an X connection)
//	Wayland  impossible from the client, documented rather than pretended
//
// The pop-out windows use it so a panel put beside the main window stays
// beside it, instead of vanishing behind it at the first click - which on a
// small screen is the difference between a second window being useful and
// being lost.
package float

import "gioui.org/app"

// Keep asks the platform to keep this window above the others, and reports
// whether the platform could. Call it on every ViewEvent: the native handle
// can change across the window's life, and re-asking is free.
func Keep(e app.ViewEvent) bool { return keep(e) }
