package nodeview

import "testing"

// A window answers with the tab it settles on, not the tab it was handed.
//
// node.window used to report the request back: asked for Hardware on a node
// whose board declares nothing, it answered "Hardware" while the window drew
// its console. The reference told a caller the field was the tab the window
// opened on, so a driven capture recorded a console picture as Hardware and
// the manifest could not catch it - a picture was taken either way.
func TestSettleTabAnswersWhatTheWindowWillShow(t *testing.T) {
	repeater := []Tab{TabConsole, TabSettings, TabRadio, TabAntenna,
		TabStats, TabActivity, TabOutput}
	withBoard := append(append([]Tab{}, repeater...), TabHardware)
	companion := []Tab{TabCompanion, TabSettings, TabRadio, TabAntenna,
		TabStats, TabActivity, TabConnect}
	observer := []Tab{TabSDR, TabSettings, TabAntenna, TabStats, TabActivity}

	for _, c := range []struct {
		what string
		tabs []Tab
		ask  Tab
		want Tab
	}{
		{"a tab the node has is kept", repeater, TabStats, TabStats},
		{"Hardware on a board that declares nothing", repeater, TabHardware, TabConsole},
		{"Hardware on a board that declares something", withBoard, TabHardware, TabHardware},
		{"a console on a companion", companion, TabConsole, TabCompanion},
		{"a console on an observer", observer, TabConsole, TabSDR},
		{"Radio on an observer", observer, TabRadio, TabSDR},
	} {
		if got := settleTab(c.ask, c.tabs); got != c.want {
			t.Errorf("%s: asked for %v, wanted %v, got %v",
				c.what, c.ask, c.want, got)
		}
	}
}

// An empty set is the caller's own bug, and swallowing it would report a tab
// that is not merely wrong but impossible.
func TestSettleTabWithNoTabsReturnsWhatWasAsked(t *testing.T) {
	if got := settleTab(TabStats, nil); got != TabStats {
		t.Errorf("wanted the request back when there is nothing to settle on, got %v", got)
	}
}
