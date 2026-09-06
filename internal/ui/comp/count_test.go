package comp

import "testing"

// A count of one reads as one.
//
// Three places had spelled this "%d assertions" and all three said
// "1 assertions" on a fixture carrying one - the Schedule panel twice, and the
// last line of a passing `meshbench test`, which is the line CI logs and the
// one somebody pastes into a report.
func TestCountSaysOneThingOnce(t *testing.T) {
	for _, c := range []struct {
		n    int
		one  string
		want string
	}{
		{0, "assertion", "0 assertions"},
		{1, "assertion", "1 assertion"},
		{2, "assertion", "2 assertions"},
		{1, "send", "1 send"},
		{3, "send", "3 sends"},
	} {
		if got := Count(c.n, c.one); got != c.want {
			t.Errorf("Count(%d, %q) = %q, want %q", c.n, c.one, got, c.want)
		}
	}
}

// An irregular plural is the caller's to supply, rather than something this
// guesses at.
func TestCountOfTakesAPluralItCannotDerive(t *testing.T) {
	if got := CountOf(1, "entry", "entries"); got != "1 entry" {
		t.Errorf("got %q", got)
	}
	if got := CountOf(4, "entry", "entries"); got != "4 entries" {
		t.Errorf("got %q", got)
	}
}
