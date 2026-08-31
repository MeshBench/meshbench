//go:build !windows

package firmware

import (
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile claims f with flock, which is what makes the lock the kernel's
// to release: it is attached to the open file description, not written to the
// file's bytes, so it vanishes with the descriptor whether that descriptor is
// closed on purpose or by the process dying.
func tryLockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if err == unix.EWOULDBLOCK {
			return errLocked
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
