package nodeview

import "testing"

// A mode is found by the label on its switch, in any case, and a name that
// matches none is refused rather than quietly opening Client.
func TestCompanionModesAreFoundByTheirLabel(t *testing.T) {
	if m, ok := CompanionModeByName("cli"); !ok || m != modeCLI {
		t.Fatalf("cli resolved to %v, %v", m, ok)
	}
	if m, ok := CompanionModeByName(""); !ok || m != modeClient {
		t.Fatalf("an empty name resolved to %v, %v", m, ok)
	}
	if _, ok := CompanionModeByName("terminal"); ok {
		t.Fatal("a name no switch carries was accepted")
	}
}

// A tab built after the flag is set opens on that mode.
func TestATabOpensOnTheChosenMode(t *testing.T) {
	OpenOnCompanionMode = modeCLI
	defer func() { OpenOnCompanionMode = modeClient }()
	c := &companionTab{}
	c.build()
	if c.mode != modeCLI {
		t.Fatalf("the tab opened on %v", c.mode)
	}
}
