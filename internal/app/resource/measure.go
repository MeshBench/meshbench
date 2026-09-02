package resource

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Measuring what is on the disk, which is the one thing this package promises
// to be exact about.

// walkBytes adds up a directory, and says how many files that was.
//
// The count matters as well as the size: zero files is "nothing cached yet",
// which is a different row from a cache that holds a few empty ones.
func walkBytes(dir string) (total int64, files int, err error) {
	err = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, walkErr error) error {
		// A file that vanished mid-walk is a cache being used, not a fault:
		// skip it and keep counting rather than abandoning the measurement.
		if walkErr != nil || e == nil || e.IsDir() {
			return nil //nolint:nilerr // a disappearing cache entry is not an error
		}
		if fi, err := e.Info(); err == nil {
			total += fi.Size()
			files++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, 0, err
	}
	return total, files, nil
}

// dirBytes is walkBytes where only the size is wanted.
func dirBytes(dir string) (int64, error) {
	total, _, err := walkBytes(dir)
	return total, err
}

// fileExists is true for something that is there and is not a directory.
// Symbolic links are followed, because a link into an unpacked emulator tree
// is exactly how the tools directory holds one.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// SIBytes is a size in the units somebody would say it in.
//
// Powers of a thousand rather than of 1024, because these numbers are read
// beside what a release page and a download manager say, and those count in
// megabytes of a million.
func SIBytes(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}
