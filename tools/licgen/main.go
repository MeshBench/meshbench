// licgen assembles the licence inventory the workbench embeds and the bundle
// carries as files.
//
// The curated half - the forks, the bundled native pieces, what is downloaded
// at runtime, the data attributions - lives in docs/licences.json with its
// texts checked in under docs/licences/, so generation needs no network. The
// Go half is discovered from the build graph of ./cmd/meshbench: every
// linked module's licence file is read from the module cache and classified,
// and a module whose licence cannot be named fails the run - that is the
// enforcement, not a warning.
//
// Run from anywhere inside the repo:
//
//	go run ./tools/licgen                  regenerate the embedded licence inventory
//	go run ./tools/licgen -text DIR        also write one text file per entry into DIR
//	go run ./tools/licgen -require-project-licence
//	                                       fail if the project itself still has no licence,
//	                                       which is what stops a tagged release shipping unlicensed
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one thing the application ships under, in the window's words.
type Entry struct {
	Name    string `json:"name"`
	Detail  string `json:"detail,omitempty"`
	Licence string `json:"licence"`
	Link    string `json:"link,omitempty"`
	Text    string `json:"text,omitempty"`
}

// Section is one group of entries; the order in the file is the order in the
// window, and the forks come before everything third-party because they are
// modified code.
type Section struct {
	Key     string  `json:"key"`
	Title   string  `json:"title"`
	Blurb   string  `json:"blurb,omitempty"`
	Entries []Entry `json:"entries"`
}

// File is what the workbench embeds.
type File struct {
	Sections []Section `json:"sections"`
}

// curated mirrors docs/licences.json: entries whose text lives in files.
type curated struct {
	Project curatedEntry   `json:"project"`
	Forks   []curatedEntry `json:"forks"`
	Bundled []curatedEntry `json:"bundled"`
	Runtime []curatedEntry `json:"runtime"`
	Data    []curatedEntry `json:"data"`
	// GoNotes says something a licence file does not, keyed by module path.
	// The one that matters: paho is dual-licensed and we take the branch
	// that is GPL-compatible, which no automatic reading would know.
	GoNotes map[string]string `json:"go_notes,omitempty"`
}

type curatedEntry struct {
	Entry
	TextFiles []string `json:"text_files,omitempty"`
}

func main() {
	textDir := flag.String("text", "", "also write one licence file per entry into this directory")
	requireLicence := flag.Bool("require-project-licence", false,
		"fail if the project's own licence is still unchosen")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}

	cur, err := readCurated(root)
	if err != nil {
		fatal(err)
	}
	if *requireLicence {
		if strings.Contains(strings.ToLower(cur.Project.Licence), "none") {
			fatal(fmt.Errorf("the project has no licence: a release cannot ship; choose one, write it up in docs/licence.md and update docs/licences.json"))
		}
		if _, err := os.Stat(filepath.Join(root, "LICENSE")); err != nil {
			fatal(fmt.Errorf("docs/licences.json names a licence but there is no LICENSE file: %w", err))
		}
		return
	}

	golibs, err := goLibraries(root, cur.GoNotes)
	if err != nil {
		fatal(err)
	}

	// The project's own text is the LICENSE file itself, so the window can
	// only ever show what the repository actually ships under.
	project := cur.Project.resolve(root)
	if b, err := os.ReadFile(filepath.Join(root, "LICENSE")); err == nil {
		project.Text = strings.TrimSpace(string(b))
	}

	out := File{Sections: []Section{
		{Key: "project", Title: "MeshBench", Entries: []Entry{project}},
		{Key: "forks", Title: "Modified forks",
			Blurb:   "Repositories forked and changed for MeshBench. Each entry says what was changed; the fork is the source offer for the binaries in this bundle.",
			Entries: resolve(root, cur.Forks)},
		{Key: "bundled", Title: "Bundled third parties",
			Blurb:   "Native pieces inside this bundle that are not Go modules.",
			Entries: resolve(root, cur.Bundled)},
		{Key: "golibs", Title: "Go libraries",
			Blurb:   "Every module linked into the binary, found from the build graph and classified at generation time.",
			Entries: golibs},
		{Key: "runtime", Title: "Downloaded at runtime",
			Blurb:   "Fetched on first use and cached; never inside this bundle.",
			Entries: resolve(root, cur.Runtime)},
		{Key: "data", Title: "Map and terrain data",
			Blurb:   "The attributions the basemaps and elevation sources require.",
			Entries: resolve(root, cur.Data)},
	}}

	buf, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		fatal(err)
	}
	dest := filepath.Join(root, inventoryPath)
	// The directory must already exist: this file is compiled into the
	// workbench, so writing it somewhere new would produce an inventory
	// nothing embeds while reporting success. The seven-layer move renamed
	// internal/workbench to internal/ui/workbench and this path did not
	// follow, which broke the release pipeline silently for two days -
	// licgen runs only on a tag, so nothing else ever executed it.
	if _, err := os.Stat(filepath.Dir(dest)); err != nil {
		fatal(fmt.Errorf("licgen: %s is not where the inventory lives: %w", dest, err))
	}
	if err := os.WriteFile(dest, append(buf, '\n'), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s: %d sections, %d Go modules\n", dest, len(out.Sections), len(golibs))

	if *textDir != "" {
		if err := writeText(*textDir, out); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote licence files to %s\n", *textDir)
	}
}

// resolve reads a curated entry's text files in and joins them with any
// inline text.
func (c curatedEntry) resolve(root string) Entry {
	e := c.Entry
	var parts []string
	if e.Text != "" {
		parts = append(parts, e.Text)
	}
	for _, f := range c.TextFiles {
		b, err := os.ReadFile(filepath.Join(root, "docs", "licences", f))
		if err != nil {
			fatal(fmt.Errorf("curated entry %q: %w", c.Name, err))
		}
		parts = append(parts, strings.TrimSpace(string(b)))
	}
	e.Text = strings.Join(parts, "\n\n----\n\n")
	return e
}

func resolve(root string, in []curatedEntry) []Entry {
	out := make([]Entry, 0, len(in))
	for _, c := range in {
		out = append(out, c.resolve(root))
	}
	return out
}

func readCurated(root string) (curated, error) {
	var c curated
	b, err := os.ReadFile(filepath.Join(root, "docs", "licences.json"))
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("docs/licences.json: %w", err)
	}
	return c, nil
}

// goLibraries walks the build graph of ./cmd/meshbench (the superset: both
// workbenches) and reads each linked module's licence from the module cache.
func goLibraries(root string, notes map[string]string) ([]Entry, error) {
	cmd := exec.Command("go", "list", "-deps",
		"-f", `{{if .Module}}{{if not .Module.Main}}{{.Module.Path}}	{{.Module.Version}}	{{.Module.Dir}}{{end}}{{end}}`,
		"./cmd/meshbench")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	seen := map[string]bool{}
	var entries []Entry
	var unknown []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 3 || seen[parts[0]] {
			continue
		}
		seen[parts[0]] = true
		mod, ver, dir := parts[0], parts[1], parts[2]
		text, err := licenceText(dir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mod, err)
		}
		name := classify(text)
		if why := incompatible(name); why != "" {
			unknown = append(unknown, fmt.Sprintf("%s (%s: %s)", mod, orWord(name, "unrecognised"), why))
			continue
		}
		detail := ver
		if n := notes[mod]; n != "" {
			detail = ver + " - " + n
		}
		entries = append(entries, Entry{
			Name:    mod,
			Detail:  detail,
			Licence: name,
			Link:    "https://" + mod,
			Text:    strings.TrimSpace(text),
		})
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("modules whose licence cannot ship: %s - classify it in tools/licgen or remove the dependency",
			strings.Join(unknown, ", "))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func orWord(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func licenceText(dir string) (string, error) {
	for _, name := range []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "LICENCE", "COPYING", "UNLICENSE"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("no licence file in %s", dir)
}

// incompatible says why a linked module's licence cannot ship inside
// MeshBench, or "" when it can. MeshBench is GPL-3.0-or-later (docs/licence.md), so
// the question is not "is this permissive" but "can this be combined with
// GPL-3.0 and conveyed".
//
// This is about *linked* code only. The emulator forks are separate processes
// and are not measured here - a GPL-2.0 QEMU beside our binary is aggregation,
// which is why that fork is fine and a GPL-2.0-only Go module would not be.
func incompatible(name string) string {
	switch {
	case name == "":
		return "licence not recognised"
	case name == "GPL-2.0-only":
		return "GPL-2.0 without 'or later' cannot combine with GPL-3.0"
	case name == "EPL-2.0":
		return "EPL-2.0 alone is not GPL-compatible; only the dual EPL/EDL form is"
	case strings.HasPrefix(name, "AGPL"):
		return "AGPL would change the terms of the whole work"
	case strings.Contains(strings.ToUpper(name), "SSPL"),
		strings.Contains(strings.ToUpper(name), "BUSL"):
		return "source-available, not free software: cannot ship in a GPL work"
	}
	return ""
}

// classify names a licence from its text. Deliberately small: it covers what
// this project actually links, and an unmatched text fails generation rather
// than shipping as a shrug.
func classify(text string) string {
	// A stated SPDX identifier beats any guessing.
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(strings.ToLower(line), "spdx-license-identifier:"); i >= 0 {
			if id := strings.TrimSpace(line[i+len("spdx-license-identifier:"):]); id != "" {
				return id
			}
		}
	}
	t := strings.ToLower(text)
	switch {
	// Eclipse dual licensing, which decides GPL compatibility and which no
	// single-name reading would catch: EPL alone cannot be linked into a GPL
	// program, EDL-1.0 is BSD-3-Clause verbatim and can.
	case strings.Contains(t, "eclipse public license") &&
		strings.Contains(t, "eclipse distribution license"):
		return "EPL-2.0 or EDL-1.0"
	case strings.Contains(t, "eclipse public license"):
		return "EPL-2.0"
	case strings.Contains(t, "gnu affero"):
		return "AGPL-3.0"
	case strings.Contains(t, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(t, "mit license") ||
		strings.Contains(t, "permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "apache license") && strings.Contains(t, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "mozilla public license") && strings.Contains(t, "2.0"):
		return "MPL-2.0"
	case strings.Contains(t, "gnu general public license"):
		// Version 2 without the "or later" escape hatch cannot be combined
		// with our GPL-3.0; version 3 and "2 or later" can.
		if strings.Contains(t, "version 2") &&
			!strings.Contains(t, "any later version") {
			return "GPL-2.0-only"
		}
		return "GPL"
	case strings.Contains(t, "redistribution and use in source and binary forms"):
		if strings.Contains(t, "neither the name") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	}
	return ""
}

// writeText writes the same inventory as files, for the people who read a
// LICENCES directory rather than open a window.
func writeText(dir string, f File) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var index strings.Builder
	index.WriteString("What MeshBench ships under. Generated by tools/licgen; the same\ncontent is in the application under Help > Licences & attributions.\n\n")
	for _, s := range f.Sections {
		index.WriteString("\n" + s.Title + "\n")
		for _, e := range s.Entries {
			fn := s.Key + "-" + slug(e.Name) + ".txt"
			fmt.Fprintf(&index, "  %-46s %s\n", e.Name, e.Licence)
			if e.Text == "" {
				continue
			}
			body := e.Name + "\n" + e.Licence + "\n"
			if e.Detail != "" {
				body += e.Detail + "\n"
			}
			if e.Link != "" {
				body += e.Link + "\n"
			}
			body += "\n" + e.Text + "\n"
			if err := os.WriteFile(filepath.Join(dir, fn), []byte(body), 0o644); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(filepath.Join(dir, "INDEX.txt"), []byte(index.String()), 0o644)
}

func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = up
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "licgen:", err)
	os.Exit(1)
}

// inventoryPath is where the generated inventory lives, relative to the repo
// root. Named here rather than spelled inline so the package that embeds it
// and the tool that writes it can be checked against each other.
const inventoryPath = "internal/ui/workbench/licences/licences.json"
