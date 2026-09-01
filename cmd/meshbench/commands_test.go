package main

import (
	"strings"
	"testing"
)

// TestEveryCommandHasARunExample holds the half of the CLI reference that
// cannot be generated. The flag tables come out of the binary's own help, so a
// command with no example documents its flags and never shows the shape of a
// real invocation, which is the thing somebody arriving cold actually needs.
func TestEveryCommandHasARunExample(t *testing.T) {
	for _, c := range commands() {
		if len(c.examples) == 0 {
			t.Errorf("%s has no example", c.name)
			continue
		}
		for i, ex := range c.examples {
			if strings.TrimSpace(ex.note) == "" {
				t.Errorf("%s example %d says nothing about what it showed", c.name, i)
			}
			// The invocation is the last line: an example may set its input up
			// first, as traffic writes a scenario file before running on it.
			lines := strings.Split(strings.TrimSpace(ex.shell), "\n")
			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, "meshbench "+c.name) {
				t.Errorf("%s example %d does not end by running that command: %q",
					c.name, i, last)
			}
		}
	}
}

// TestCommandNamesAreUniqueAndSummarised guards the dispatch in main, which
// takes the first command whose name matches: a duplicate would shadow the one
// below it with nothing to say so.
func TestCommandNamesAreUniqueAndSummarised(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands() {
		if seen[c.name] {
			t.Errorf("two commands are called %q", c.name)
		}
		seen[c.name] = true
		if c.summary == "" || c.run == nil {
			t.Errorf("%s has no summary or no implementation", c.name)
		}
	}
}
