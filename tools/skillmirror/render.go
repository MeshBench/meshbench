package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// frontPages are the one thing a mirror has that the canonical tree has no
// place for: a front page written for somebody who arrived at a bare
// repository with no MeshBench checkout and has to be told what a skill is and
// where to put it. The skill table and the provenance line are filled in from
// the mapping, so a mirror's README cannot come to disagree with what the
// mirror actually carries.
//
//go:embed readme/*.md
var frontPages embed.FS

// canonicalSkills is where the source of truth lives, relative to the root.
const canonicalSkills = ".claude/skills"

// file is one path inside a rendered mirror and its bytes.
type file struct {
	path    string
	content []byte
}

// renderMirror produces the whole tree of one published repository.
func renderMirror(root, sha string, m mirror) ([]file, error) {
	readme, err := renderFrontPage(sha, m)
	if err != nil {
		return nil, err
	}
	files := []file{readme}

	// The mirrors carry the project's licence rather than pointing at it: a
	// repository someone is invited to clone and copy out of should say what
	// they may do with it without a second clone.
	licenceFile, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		return nil, fmt.Errorf("%s: reading the licence to publish with it: %w", m.repo, err)
	}
	files = append(files, file{path: "LICENSE", content: licenceFile})

	manifests, err := renderPlugin(sha, m)
	if err != nil {
		return nil, err
	}
	files = append(files, manifests...)

	for _, s := range m.skills {
		f, err := renderSkill(root, sha, s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.repo, err)
		}
		files = append(files, f)
	}
	return files, nil
}

// renderSkill is the canonical SKILL.md as the mirror installs it: under the
// directory that mirror uses, announcing that directory, and saying where it
// came from.
func renderSkill(root, sha string, s installed) (file, error) {
	src, err := os.ReadFile(filepath.Join(root, canonicalSkills, s.canonical, "SKILL.md"))
	if err != nil {
		return file{}, err
	}
	body, err := rewriteFrontMatterName(src, s.canonical, s.name())
	if err != nil {
		return file{}, err
	}
	body = append(body, []byte(provenance(sha, s.canonical))...)
	return file{path: filepath.Join("skills", s.name(), "SKILL.md"), content: body}, nil
}

// rewriteFrontMatterName makes the published copy announce the directory it is
// installed under.
//
// An agent invokes a skill by its directory and matches on its front matter,
// so the two disagreeing gives a skill that either does not load or loads
// under a name nothing calls. That is not hypothetical: the published
// meshbench-driving introduced itself as "meshbench" for as long as it was
// hand-copied.
//
// It refuses when the canonical front matter does not already name its own
// directory, because at that point the rename this is performing is guesswork.
func rewriteFrontMatterName(src []byte, canonical, want string) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("%s/SKILL.md opens with no front matter", canonical)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			break
		}
		rest, ok := strings.CutPrefix(lines[i], "name:")
		if !ok {
			continue
		}
		if got := strings.TrimSpace(rest); got != canonical {
			return nil, fmt.Errorf("%s/SKILL.md says name: %s; the front matter has to name its own"+
				" directory before a mirror can rename it to %s", canonical, got, want)
		}
		lines[i] = "name: " + want
		return []byte(strings.Join(lines, "\n")), nil
	}
	return nil, fmt.Errorf("%s/SKILL.md has front matter carrying no name", canonical)
}

// provenance travels inside the skill rather than only in the README, because
// the README is not what gets installed: a person copies skills/<name> into
// their agent's directory and the front page stays behind. A comment is
// invisible where the file is rendered and is not read as instruction where it
// is loaded.
func provenance(sha, canonical string) string {
	return fmt.Sprintf(`
<!--
Published from MeshBench %s by tools/skillmirror.

The source of truth is %s/%s/SKILL.md in
https://github.com/MeshBench/meshbench, where a skill is corrected in the same
commit as the change that made it wrong. An edit made here is overwritten by
the next publish, so send it there.
-->
`, sha, canonicalSkills, canonical)
}

// frontPageData is what a mirror's README template is given.
type frontPageData struct {
	// Table is the body of the skill table, one row per skill it carries.
	Table string
	// Commit is the MeshBench commit this publish was rendered from.
	Commit string
	// Repo is owner/name, which is what a reader types at
	// `/plugin marketplace add`, and RepoName is the directory a clone of it
	// lands in.
	Repo     string
	RepoName string
	// Plugin and Marketplace are the two halves of the install command, taken
	// from the same mapping the manifests are written from so the front page
	// cannot tell somebody to type a name that is not in them.
	Plugin      string
	Marketplace string
}

func renderFrontPage(sha string, m mirror) (file, error) {
	src, err := frontPages.ReadFile("readme/" + m.repo + ".md")
	if err != nil {
		return file{}, fmt.Errorf("%s has no front page under tools/skillmirror/readme: %w", m.repo, err)
	}
	tmpl, err := template.New(m.repo).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return file{}, err
	}

	rows := make([]string, 0, len(m.skills))
	for _, s := range m.skills {
		if s.blurb == "" {
			return file{}, fmt.Errorf("%s: %s has no blurb, so its row in the README would be blank",
				m.repo, s.name())
		}
		rows = append(rows, fmt.Sprintf("| `%s` | %s |", s.name(), s.blurb))
	}

	var out bytes.Buffer
	data := frontPageData{
		Table:       strings.Join(rows, "\n"),
		Commit:      sha,
		Repo:        org + "/" + m.repo,
		RepoName:    m.repo,
		Plugin:      m.plugin.name,
		Marketplace: m.plugin.catalogue,
	}
	if err := tmpl.Execute(&out, data); err != nil {
		return file{}, err
	}
	if !bytes.Contains(out.Bytes(), []byte(sha)) {
		return file{}, errors.New(m.repo + ": its front page does not say what it was published from")
	}
	return file{path: "README.md", content: out.Bytes()}, nil
}
