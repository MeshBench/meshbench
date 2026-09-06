package fixture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// No shipped fixture carries a replacement character.
//
// U+FFFD is what a decoder leaves behind when it is handed bytes that are not
// valid UTF-8, so one in a name is not a character somebody chose: it is the
// mark of a name that was cut mid-character somewhere upstream and can never be
// recovered, because the bytes it stood for are gone. Ten of them shipped
// across four fixtures, and the first anybody knew was a node called
// "Drumcarrow Craig NWR" with two boxes after it.
//
// Checked over the whole file rather than over names alone: a schedule or a
// study area naming the same node has to carry the same string, and a fixture
// half-repaired is worse than one that is not.
func TestNoShippedFixtureCarriesAReplacementCharacter(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "fixtures", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found to check: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		// Both spellings, and they are genuinely different bytes. These
		// fixtures stored the escape - the six ASCII characters \ u f f f d -
		// so a check for the character alone reads the file as clean while the
		// name it names is broken. A fresh import would carry the character
		// itself, so both are refused.
		if i := strings.Index(string(b), `\ufffd`); i >= 0 {
			t.Errorf("%s holds an escaped U+FFFD at byte %d - a name cut "+
				"mid-character, not a character anybody typed",
				filepath.Base(p), i)
		}
		if strings.ContainsRune(string(b), '\uFFFD') {
			t.Errorf("%s holds a literal U+FFFD", filepath.Base(p))
		}
		if !utf8.Valid(b) {
			t.Errorf("%s is not valid UTF-8", filepath.Base(p))
		}
		// And it still parses, because the repair was a text edit.
		var any map[string]json.RawMessage
		if err := json.Unmarshal(b, &any); err != nil {
			t.Errorf("%s does not parse: %v", filepath.Base(p), err)
		}
	}
}

// No shipped fixture names the same study area twice.
//
// Three of them did, byte for byte: fixture-fem-e22, fixture-fife-permissive
// and fixture-fife-strict each carried "Fife" as two identical 11 kB entries.
// They are the hand-built ones - regenerate.mjs rebuilds only the national
// fixtures, and those were clean - and they predate boundary.accept learning to
// refuse a place already in the study area:
//
//	// Once each. Accepting the same place twice used to stack it, and a
//	// study area listed as "Fife, Fife, Fife" says nothing three times.
//
// Nothing then re-checked the files, so opening one put the duplicate back into
// the world past that guard: the boundary drawn twice, "2 areas" in the log,
// and a Boundary panel showing the same place on two rows with the same counts,
// which reads as a fault in the panel rather than in the data.
func TestNoShippedFixtureNamesAStudyAreaTwice(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "fixtures", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found to check: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		var f struct {
			Areas []struct {
				Name string `json:"name"`
			} `json:"areas"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatalf("%s does not parse: %v", p, err)
		}
		seen := map[string]bool{}
		for _, a := range f.Areas {
			k := strings.ToLower(a.Name)
			if seen[k] {
				t.Errorf("%s names the study area %q more than once: everything "+
					"that walks the areas walks it twice, and the Boundary panel "+
					"lists it on two rows with the same numbers",
					filepath.Base(p), a.Name)
			}
			seen[k] = true
		}
	}
}
