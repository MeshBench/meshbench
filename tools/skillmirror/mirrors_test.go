// The mapping, read against the tree in both directions, with no network.
//
// A check that reaches the published repositories to see whether they are
// behind can fail for two reasons that look identical on the way past: real
// drift, and GitHub being unreachable, rate limited or holding a token that has
// expired. This repository already has one network-dependent test and it
// flakes, which is exactly how a check stops being read.
//
// It is also unnecessary. The mirrors are rendered rather than copied, so there
// is no second version of a skill anywhere for the first to drift from; the
// remote falling behind is not a state anybody's change can create, it is a
// publish that did not run, and a workflow that failed is already loud. What a
// change here genuinely can break is the mapping - a skill added with nobody
// deciding which audience it belongs to, a mirror still naming a skill that has
// been deleted or renamed, front matter that no longer says what it is called -
// and every one of those is answerable from files on disk.
//
// A test rather than a CI step, for the reason internal/layoutmap_test.go gives
// for the same choice: a CI step fails for whoever pushes next, a test fails on
// the machine of whoever moved the skill, while they still remember why.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stamp stands in for the commit, so the test needs no git.
const stamp = "0000000000000000000000000000000000000000"

func TestEverySkillIsPublishedExactlyOnce(t *testing.T) {
	root := mustRepoRoot(t)

	published := map[string]string{}
	for _, m := range mirrors {
		for _, s := range m.skills {
			if was, dup := published[s.canonical]; dup {
				t.Errorf("%s is published by both %s and %s; one skill, one audience",
					s.canonical, was, m.repo)
			}
			published[s.canonical] = m.repo

			if _, err := os.Stat(filepath.Join(root, canonicalSkills, s.canonical, "SKILL.md")); err != nil {
				t.Errorf("%s publishes %s, which is not in %s any more: drop the row, or"+
					" name where the skill went", m.repo, s.canonical, canonicalSkills)
			}
		}
	}

	for _, name := range canonicalNames(t, root) {
		if _, ok := published[name]; ok {
			continue
		}
		if _, said := unpublished[name]; !said {
			t.Errorf("%s/%s is published by no mirror and is not in unpublished; add it to"+
				" a mirror in mirrors.go, or record there why it stays in the checkout only",
				canonicalSkills, name)
		}
	}

	// A reason for a skill that has gone is a reason nobody can check, and it
	// would let a deletion pass as a decision.
	for name := range unpublished {
		if _, err := os.Stat(filepath.Join(root, canonicalSkills, name, "SKILL.md")); err != nil {
			t.Errorf("unpublished names %s, which is not in %s any more; drop the row",
				name, canonicalSkills)
		}
		if repo, ok := published[name]; ok {
			t.Errorf("%s is both published by %s and listed as unpublished", name, repo)
		}
	}
}

func TestPublishedSkillsAnnounceTheDirectoryTheyAreInstalledUnder(t *testing.T) {
	for repo, files := range renderEverything(t) {
		for _, f := range files {
			if filepath.Base(f.path) != "SKILL.md" {
				continue
			}
			dir := filepath.Base(filepath.Dir(f.path))
			want := "name: " + dir
			if !strings.Contains(string(f.content), want) {
				t.Errorf("%s: %s does not carry %q; an agent invokes the directory, so a"+
					" front-matter name that says otherwise is a name nothing calls",
					repo, f.path, want)
			}
		}
	}
}

// The house rule, applied to what goes out under the project's name. The
// hand-copied mirrors carried em-dashes in prose written here without them,
// which is a small illustration of a copy going its own way.
func TestNothingPublishedCarriesAnEmDash(t *testing.T) {
	for repo, files := range renderEverything(t) {
		for _, f := range files {
			if f.path == "LICENSE" {
				continue
			}
			// Escaped rather than written out, so that a grep for the
			// character across the tree does not find its own detector.
			if strings.ContainsRune(string(f.content), '\u2014') {
				t.Errorf("%s: %s has an em-dash; comma, colon, full stop or spaced hyphen", repo, f.path)
			}
		}
	}
}

// The canonical README's table is what a person reads to find out which skills
// exist, and it is the one place the mapping is written twice. Reading it
// against the tree costs two lines and catches the fourth skill arriving.
func TestTheSkillsReadmeNamesEverySkill(t *testing.T) {
	root := mustRepoRoot(t)
	readme, err := os.ReadFile(filepath.Join(root, canonicalSkills, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range canonicalNames(t, root) {
		if !strings.Contains(string(readme), "`"+name+"`") {
			t.Errorf("%s/README.md never names %s", canonicalSkills, name)
		}
	}
}

// canonicalNames is every skill in the tree: a directory under .claude/skills
// holding a SKILL.md.
func canonicalNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, canonicalSkills))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, canonicalSkills, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		t.Fatalf("no skills under %s; this test would pass on an empty tree", canonicalSkills)
	}
	return names
}

func renderEverything(t *testing.T) map[string][]file {
	t.Helper()
	root := mustRepoRoot(t)
	out := map[string][]file{}
	for _, m := range mirrors {
		files, err := renderMirror(root, stamp, m)
		if err != nil {
			t.Fatalf("rendering %s: %v", m.repo, err)
		}
		out[m.repo] = files
	}
	return out
}

func mustRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}
