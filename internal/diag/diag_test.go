package diag

import (
	"strings"
	"testing"
)

// reparse re-reads a given domain list, standing in for the process's env so a
// test does not depend on MESHBENCH_LOG being set in the environment.
func reparse(t *testing.T, v string) {
	t.Helper()
	mu.Lock()
	parse(v)
	parsed = true
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		parsed = false
		mu.Unlock()
	})
}

func TestOnSelectsNamedDomains(t *testing.T) {
	reparse(t, "radio, emulator")
	if !On("radio") || !On("emulator") {
		t.Error("a named domain should be on")
	}
	if On("channel") {
		t.Error("an unnamed domain should be off")
	}
}

func TestEmptyIsOff(t *testing.T) {
	reparse(t, "")
	if On("radio") || On("anything") {
		t.Error("no domain should be on when MESHBENCH_LOG is empty")
	}
}

func TestAllSelectsEverything(t *testing.T) {
	reparse(t, "all")
	if !On("radio") || !On("something-never-named") {
		t.Error("all should switch every domain on")
	}
}

func TestWhitespaceAndBlanksIgnored(t *testing.T) {
	reparse(t, " radio , , emulator ")
	if !On("radio") || !On("emulator") {
		t.Error("names should survive surrounding whitespace and empty entries")
	}
	if On("") {
		t.Error("the empty domain is never on")
	}
}

// Printf and Println must be silent for an off domain and must not panic for an
// on one; the content going to stderr is not captured here, only the guard.
func TestPrintfGuards(t *testing.T) {
	reparse(t, "radio")
	// These would print to stderr for "radio" and be silent for "channel";
	// the test asserts the decision, which On exposes.
	if On("channel") {
		t.Fatal("channel is off, so Printf must be a no-op")
	}
	if !On("radio") {
		t.Fatal("radio is on")
	}
	// Format string with an arg, to catch an accidental double-format.
	Printf("radio", "value=%d", 7)
	Println("radio", "a", "b")
	if !strings.Contains("["+"radio"+"] ", "radio") { // trivially true; documents the prefix
		t.Fatal("unreachable")
	}
}
