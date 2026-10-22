package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The lint gate must distinguish findings from a tool that never completed.
// Run the real script in a tiny fixture with a fake linter: no downloads,
// configuration dependencies, or changes to the repository's own baseline.
func TestLintRatchetFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the CI lint ratchet is a Bash script")
	}
	script, err := os.ReadFile("../tools/lint-ratchet.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, output, status, want string
		update, pass               bool
		baseline                   string
	}{
		{"holds", "2 issues:\n* gosec: 2\n", "1", "lint ratchet holds", false, true, ""},
		{"rises", "3 issues:\n* gosec: 3\n", "1", "RISEN", false, false, ""},
		{"falls", "1 issues:\n* gosec: 1\n", "1", "fallen", false, true, ""},
		{"clears", "0 issues.\n", "0", "cleared", false, true, ""},
		{"tool error", "configuration error\n", "3", "lint did not complete", false, false, ""},
		{"crash with summary", "0 issues.\n", "2", "lint did not complete", false, false, ""},
		{"no output", "", "0", "unrecognised lint output", false, false, ""},
		{"error without counts", "analysis failed\n", "1", "unrecognised lint output", false, false, ""},
		{"incomplete counts", "3 issues:\n* gosec: 2\n", "1", "inconsistent lint summary", false, false, ""},
		{"zero with findings", "2 issues:\n* gosec: 2\n", "0", "inconsistent lint summary", false, false, ""},
		{"one without findings", "0 issues.\n", "1", "inconsistent lint summary", false, false, ""},
		{"update after failure", "", "2", "lint did not complete", true, false, ""},
		{"missing baseline", "0 issues.\n", "0", "invalid or missing lint baseline", false, false, "<missing>"},
		{"invalid baseline", "0 issues.\n", "0", "invalid or missing lint baseline", false, false, "gosec nope\n"},
		{"duplicate baseline", "0 issues.\n", "0", "invalid or missing lint baseline", false, false, "gosec 2\ngosec 3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tools := filepath.Join(root, "tools")
			if err := os.Mkdir(tools, 0o755); err != nil {
				t.Fatal(err)
			}
			write := func(path string, data []byte, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			gate := filepath.Join(tools, "lint-ratchet.sh")
			write(gate, script, 0o755)
			baseline := filepath.Join(root, ".golangci-baseline.txt")
			if tc.baseline != "<missing>" {
				base := tc.baseline
				if base == "" {
					base = "gosec 2\n"
				}
				write(baseline, []byte(base), 0o644)
			}
			linter := filepath.Join(root, "linter")
			write(linter, []byte("#!/usr/bin/env bash\ncat \"$LINT_FIXTURE\"\nexit \"$LINT_STATUS\"\n"), 0o755)
			fixture := filepath.Join(root, "output")
			write(fixture, []byte(tc.output), 0o644)
			args := []string{gate}
			if tc.update {
				args = append(args, "--update")
			}
			cmd := exec.Command("bash", args...)
			cmd.Env = append(os.Environ(), "GOLANGCI_LINT="+linter,
				"LINT_FIXTURE="+fixture, "LINT_STATUS="+tc.status)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.pass || !strings.Contains(string(out), tc.want) {
				t.Fatalf("exit %v, output %q; want pass=%v and %q", err, out, tc.pass, tc.want)
			}
			if tc.update && !tc.pass {
				got, err := os.ReadFile(baseline)
				if err != nil || string(got) != "gosec 2\n" {
					t.Fatalf("failed lint must leave the baseline intact: %q, %v", got, err)
				}
			}
		})
	}
}
