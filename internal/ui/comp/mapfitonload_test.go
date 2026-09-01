package comp

import "testing"

// A network that has just been loaded is framed, and a camera somebody aimed
// on purpose is not.
//
// The two requests are different questions. Opening a network onto a camera
// pointed elsewhere is a blank map with a node count beside it, so a load asks
// for a fit; but -look exists so a capture can be taken of one particular
// place, and a fixture finishing its load a few seconds later is no reason to
// overrule it.
func TestALoadFramesAnUnaimedCameraAndLeavesAnAimedOne(t *testing.T) {
	m := &MapView{Zoom: 1000, CentreLat: 56, CentreLon: -3, initialised: true}
	m.FitLoaded = true
	if !m.wantsFit() {
		t.Error("a network arriving does not frame a camera nobody has aimed")
	}

	m.StartAt(56.3, -3.3, 4000)
	m.FitLoaded = true
	if m.wantsFit() {
		t.Error("a network arriving overruled a camera placed on purpose")
	}

	// Asking outright still works, whatever the camera was doing: that is the
	// map menu's fit and the map.fit verb, and both are somebody asking.
	m.FitNext = true
	if !m.wantsFit() {
		t.Error("an outright fit was refused")
	}
}
