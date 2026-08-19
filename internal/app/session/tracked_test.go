package session

import (
	"os/exec"
	"strings"
	"testing"
)

// Every source file is in the repository.
//
// A bare "workbench2" line in .gitignore, added to keep an accidentally
// committed binary out, also matched the directory cmd/workbench2 - so three
// files of a new panel were written, built, run, screenshotted and discussed
// while git had never heard of them. Nothing complained: the build reads the
// working tree, and git status hides what it is ignoring.
//
// This is the check that would have caught it on the first commit.
func TestNoSourceFileIsIgnored(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "--others", "--ignored",
		"--exclude-standard", "../../../cmd", "../../../internal").Output()
	if err != nil {
		t.Skip("not a git checkout")
	}
	var bad []string
	for _, f := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasSuffix(f, ".go") {
			bad = append(bad, f)
		}
	}
	if len(bad) > 0 {
		t.Errorf("these Go files are ignored by git and would be lost:\n  %s",
			strings.Join(bad, "\n  "))
	}
}
