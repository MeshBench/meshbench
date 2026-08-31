//go:build windows

package firmware

import (
	"os"

	"golang.org/x/sys/windows"
)

// tryLockFile claims f with LockFileEx, Windows's equivalent of flock: the
// lock belongs to the handle, and the kernel drops it when the handle closes,
// on purpose or because the process holding it is gone.
func tryLockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err != nil {
		return errLocked
	}
	return nil
}

func unlockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
