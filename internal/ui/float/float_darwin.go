package float

import "gioui.org/app"

// macOS: always-on-top is a property of a normal window, and a floating-level
// one stops being topmost only with the app, which is what a tool window
// means here. The spot and the preference are meaningless - the compositor
// places the window and there is no price to decline.
func above(Spot, bool) []app.Option {
	return []app.Option{app.TopMost(true)}
}
