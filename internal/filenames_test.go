// Filenames a reader can tell apart, enforced.
//
// The Google Go Style Guide, which CLAUDE.md holds this tree to, sets the test
// for how code is spread across files: they must be "focused enough that a
// maintainer can tell which file contains something, and small enough that it
// will be easy to find once there". The first half is the one a rule can check.
//
// It was failing quietly. internal/ui/workbench carried logpanel.go and
// logspanel.go - two different panels, one letter apart, one showing what has
// happened this session and the other every status line it has said. Beside
// them sat nodewindow.go and nodewindows.go, firmwarewindow.go and
// firmwarewindows.go: in each pair one file is a single window and the other is
// the collection of them. Nothing about the names says which.
//
// The shared property is that removing the letter s makes the two identical.
// That is the whole rule here, and it is deliberately narrow: it catches the
// plural-versus-singular collision that actually happened and stays quiet about
// everything else, rather than trying to be a general theory of good names.
package internal_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// roots are the trees this applies to. Not pkg/, whose clients follow the
// naming of the languages they are written for, and not testdata.
var roots = []string{"..", "../cmd", "../tools"}

// stem is a Go file's name with the extension and any build-constraint or test
// suffix taken off, so a file and its own test are one name rather than two.
func stem(name string) string {
	n := strings.TrimSuffix(name, ".go")
	for _, suffix := range []string{"_test", "_unix", "_windows", "_darwin", "_linux", "_other"} {
		n = strings.TrimSuffix(n, suffix)
	}
	return n
}

func TestNoTwoFilesDifferOnlyByAnS(t *testing.T) {
	for _, root := range roots {
		byPackage := map[string]map[string][]string{}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				// Vendored, generated and fixture trees are not ours to name,
				// and neither is another worktree: .claude/worktrees holds
				// checkouts of this same repository, so walking into one
				// reports every fault twice and blames a path nobody edits.
				name := info.Name()
				if strings.HasPrefix(name, ".") && name != "." && name != ".." {
					return filepath.SkipDir
				}
				switch name {
				case "testdata", "vendor", "node_modules", "pkg":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}
			dir := filepath.Dir(path)
			if byPackage[dir] == nil {
				byPackage[dir] = map[string][]string{}
			}
			s := stem(info.Name())
			// The key is the stem with every s removed. Two stems that collide
			// on it differ only by pluralisation, which is the pair a reader
			// cannot tell apart.
			key := strings.ReplaceAll(s, "s", "")
			seen := byPackage[dir][key]
			for _, already := range seen {
				if already == s {
					return nil // the same stem again, from its own test file
				}
			}
			byPackage[dir][key] = append(seen, s)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}

		for dir, keys := range byPackage {
			for _, stems := range keys {
				if len(stems) < 2 {
					continue
				}
				sort.Strings(stems)
				t.Errorf("%s holds %v, which differ only by an s.\n"+
					"Two files a reader has to open to tell apart are two files "+
					"they will open. Name them for what they hold: the single "+
					"thing and the collection of them are different words, not "+
					"one word and its plural.", dir, stems)
			}
		}
	}
}
