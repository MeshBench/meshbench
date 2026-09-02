package firmware

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ImportLabel is what to call an imported build when nobody said.
//
// A timestamp, because the alternative was the constant "imported" and every
// import then collided with the last one: the library showed a single entry,
// pinning it pinned whichever file had most recently overwritten the other,
// and there was no way to say which of two local builds a node was running. A
// label somebody chose is always better, and this is what happens when they
// did not.
//
// Local time, and no separators past the day, so it sorts the way it reads and
// survives being part of a filename on every platform.
func ImportLabel(chosen string) string {
	if chosen != "" {
		return chosen
	}
	return "imported-" + time.Now().Format("20060102-150405")
}

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

// BoardDir is where downloaded and imported board images live.
//
// Separate from native/ because they are not interchangeable: one is an
// executable for this machine and the other is a flash image for a particular
// piece of hardware, and putting them in one directory invites running the
// wrong one.
const BoardDir = "board"

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
			//
			// A downloaded build's digest sits beside it and keeps its name, so
			// it parses to the same role and the same version as the build. The
			// library keys rows on exactly that, and ReadDir hands the sidecar
			// over second, so it replaced the build it describes: every row read
			// as 65 bytes of hex.
			if f.IsDir() || !strings.HasPrefix(f.Name(), "meshcore-") || isChecksumFile(f.Name()) {
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
	root := filepath.Join(cacheDir, BoardDir)
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
			// A build's settings sit beside it under its own name. Listed,
			// they would appear as builds whose role is the whole filename.
			if f.IsDir() || isSettingsFile(f.Name()) {
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

// labelSep separates an imported build's role from the label it was given.
//
// A hyphen was used for both this and the published form, and the two are not
// separable: roles contain hyphens ("room-server") and so does any label worth
// having ("wadamesh-20260823-142530"), so splitting at the last one turned
// "repeater-mine-1" into role "repeater-mine" at version "1". A character that
// appears in neither ends the guessing.
//
// Published downloads keep the hyphen. Their versions never contain one, the
// reader below handles both, and changing their names would invalidate every
// cache already on disk - which for somebody with thirty boards downloaded is
// a gigabyte re-fetched to fix a bug they never hit.
const labelSep = "@"

// roleVersionFromImage pulls role and version out of a stored board image.
//
// Two forms: <role>@<label> for an imported build, and <role>-<version> for a
// published one.
func roleVersionFromImage(name string) (role, version string) {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	if k := strings.Index(s, labelSep); k > 0 {
		return s[:k], s[k+1:]
	}
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
	// And whatever was decided about it, which means nothing once the build
	// it describes is gone and would be inherited by the next build to be
	// given the same name.
	if err := os.Remove(SettingsPath(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The digest, for the same reason and with a sharper edge. Import writes
	// no digest, so a local build copied over a deleted download inherits the
	// download's: checksumOK then rejects bytes that are perfectly good,
	// cachedBinary reports the build as absent, and the import silently does
	// not take. Left behind it would also keep the version directory from
	// emptying below.
	if err := os.Remove(checksumSidecarPath(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
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
	// Refused rather than sanitised. A label quietly rewritten is a build
	// nobody can find again under the name they gave it, and one containing a
	// path separator is a build written somewhere nobody asked for.
	if strings.ContainsAny(version, `/\`+labelSep) || strings.ContainsAny(role, `/\`+labelSep) {
		return Installed{}, fmt.Errorf(
			"firmware: %q is not usable as a name; no %s, / or \\ in a role or a label",
			version, labelSep)
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
		dst = filepath.Join(cacheDir, BoardDir, board, role+labelSep+version+filepath.Ext(src))
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
	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}

	same, err := sameFile(src, dst)
	if err != nil {
		return err
	}
	if same {
		// A build already lands at the path a later import is handed, so src
		// and dst can be the identical file. Opening dst with O_TRUNC in that
		// case truncates the file src is about to read, emptying the build
		// before a byte moves and leaving no error to explain it. There is
		// nothing to copy; only the mode may still need setting.
		return os.Chmod(dst, mode)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

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

// sameFile reports whether src and dst name the identical file, by identity
// rather than by string comparison: a symlink or a relative path can make two
// spellings of one file look like two files, and a copy that trusts the
// strings truncates the one it meant to read from.
//
// A destination that does not exist yet is not an error here; it just is not
// the same file, which is the ordinary case for every import.
func sameFile(src, dst string) (bool, error) {
	sSrc, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	sDst, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(sSrc, sDst), nil
}
