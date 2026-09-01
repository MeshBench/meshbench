package main

import (
	"strings"
	"testing"
)

// Every command carries a worked example, because a flag list on its own
// documents the vocabulary and not the language. The reference is generated,
// so nothing else would notice a command that arrived without one.
func TestEveryCommandHasAnExample(t *testing.T) {
	for _, c := range commands() {
		if len(c.examples) == 0 {
			t.Errorf("%s has no example", c.name)
			continue
		}
		for _, ex := range c.examples {
			if !strings.HasPrefix(ex.Line, "meshbench "+c.name+" ") &&
				ex.Line != "meshbench "+c.name {
				t.Errorf("%s: example does not invoke it: %q", c.name, ex.Line)
			}
			if ex.Why == "" {
				t.Errorf("%s: example %q says nothing about what it shows", c.name, ex.Line)
			}
		}
	}
}

// The index is what the usage screen prints and what the reference is built
// from, so a command with no summary is invisible in both.
func TestEveryCommandIsDescribed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands() {
		if c.summary == "" {
			t.Errorf("%s has no summary", c.name)
		}
		if c.run == nil {
			t.Errorf("%s has nothing to run", c.name)
		}
		if seen[c.name] {
			t.Errorf("%s is listed twice", c.name)
		}
		seen[c.name] = true
	}
}
