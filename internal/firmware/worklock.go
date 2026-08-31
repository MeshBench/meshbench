package firmware

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// lockFileName is the sentinel a WorkDirLock claims, rather than the
// directory's own existence: the directory holds a node's real files -
// flash.bin, radio.sock, the peripheral sockets - and those exist whether or
// not anything currently holds the lock.
const lockFileName = ".lock"

// errLocked means the sentinel is already held by somebody else. Wrapped
// rather than returned bare so LockWorkDir's own error can name the directory.
var errLocked = errors.New("locked by another process")

// WorkDirLock is one process's exclusive claim on a node's working directory.
type WorkDirLock struct {
	file *os.File
}

// LockWorkDir claims dir for the caller's exclusive use, refusing rather than
// racing when another process already holds it.
//
// A node's WorkDir is stable across runs by design (see NodeWorkDir) - a
// repeater keeping its identity between sessions is how hardware behaves -
// which is exactly what makes two live processes against the same node name
// dangerous: nothing stopped them sharing radio.sock, flash.bin and the
// peripheral sockets, and three hundred processes once did, silently.
//
// The lock is filesystem-advisory and owned by the operating system rather
// than recorded as a fact on disk, so a process that never calls Release -
// killed, crashed, or simply forgotten - cannot leave a stale lock behind: the
// kernel drops it the moment that process's file descriptors close, which
// happens on exit however the exit occurs. A lock file whose mere presence
// meant "in use" would need something to notice the holder was gone and clean
// up after it; this needs nothing to notice anything.
func LockWorkDir(dir string) (*WorkDirLock, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("firmware: node work dir: %w", err)
	}
	path := filepath.Join(dir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("firmware: node work dir lock: %w", err)
	}
	if err := tryLockFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("firmware: %s is already in use by another node process: %w", dir, err)
	}
	return &WorkDirLock{file: f}, nil
}

// Release gives up the claim. Safe on a nil lock or one already released, so a
// caller that never acquired one - WorkDir left empty - does not need to guard
// the call, and neither does a second call from a defer that races a normal
// return.
func (l *WorkDirLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
