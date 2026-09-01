//go:build windows

package firmware

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile claims f with LockFileEx, Windows's equivalent of flock: the
// lock belongs to the handle, and the kernel drops it when the handle closes,
// on purpose or because the process holding it is gone.
//
// Only a lock actually held elsewhere becomes errLocked. Anything else - a bad
// handle, a filesystem that cannot lock - is returned as itself, because
// reporting it as contention would send the reader looking for a second node
// process that does not exist.
func tryLockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err == nil {
		return nil
	}
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLocked
	}
	return err
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
