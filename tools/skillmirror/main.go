// skillmirror renders the standalone skill repositories from the canonical
// skills under .claude/skills.
//
// The skills have to be installable into any agent and not only into one
// working in this checkout, so they are published as two ordinary,
// clone-and-use repositories, split by audience: somebody driving the
// workbench should not be handed the Gio design language. Keeping those in
// step used to be a rule people were asked to remember, and by the time anyone
// diffed the trees the three files were 502 lines apart. That is the worst
// thing this repository can publish: an agent loads a skill and acts on it with
// confidence rather than going and looking, so a stale one does not degrade, it
// teaches something false.
//
// So a mirror is an output, not a copy. Nothing in this tree holds a second
// version of a skill, which is also why there is no generated tree checked in
// for a job to diff: the mirror content exists only as what this renders, and
// two copies cannot disagree when there is one. What can still be got wrong is
// the mapping - a skill added with no mirror, or a mirror naming a skill that
// has gone - and mirror_test.go fails on both, offline, on the machine that
// moved it.
//
// Run from anywhere inside the repo:
//
//	go run ./tools/skillmirror -out DIR   render every mirror into DIR/<repo>
//	go run ./tools/skillmirror -list      name the repositories, one per line
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	out := flag.String("out", "", "render every mirror into this directory, one subdirectory per repository")
	list := flag.Bool("list", false, "print the repository names, one per line, and stop")
	commit := flag.String("commit", "", "the MeshBench commit to stamp into what is published (default: HEAD)")
	flag.Parse()

	if *list {
		for _, m := range mirrors {
			fmt.Println(m.repo)
		}
		return
	}
	if *out == "" {
		fatal(errors.New("-out is required: say where the mirror trees should be written"))
	}

	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	sha := *commit
	if sha == "" {
		if sha, err = headCommit(root); err != nil {
			fatal(err)
		}
	}

	for _, m := range mirrors {
		files, err := renderMirror(root, sha, m)
		if err != nil {
			fatal(err)
		}
		dir := filepath.Join(*out, m.repo)
		if err := writeTree(dir, files); err != nil {
			fatal(err)
		}
		fmt.Printf("%s: %d files from %s\n", m.repo, len(files), sha)
	}
}

// writeTree writes a rendered mirror out. The destination is emptied first so
// that a file the renderer has stopped producing disappears from the mirror
// too: a skill that was deleted here has to stop being installable there, and
// leaving it behind is the same stale-skill fault by another route.
func writeTree(dir string, files []file) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// headCommit is what the published copies name as their origin, so that
// somebody holding an install can tell whether it is current. Without it a
// stale install is invisible rather than merely stale.
func headCommit(root string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("asking git for HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// repoRoot finds the checkout above the working directory, so the tool can be
// run from anywhere in it rather than only from the top.
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
			return "", errors.New("no go.mod above the working directory")
		}
		dir = up
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "skillmirror:", err)
	os.Exit(1)
}
