package nodeview

import "testing"

// A window already open must switch to the tab it is asked for.
//
// It used to keep whatever tab it was on and answer with the tab that had been
// requested, so a caller naming one was told it had what it asked for and
// nothing moved. Every panel and section here is reachable by verb precisely
// so a capture can reach it.
func TestARecalledWindowIsAskedForTheTab(t *testing.T) {
	w := NewWindowSet()
	if _, ok := w.takeTab("Abernethy Repeater"); ok {
		t.Fatal("a wish existed before anything asked for one")
	}

	w.askTab("Abernethy Repeater", TabRadio)
	got, ok := w.takeTab("Abernethy Repeater")
	if !ok {
		t.Fatal("the wish was not left for the window")
	}
	if got != TabRadio {
		t.Errorf("the window was asked for tab %v, want %v", got, TabRadio)
	}

	// Once, not every frame: a person who clicks another tab afterwards keeps
	// it rather than being dragged back on the next draw.
	if _, ok := w.takeTab("Abernethy Repeater"); ok {
		t.Error("the wish was still there after the window took it")
	}
}

// One node's wish is not another's.
func TestTabWishesArePerNode(t *testing.T) {
	w := NewWindowSet()
	w.askTab("one", TabStats)
	w.askTab("two", TabAntenna)

	if got, _ := w.takeTab("one"); got != TabStats {
		t.Errorf("node one was asked for %v, want %v", got, TabStats)
	}
	if got, _ := w.takeTab("two"); got != TabAntenna {
		t.Errorf("node two was asked for %v, want %v", got, TabAntenna)
	}
}
