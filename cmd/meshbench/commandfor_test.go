package main

import (
	"slices"
	"testing"
)

// The Windows installer's shortcut passes no arguments and neither does a
// double-clicked meshbench.exe, so what a bare argument vector means is the
// difference between the workbench opening and the application appearing to
// start and vanish. Held here rather than left to main.
func TestCommandFor(t *testing.T) {
	for _, c := range []struct {
		what string
		argv []string
		name string
		args []string
	}{
		{"the Start menu shortcut", []string{`C:\Program Files\MeshBench\meshbench.exe`},
			"workbench", nil},
		{"an empty vector", nil, "workbench", nil},
		{"a named command", []string{"meshbench", "link"}, "link", []string{}},
		{"a command with flags", []string{"meshbench", "test", "-fixture", "fife-strict"},
			"test", []string{"-fixture", "fife-strict"}},
		{"the version flag", []string{"meshbench", "-version"}, "-version", []string{}},
	} {
		name, args := commandFor(c.argv)
		if name != c.name || !slices.Equal(args, c.args) {
			t.Errorf("%s: commandFor(%q) = %q, %q; want %q, %q",
				c.what, c.argv, name, args, c.name, c.args)
		}
	}
}
