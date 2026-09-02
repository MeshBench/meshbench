package session

import (
	"encoding/json"
	"strings"
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

// A bare number fills the primary field and no other.
//
// numField answers with the bare parameter whatever field it is asked for,
// which is right for the one field that parameter means and wrong for every
// other: numField(5, "lat") and numField(5, "lon") both answered 5, and
// nodes.place read both through it, so a bare 5 placed a node at 5N 5E and
// reported the position as the one it had been given.
func TestABareNumberFillsOnlyOneField(t *testing.T) {
	if got, ok := numField(5, "lat"); !ok || got != 5 {
		t.Errorf("the primary field of a bare number is %v/%v, want 5", got, ok)
	}
	if got, ok := namedNum(5, "lon"); ok || got != 0 {
		t.Errorf("a second field of a bare number is %v/%v, want absent", got, ok)
	}
	if got, ok := namedNum(map[string]any{"lon": -3.4}, "lon"); !ok || got != -3.4 {
		t.Errorf("a named field is %v/%v, want -3.4", got, ok)
	}
}

// A bare value a number-taking verb cannot read is a refusal, not a default.
//
// The collapse params.go exists to prevent, in the one shape it still had. A
// bare "lots" handed to coverage.resolution read as nobody having asked, so the
// verb answered with the resolution it had not changed and the caller was told
// their setting had been applied.
func TestAnUnreadableBareNumberIsRefused(t *testing.T) {
	if _, asked, err := numAsked("coverage.resolution", "cells", "lots"); err == nil {
		t.Errorf("a bare string read as asked=%v with no refusal", asked)
	} else if !strings.Contains(err.Error(), "cells") {
		t.Errorf("refused with %v, which does not name the parameter", err)
	}
	if _, asked, err := numAsked("coverage.resolution", "cells", nil); err != nil || asked {
		t.Errorf("no parameter at all read as %v/%v, want absent and no error", asked, err)
	}
	// And a bare value meant for some other field stays absence rather than
	// becoming a refusal: validate.fetch takes a bare URL and an optional
	// hours, and reading the URL as the hours would refuse a well formed call.
	if _, asked, err := namedNumAsked("validate.fetch", "hours",
		"https://example.invalid/feed"); err != nil || asked {
		t.Errorf("a bare primary read as hours=%v/%v, want absent and no error", asked, err)
	}
}
