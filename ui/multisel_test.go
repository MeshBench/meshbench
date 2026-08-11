package ui

import "testing"

type flatT struct{}

func (flatT) ElevationM(_, _ float64) (float64, bool) { return 100, true }

// The multi-selection rules, tested for the same reason the link selection is:
// they are stateful and the drawing around them is not testable.
func TestMultiSelectSeedsAndToggles(t *testing.T) {
	a := New(flatT{})

	// Shift-click with a node already selected reads as "these two".
	a.SelectNode(0, false)
	a.toggleMulti(1)
	if len(a.msel) != 2 || a.msel[0] != 0 || a.msel[1] != 1 {
		t.Fatalf("msel = %v, want [0 1]", a.msel)
	}

	// Shift-click again removes.
	a.toggleMulti(1)
	if len(a.msel) != 1 || a.msel[0] != 0 {
		t.Fatalf("msel = %v after toggle-off, want [0]", a.msel)
	}

	// A plain click clears the multi-selection entirely.
	a.toggleMulti(2)
	a.SelectNode(1, false)
	if a.msel != nil {
		t.Fatalf("plain click left msel = %v", a.msel)
	}

	// Deleting a node renumbers everything, so the multi-selection must go.
	a.toggleMulti(0)
	a.toggleMulti(2)
	a.DeleteNode(0)
	if a.msel != nil {
		t.Fatalf("delete left msel = %v", a.msel)
	}
}
