package float

import "gioui.org/app"

// Windows: HWND_TOPMOST, asked for when the window is made. The spot and the
// preference are meaningless - the compositor places the window and there is
// no price to decline.
func above(Spot, bool) []app.Option {
	return []app.Option{app.TopMost(true)}
}
