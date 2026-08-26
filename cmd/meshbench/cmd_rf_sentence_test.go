package main

import "testing"

// The board notes are full of version numbers and file extensions, and a full
// stop inside one is not the end of a sentence. Both cases below were printing
// truncated in `meshbench boards` before this was fixed.
func TestFirstSentenceKeepsVersionsAndExtensions(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"The board whose transmit failure 1.17.1 fixed: the pin was -1. And then more.",
			"The board whose transmit failure 1.17.1 fixed: the pin was -1."},
		{"the release publishes it as .uf2 like the others. Next sentence.",
			"the release publishes it as .uf2 like the others."},
		{"One sentence only, no full stop at all", "One sentence only, no full stop at all"},
		{"Ends with a stop and nothing after.", "Ends with a stop and nothing after."},
		{"First. Second.", "First."},
		{"An E22 on a devkit. Not a product.", "An E22 on a devkit."},
	} {
		if got := firstSentence(c.in); got != c.want {
			t.Errorf("firstSentence(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}
