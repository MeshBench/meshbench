//go:build !windows

package firmware

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLockFile claims f with flock, which is what makes the lock the kernel's
// to release: it is attached to the open file description, not written to the
// file's bytes, so it vanishes with the descriptor whether that descriptor is
// closed on purpose or by the process dying.
//
// Only contention becomes errLocked. Anything else - a filesystem with no
// working flock, a bad descriptor - is returned as itself, because reporting
// it as contention would send the reader looking for a second node process
// that does not exist.
func tryLockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return errLocked
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
