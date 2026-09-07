package basemap

import "testing"

// The verb that reports the basemap and the window that draws it must agree.
// They did not: map.basemap answered the stored preference, which is empty
// until somebody chooses, so every fresh profile reported "" for a window
// visibly drawing carto-dark.
func TestTheDefaultFollowsTheKey(t *testing.T) {
	t.Setenv("MESHBENCH_CARTO_KEY", "a-key")
	if got := DefaultID(); got != "carto-dark" {
		t.Errorf("with a key the default is %q, want carto-dark", got)
	}
	t.Setenv("MESHBENCH_CARTO_KEY", "")
	// A build stamped with a key by the release pipeline still has one, so
	// the keyless answer is only checked where there is genuinely no key.
	if defaultCartoKey == "" {
		if got := DefaultID(); got != "osm" {
			t.Errorf("with no key the default is %q, want osm", got)
		}
	}
	if DefaultID() == "" {
		t.Error("the default is empty, which names no map at all")
	}
}
