package shell

import (
	"runtime"
	"strings"
	"testing"

	"gioui.org/io/key"
)

// "Ctrl+" in the menu table means the platform's shortcut modifier, not the
// control key. It bound key.ModCtrl outright, and modifiers are matched
// exactly rather than by containment - so on a Mac Ctrl+S saved and Command+S
// did nothing, the opposite of what every application on that platform does.
func TestAShortcutBindsThePlatformModifier(t *testing.T) {
	name, mods, ok := parseShortcut("Ctrl+S")
	if !ok {
		t.Fatal("Ctrl+S did not parse")
	}
	if name != "S" {
		t.Errorf("the key is %q, want S", name)
	}
	if mods&key.ModShortcut == 0 {
		t.Errorf("Ctrl+S binds %v, which does not include the platform shortcut", mods)
	}

	// The other modifiers still mean themselves.
	_, mods, ok = parseShortcut("Ctrl+Shift+S")
	if !ok || mods&key.ModShift == 0 {
		t.Errorf("Ctrl+Shift+S binds %v, want the shift modifier too", mods)
	}
}

// The caption and the binding have to agree, and the table holds one neutral
// spelling that this renders.
func TestTheCaptionMatchesThePlatform(t *testing.T) {
	got := shortcutCaption("Ctrl+Shift+S")
	if runtime.GOOS == "darwin" {
		if strings.Contains(got, "Ctrl") {
			t.Errorf("a Mac menu offers %q, which no Mac application does", got)
		}
		if !strings.Contains(got, "⌘") {
			t.Errorf("a Mac caption is %q, want the command symbol", got)
		}
		// Shift before command: a Mac menu writes its modifiers in the order
		// control, option, shift, command, and a caption in the table's own
		// order reads as a menu that was not made for the platform.
		if got != "⇧⌘S" {
			t.Errorf("a Mac caption is %q, want ⇧⌘S", got)
		}
		return
	}
	if got != "Ctrl+Shift+S" {
		t.Errorf("off macOS the caption is %q, want it unchanged", got)
	}
}
