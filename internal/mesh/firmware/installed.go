package firmware

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Installed is one firmware build sitting in the cache.
//
// The cache is the only thing that decides what a node can actually run, and
// until now nothing could see it. A version that failed to download, a build
// left behind by a rename, and a build in daily use all look the same from
// outside the directory, which makes "why is this node not starting" a question
// answered by a shell rather than by the application.
type Installed struct {
	// Native builds run on this machine and carry no board: MeshCore compiled
	// for the host. Board builds are the published images people flash, and
	// only mean anything alongside the hardware they were built for.
	Native bool

	Version string
	Role    string
	Board   string

	Path  string
	Bytes int64
}

// Label is how a row reads in the manager.
func (i Installed) Label() string {
	if i.Native {
		return i.Role + " " + i.Version + " (native)"
	}
	return i.Role + " " + i.Version + " (" + i.Board + ")"
}

// boardDir is where downloaded and imported board images live.
//
// Separate from native/ because they are not interchangeable: one is an
// executable for this machine and the other is a flash image for a particular
// piece of hardware, and putting them in one directory invites running the
// wrong one.
const boardDir = "board"

// ListInstalled reports every build in the cache, newest naming first.
//
// Missing or unreadable directories are not an error. A cache that does not
// exist yet is the normal state on a fresh machine, and reporting nothing is
// the honest answer.
func ListInstalled(cacheDir string) []Installed {
	var out []Installed
	out = append(out, listNative(cacheDir)...)
	out = append(out, listBoard(cacheDir)...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Native != out[j].Native {
			return out[i].Native
		}
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].Role < out[j].Role
	})
	return out
}

func listNative(cacheDir string) []Installed {
	root := filepath.Join(cacheDir, "native")
	versions, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Installed
	for _, v := range versions {
		if !v.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, v.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			// obj/ is the build tree a local compile leaves behind; it is not a
			// firmware and listing it would be noise.
			if f.IsDir() || !strings.HasPrefix(f.Name(), "meshcore-") {
				continue
			}
			role := roleFromBinary(f.Name())
			p := filepath.Join(root, v.Name(), f.Name())
			out = append(out, Installed{
				Native: true, Version: v.Name(), Role: role,
				Path: p, Bytes: sizeOf(p),
			})
		}
	}
	return out
}

func listBoard(cacheDir string) []Installed {
	root := filepath.Join(cacheDir, boardDir)
	boards, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Installed
	for _, b := range boards {
		if !b.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, b.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			role, version := roleVersionFromImage(f.Name())
			p := filepath.Join(root, b.Name(), f.Name())
			out = append(out, Installed{
				Version: version, Role: role, Board: b.Name(),
				Path: p, Bytes: sizeOf(p),
			})
		}
	}
	return out
}

func sizeOf(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

// roleFromBinary pulls the role out of meshcore-<role>-<os>-<arch>.
func roleFromBinary(name string) string {
	s := strings.TrimPrefix(name, "meshcore-")
	s = strings.TrimSuffix(s, ".exe")
	// The trailing os-arch is two segments, and a role may itself contain
	// hyphens, so trim from the right rather than splitting.
	for i := 0; i < 2; i++ {
		if k := strings.LastIndex(s, "-"); k > 0 {
			s = s[:k]
		}
	}
	return s
}

// roleVersionFromImage pulls role and version out of <role>-<version>.bin,
// which is how board images are stored.
func roleVersionFromImage(name string) (role, version string) {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	k := strings.LastIndex(s, "-")
	if k <= 0 {
		return s, ""
	}
	return s[:k], s[k+1:]
}

// Remove deletes one build.
//
// It refuses anything outside the cache. The manager passes back a path this
// package produced, but a delete button driven by a path is exactly the shape
// of thing that removes somebody's home directory when a future caller builds
// the path differently.
func Remove(cacheDir string, in Installed) error {
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return err
	}
	p, err := filepath.Abs(in.Path)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		return fmt.Errorf("firmware: refusing to delete %s, which is outside the cache", p)
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	// Take the version directory with it when it empties, so a cache that has
	// been cleared out does not read as a list of empty versions.
	dir := filepath.Dir(p)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
	return nil
}

// Import copies a local build into the cache so a node can select it.
//
// Wanted for anything that was never released, which is most of what is worth
// testing: a branch build, a patched image, a colleague's binary.
func Import(cacheDir, src, version, role, board string) (Installed, error) {
	if version == "" || role == "" {
		return Installed{}, fmt.Errorf("firmware: an imported build needs a version and a role")
	}
	st, err := os.Stat(src)
	if err != nil {
		return Installed{}, err
	}
	if st.IsDir() {
		return Installed{}, fmt.Errorf("firmware: %s is a directory", src)
	}

	var dst string
	native := board == ""
	if native {
		dst = filepath.Join(cacheDir, "native", version, NativeBinaryName(role))
	} else {
		dst = filepath.Join(cacheDir, boardDir, board, role+"-"+version+filepath.Ext(src))
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Installed{}, err
	}
	if err := copyFile(src, dst, native); err != nil {
		return Installed{}, err
	}
	return Installed{
		Native: native, Version: version, Role: role, Board: board,
		Path: dst, Bytes: sizeOf(dst),
	}, nil
}

// copyFile writes the build in, executable if it is one for this machine.
//
// Downloads arrive without the execute bit and a native build that cannot be
// executed fails at start with a permission error that names the file and not
// the reason.
func copyFile(src, dst string, executable bool) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
