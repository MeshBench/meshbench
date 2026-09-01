package session

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// framingUI counts the two camera requests apart.
type framingUI struct {
	stubUI
	fits, opens int
}

func (u *framingUI) FitMap()       { u.fits++ }
func (u *framingUI) FitMapOnOpen() { u.opens++ }

// Opening a network puts it on screen.
//
// The camera used to stay where it was, so a fixture opened onto a camera
// pointed elsewhere was an empty map with a node count beside it - which is
// what somebody following the documentation reads as an open that did nothing.
// Asked for on the open rather than on the first play, because every step
// between the two would otherwise be taken against that same blank map.
func TestOpeningANetworkFramesIt(t *testing.T) {
	st := state.New(10)
	sm := &Sim{}
	Register(st, sm)
	ui := &framingUI{}
	sm.SetUI(ui)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "project.open",
		"../../../fixtures/fixture-fife-strict.json"); err != nil {
		t.Skip("no fixture:", err)
	}
	if ui.opens != 1 {
		t.Errorf("opening a network asked to frame it %d times", ui.opens)
	}
	// And it asks in the softer of the two ways: an outright fit overrules a
	// camera a capture flag placed on purpose, and a load must not.
	if ui.fits != 0 {
		t.Errorf("opening a network overruled the camera %d times", ui.fits)
	}
	// map.fit is still the outright one, for somebody who asks.
	if _, err := st.Do(ctx, "map.fit", nil); err != nil {
		t.Fatal(err)
	}
	if ui.fits != 1 {
		t.Errorf("map.fit reached the camera %d times", ui.fits)
	}
}
