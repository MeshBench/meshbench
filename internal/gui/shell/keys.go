package shell

import (
	"sort"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
)

// The shortcut registry (11.2).
//
// One list, and everything else is generated from it: the menu labels, the
// sheet, and the dispatch. A menu that spells its own shortcut drifts from the
// binding, which is the usual way a shortcut sheet ends up lying about the
// application it documents.

// Shortcut is one binding.
type Shortcut struct {
	// Name is the Gio key name: a letter, or one of its named keys.
	Name key.Name
	// Mods that must be held. Zero means none.
	Mods key.Modifiers
	// Action is the same string a menu entry carries, so a key and a menu
	// entry are two routes to one thing rather than two things.
	Action string
	// What it does, in words, for the sheet.
	What string
}

// String is how a binding is written down, for a menu and for the sheet.
func (s Shortcut) String() string {
	var parts []string
	if s.Mods.Contain(key.ModCtrl) {
		parts = append(parts, "ctrl")
	}
	if s.Mods.Contain(key.ModShift) {
		parts = append(parts, "shift")
	}
	if s.Mods.Contain(key.ModAlt) {
		parts = append(parts, "alt")
	}
	name := string(s.Name)
	if name == " " {
		name = "space"
	}
	return strings.Join(append(parts, strings.ToLower(name)), " ")
}

// SetShortcuts registers the bindings, and reports any conflict rather than
// letting the last one silently win.
//
// A conflict is returned rather than logged because two actions on one key is
// a mistake somebody has to fix, and an interface that quietly picks one is an
// interface where a key does something different from what the sheet says.
func (sh *Shell) SetShortcuts(list []Shortcut) []string {
	seen := map[string]string{}
	var clashes []string
	for _, s := range list {
		k := s.String()
		if prev, ok := seen[k]; ok {
			clashes = append(clashes, k+" is bound to both "+prev+" and "+s.Action)
			continue
		}
		seen[k] = s.Action
	}
	sh.shortcuts = list
	sort.Strings(clashes)
	return clashes
}

// Shortcuts is the registry, for a sheet that cannot disagree with it.
func (sh *Shell) Shortcuts() []Shortcut { return sh.shortcuts }

// shortcutFor is the written form of whatever is bound to an action, or "".
func (sh *Shell) shortcutFor(action string) string {
	for _, s := range sh.shortcuts {
		if s.Action == action {
			return s.String()
		}
	}
	return ""
}

// keys handles the frame's key events and dispatches by action.
//
// Registered on the shell itself as the focus target, so a key works wherever
// the pointer is: a shortcut that only fires while the map has focus is a
// shortcut somebody has to hunt for.
func (sh *Shell) keys(gtx layout.Context) {
	if len(sh.shortcuts) == 0 {
		return
	}
	event.Op(gtx.Ops, sh)
	filters := make([]event.Filter, 0, len(sh.shortcuts)+1)
	filters = append(filters, key.FocusFilter{Target: sh})
	for _, s := range sh.shortcuts {
		filters = append(filters, key.Filter{
			Focus: sh, Name: s.Name, Required: s.Mods,
		})
	}
	for {
		ev, ok := gtx.Event(filters...)
		if !ok {
			return
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		for _, s := range sh.shortcuts {
			if s.Name == ke.Name && s.Mods == ke.Modifiers && sh.OnMenu != nil {
				sh.OnMenu(s.Action)
				break
			}
		}
	}
}
