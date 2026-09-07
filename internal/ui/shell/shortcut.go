// A menu shortcut: the one neutral spelling the table holds, what it binds,
// and how the platform spells it back.
//
// Its own file because menu.go had reached the length limit, and because the
// binding and the caption are one subject: they have to agree, and only one
// of them can be the source.
package shell

import (
	"runtime"
	"strings"

	"gioui.org/io/key"
)

// parseShortcut reads the neutral caption: "Ctrl+O", "Ctrl+Shift+S", "Space".
//
// "Ctrl+" in the table means the platform's shortcut modifier, not the control
// key: key.ModShortcut is Command on macOS and Ctrl everywhere else, and Gio
// carries the constant for exactly this. It used to bind key.ModCtrl outright,
// and modifiers are matched exactly rather than by containment - so on a Mac
// Ctrl+S saved and Command+S did nothing, which is the opposite of what every
// application on that platform does. Command+Q especially: it is muscle memory
// for every Mac user alive.
func parseShortcut(s string) (key.Name, key.Modifiers, bool) {
	var mods key.Modifiers
	rest := s
	for {
		switch {
		case cutPrefix(&rest, "Ctrl+"):
			mods |= key.ModShortcut
		case cutPrefix(&rest, "Shift+"):
			mods |= key.ModShift
		case cutPrefix(&rest, "Alt+"):
			mods |= key.ModAlt
		default:
			if rest == "Space" {
				return key.NameSpace, mods, true
			}
			if len(rest) == 1 {
				return key.Name(rest), mods, true
			}
			return "", 0, false
		}
	}
}

// shortcutCaption spells a shortcut the way the platform does.
//
// The table holds one neutral spelling and this renders it, because the
// binding and the caption have to agree and only one of them can be the
// source. On macOS the modifier is Command, drawn as the symbol every menu
// there uses; everywhere else it is Ctrl and the caption is unchanged.
func shortcutCaption(s string) string {
	if runtime.GOOS != "darwin" {
		return s
	}
	// Parsed into a set and emitted in the platform's own order, not
	// substituted in place: substitution kept the table's order, so
	// Ctrl+Shift+S came out as command-shift-S, and a Mac menu writes its
	// modifiers control, option, shift, command - shift before command, always.
	var alt, shift, cmd bool
	rest := s
	for {
		switch {
		case cutPrefix(&rest, "Ctrl+"):
			cmd = true
		case cutPrefix(&rest, "Shift+"):
			shift = true
		case cutPrefix(&rest, "Alt+"):
			alt = true
		default:
			var b strings.Builder
			if alt {
				b.WriteString("\u2325")
			}
			if shift {
				b.WriteString("\u21e7")
			}
			if cmd {
				b.WriteString("\u2318")
			}
			b.WriteString(rest)
			return b.String()
		}
	}
}
