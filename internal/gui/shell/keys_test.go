package shell

import (
	"testing"

	"gioui.org/io/key"
)

// A binding on two actions is a mistake somebody has to fix, and an interface
// that quietly picks one is an interface where a key does something other than
// what the sheet says.
func TestAConflictIsReported(t *testing.T) {
	sh := New()
	got := sh.SetShortcuts([]Shortcut{
		{Name: "S", Mods: key.ModCtrl, Action: "run.save"},
		{Name: "S", Mods: key.ModCtrl, Action: "project.save"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d conflicts, want 1: %v", len(got), got)
	}
}

// The same key with different modifiers is not a conflict.
func TestModifiersDistinguishBindings(t *testing.T) {
	sh := New()
	if got := sh.SetShortcuts([]Shortcut{
		{Name: "S", Mods: key.ModCtrl, Action: "run.save"},
		{Name: "S", Mods: key.ModCtrl | key.ModShift, Action: "run.save_as"},
	}); len(got) != 0 {
		t.Fatalf("reported a conflict that is not one: %v", got)
	}
}

// The menu's label comes from the registry, so the two cannot disagree. This
// is the property the whole arrangement exists for.
func TestTheMenuLabelComesFromTheRegistry(t *testing.T) {
	sh := New()
	sh.SetShortcuts([]Shortcut{
		{Name: "S", Mods: key.ModCtrl, Action: "run.save"},
	})
	if got := sh.shortcutFor("run.save"); got != "ctrl s" {
		t.Fatalf("menu would show %q", got)
	}
	if got := sh.shortcutFor("nothing.bound"); got != "" {
		t.Fatalf("an unbound action showed %q rather than nothing", got)
	}
}

// Space is written as a word. A menu with an invisible shortcut is a menu that
// looks like it has none.
func TestSpaceIsSpelled(t *testing.T) {
	if got := (Shortcut{Name: key.NameSpace}).String(); got != "space" {
		t.Fatalf("got %q", got)
	}
}
