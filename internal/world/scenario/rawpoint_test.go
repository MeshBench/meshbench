package scenario

import "testing"

// A tap on the picture has to become the point the panel would report.
//
// The T-Deck's firmware computes screen x from the panel's y and screen y
// backwards from the panel's x. So the round trip is the test: turning a
// screen point into a raw one and applying the firmware's own mapping has to
// give back what was tapped. Getting this wrong sends every touch somewhere
// else, which looks exactly like a touch layer nobody wired up.
func TestATapIsTurnedIntoThePanelsOwnAxes(t *testing.T) {
	const w, h = 320, 240
	panel := Part{Kind: Touch, Rotate: 90}
	for _, p := range [][2]int{{0, 0}, {319, 239}, {235, 210}, {160, 120}} {
		rx, ry := panel.RawPoint(p[0], p[1], w, h)
		// The firmware's mapping, from the board's own driver.
		sx, sy := ry, h-1-rx
		if sx != p[0] || sy != p[1] {
			t.Errorf("tap at %d,%d became raw %d,%d, which the firmware reads "+
				"as %d,%d", p[0], p[1], rx, ry, sx, sy)
		}
	}
}

// A panel mounted straight is left alone.
func TestAnUnrotatedPanelIsNotTurned(t *testing.T) {
	p := Part{Kind: Touch}
	if x, y := p.RawPoint(12, 34, 320, 240); x != 12 || y != 34 {
		t.Errorf("a panel with no rotation moved a tap to %d,%d", x, y)
	}
}
