package session

import (
	"encoding/json"
	"testing"
)

// Does a verb take a number from the interface, not just from a script?
//
// It did not, and the way it failed was invisible: the control socket arrives
// as JSON, where every number is a float, so everything scripted worked. The
// interface calls the same verbs in process and passes an int, like any Go
// caller would - and the map only understood floats, so it answered "needs a
// point" to a call that had one. Tapping a drawn screen and pressing a drawn
// button both did nothing, with no way to tell that from not being wired.
func TestAVerbTakesANumberFromAnyCaller(t *testing.T) {
	for _, c := range []struct {
		name string
		v    any
	}{
		{"int, as the interface passes", 235},
		{"int64", int64(235)},
		{"int32", int32(235)},
		{"uint8", uint8(235)},
		{"float64, as JSON decodes to", float64(235)},
		{"float32", float32(235)},
		{"json.Number", json.Number("235")},
	} {
		got, ok := numField(map[string]any{"x": c.v}, "x")
		if !ok || got != 235 {
			t.Errorf("%s: read back %v, ok=%v - want 235", c.name, got, ok)
		}
	}
}

// And it still refuses what is genuinely not a number, rather than reading it
// as zero: a verb that treats "over there" as the origin is worse than one
// that says it cannot.
func TestAVerbRefusesSomethingThatIsNotANumber(t *testing.T) {
	for _, v := range []any{nil, "235", true, []int{235}} {
		if _, ok := numField(map[string]any{"x": v}, "x"); ok {
			t.Errorf("%#v was accepted as a number", v)
		}
	}
}
