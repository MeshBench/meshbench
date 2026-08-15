package fixture

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	embedded "github.com/MeshBench/meshbench/fixtures"
)

// Finding a fixture, given whatever the user or a default said.
//
// `-fixture fixtures/fixture-fife-strict.json` is a path relative to the
// working directory, and that was the whole of it until 0.0.1 shipped: a
// .deb keeps its fixtures in /usr/share/meshbench, a macOS bundle in
// Contents/Resources, and both launch from somewhere else entirely, so the
// application opened with nothing loaded and no explanation. The tarball only
// worked because the README says to cd into it first.
//
// So a fixture may now be named three ways, tried in this order:
//
//  1. a path that exists, exactly as given - this always wins, because
//     somebody who typed a path meant that file
//  2. a name, looked for in the places an installed copy keeps them
//  3. a name, from the copy compiled into the binary
//
// Which means `-fixture fife-strict` works anywhere, `-fixture ./mine.json`
// works as it always did, and an installed application can open its own
// example network on a machine with no fixtures on disk at all.

// SearchDirs is where an installed copy might keep its fixtures, in the order
// they are tried. Exported because the Firmware Library and the file chooser
// want to start somewhere sensible too.
func SearchDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		if exe, err := filepath.EvalSymlinks(exe); err == nil {
			d := filepath.Dir(exe)
			// Beside the binary: the tarball and the Windows zip.
			dirs = append(dirs, filepath.Join(d, "fixtures"))
			// A macOS bundle: MacOS/meshbench-bin -> ../Resources/fixtures.
			if runtime.GOOS == "darwin" {
				dirs = append(dirs, filepath.Join(d, "..", "Resources", "fixtures"))
			}
			// An AppImage or a /usr/bin install: ../share/meshbench/fixtures.
			dirs = append(dirs, filepath.Join(d, "..", "share", "meshbench", "fixtures"))
		}
	}
	if runtime.GOOS != "windows" {
		dirs = append(dirs, "/usr/share/meshbench/fixtures", "/usr/local/share/meshbench/fixtures")
	}
	// XDG, last, because a system copy should not shadow the one beside the
	// binary somebody just downloaded.
	if x := os.Getenv("XDG_DATA_DIRS"); x != "" {
		for _, d := range filepath.SplitList(x) {
			dirs = append(dirs, filepath.Join(d, "meshbench", "fixtures"))
		}
	}
	// And the working directory, which is what every existing script assumes.
	dirs = append(dirs, "fixtures")
	return dirs
}

// Find turns whatever was asked for into something Load can read: a path on
// disk, or a name of an embedded fixture. The second return says whether the
// answer is embedded rather than a path.
func Find(nameOrPath string) (string, bool, error) {
	if nameOrPath == "" {
		return "", false, fmt.Errorf("fixture: nothing named")
	}
	// 1. Exactly what was asked for, if it is there.
	if st, err := os.Stat(nameOrPath); err == nil && !st.IsDir() {
		return nameOrPath, false, nil
	}
	// 2. The same file name in the places an install keeps them. Both the
	//    bare name and the shipped file-name shape are tried, so
	//    "fife-strict", "fixture-fife-strict" and the full file name all
	//    reach the same fixture.
	for _, dir := range SearchDirs() {
		for _, cand := range candidates(nameOrPath) {
			p := filepath.Join(dir, cand)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, false, nil
			}
		}
	}
	// 3. The copy inside the binary.
	for _, cand := range candidates(nameOrPath) {
		if _, err := embedded.FS.Open(cand); err == nil {
			return cand, true, nil
		}
	}
	return "", false, fmt.Errorf(
		"fixture: no fixture called %q - built in: %s", nameOrPath,
		strings.Join(Embedded(), ", "))
}

// candidates is the file names a given name might mean.
func candidates(name string) []string {
	base := filepath.Base(name)
	out := []string{base}
	if !strings.HasSuffix(base, ".json") {
		out = append(out, base+".json", "fixture-"+base+".json")
	}
	if !strings.HasPrefix(base, "fixture-") {
		out = append(out, "fixture-"+base)
	}
	return out
}

// Embedded is the names built into this binary, for an error message that
// tells somebody what they could have typed.
func Embedded() []string {
	var out []string
	_ = fs.WalkDir(embedded.FS, ".", func(p string, d fs.DirEntry, err error) error {
		// A per-entry err here means this one entry could not be read, not
		// that the walk should stop - skipping it and continuing is the
		// correct response, not an error being dropped on the floor.
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil //nolint:nilerr
		}
		n := strings.TrimSuffix(strings.TrimPrefix(p, "fixture-"), ".json")
		out = append(out, n)
		return nil
	})
	sort.Strings(out)
	return out
}
