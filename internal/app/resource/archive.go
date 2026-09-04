package resource

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ulikunitz/xz"
)

// Unpacking the emulator archives.
//
// In process rather than by shelling out to tar, which would put the one path
// where "it did not work" is hardest to diagnose at the mercy of whatever tar
// the machine happens to have.

// maxArchiveEntry is the largest single file any of this is allowed to write.
// The emulators are large but their biggest file is under 100 MB, and a
// decompressor with no ceiling is how a hostile archive fills a disk.
const maxArchiveEntry = 512 << 20

// extractArchive unpacks an archive into dir, keeping the layout the archive
// carries because the tools depend on it: QEMU resolves its own path to find
// its ROM images beside it, and Renode's portable package is a tree its
// runtime walks.
func extractArchive(src, dir string, k archiveKind) error {
	if k == zipArchive {
		return extractZip(src, dir)
	}
	f, err := os.Open(src) //nolint:gosec // the archive this fetch just wrote
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	switch k {
	case tarGzip:
		zr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("resource: %s will not open as gzip: %w", src, err)
		}
		defer func() { _ = zr.Close() }()
		return writeTar(tar.NewReader(zr), dir)
	case tarXZ:
		// xz rather than gzip because that is what the QEMU fork's
		// cross-compiled matrix publishes, and the standard library has no
		// decompressor for it. Shelling out to tar was the alternative, and it
		// is not one: this has to work on a Windows machine that has no tar,
		// from a desktop application that inherits no PATH.
		zr, err := xz.NewReader(f)
		if err != nil {
			return fmt.Errorf("resource: %s will not open as xz: %w", src, err)
		}
		return writeTar(tar.NewReader(zr), dir)
	default:
		return fmt.Errorf("resource: %s is not an archive this knows how to open", src)
	}
}

func writeTar(tr *tar.Reader, dir string) error {
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("resource: reading the archive: %w", err)
		}
		dst, err := safeJoin(dir, h.Name)
		if err != nil {
			return err
		}
		if err := writeEntry(tr, h, dir, dst); err != nil {
			return err
		}
	}
}

// writeEntry puts one archive entry on disk. Anything that is not a directory,
// a regular file or a symbolic link is refused rather than skipped: a device
// node or a hard link in one of these archives means the archive is not what
// this was written against, and quietly ignoring it would produce a tree that
// is subtly not the published one.
func writeEntry(tr *tar.Reader, h *tar.Header, dir, dst string) error {
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dst, 0o755)
	case tar.TypeSymlink:
		return writeLink(h, dir, dst)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		// The archive's own mode, masked: the executable bit is load-bearing
		// here, and setuid is not something an emulator download gets to set.
		return writeFile(tr, dst, os.FileMode(h.Mode).Perm(), h.Size)
	default:
		return fmt.Errorf("resource: the archive holds %s, which is not a file, "+
			"a directory or a link", h.Name)
	}
}

func writeFile(r io.Reader, dst string, mode os.FileMode, size int64) error {
	if size > maxArchiveEntry {
		return fmt.Errorf("resource: %s claims to be %d bytes, which is more than "+
			"anything published here is", dst, size)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // dst is inside the tools directory, checked by safeJoin
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, io.LimitReader(r, maxArchiveEntry)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// extractZip unpacks a zip, which is how Renode publishes its Windows build.
//
// A separate reader because a zip has a central directory rather than a stream
// of headers, so nothing in the tar path can be reused. What is shared is the
// two rules that matter: every entry lands inside the tree, and nothing is
// written that is larger than anything published here is.
//
// Symbolic links are refused rather than followed. A tar from these forks
// carries legitimate ones and writeLink checks where they arrive; a zip from
// them does not carry any, so an entry claiming to be one means the archive is
// not what this was written against.
func extractZip(src, dir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("resource: %s will not open as a zip: %w", src, err)
	}
	defer func() { _ = r.Close() }()
	for _, e := range r.File {
		dst, err := safeJoin(dir, e.Name)
		if err != nil {
			return err
		}
		mode := e.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			return fmt.Errorf("resource: the zip holds a symbolic link at %s, "+
				"which nothing published here carries", e.Name)
		case e.FileInfo().IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			rc, err := e.Open()
			if err != nil {
				return err
			}
			// The zip's own mode, masked, for the same reason the tar path
			// masks: the executable bit matters and setuid is not something an
			// emulator download gets to set. A zip written on Windows carries
			// no mode at all, which arrives as 0666 and would leave a Renode
			// nobody can run, so anything without the bit gets it.
			perm := mode.Perm()
			if perm&0o111 == 0 {
				perm = 0o755
			}
			err = writeFile(rc, dst, perm, int64(e.UncompressedSize64))
			_ = rc.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// writeLink makes a symbolic link, having first established that following it
// lands inside the tree being unpacked.
//
// The link text is not what has to be checked: Renode's package carries
// "../../IntegrationLibrary/libs/socket-cpp", which leaves its own directory
// and is perfectly legitimate. What matters is where that arrives, so the
// target is resolved against the entry's own place in the archive - in archive
// coordinates, with path rather than filepath - and then checked like any
// other entry.
func writeLink(h *tar.Header, dir, dst string) error {
	if path.IsAbs(h.Linkname) {
		return fmt.Errorf("resource: %s links to an absolute path", h.Name)
	}
	//nolint:gosec // G305 is this check: safeJoin is what refuses the traversal
	if _, err := safeJoin(dir, path.Join(path.Dir(h.Name), h.Linkname)); err != nil {
		return fmt.Errorf("resource: %s links outside the tools directory", h.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Replaced rather than added to, so a re-fetch over an existing tree
	// leaves the same thing a first fetch would.
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(h.Linkname, dst)
}

// safeJoin resolves an archive entry's name inside dir, refusing anything that
// would land outside it.
//
// An archive is somebody else's data even when it is our own fork's release,
// and "../../.ssh/authorized_keys" is a path a tar file is perfectly able to
// carry. The check is on the cleaned result rather than on the name, because
// "a/../../b" contains no leading "..".
func safeJoin(dir, name string) (string, error) {
	clean := filepath.Clean(filepath.Join(dir, filepath.FromSlash(name)))
	if clean != filepath.Clean(dir) &&
		!strings.HasPrefix(clean, filepath.Clean(dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("resource: the archive holds %s, which would land "+
			"outside the tools directory", name)
	}
	return clean, nil
}
