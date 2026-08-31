package firmware

import (
	"path/filepath"
	"testing"
)

// The whole point: two processes against the same node name are refused
// rather than raced. A second Native or EmulatedNode is a stand-in for that
// second process here - both call LockWorkDir with the same directory, and
// the second one must not get past it.
func TestLockWorkDirRefusesASecondHolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node-1")

	first, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	defer func() { _ = first.Release() }()

	if _, err := LockWorkDir(dir); err == nil {
		t.Fatal("a second lock on the same work dir was granted")
	}
}

// A lock that has been released must let a later run through. Without this,
// every node's second-ever start would be refused rather than only a
// concurrent one.
func TestLockWorkDirIsReusableAfterRelease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node-1")

	first, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}

	second, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("a released lock still blocked a later run: %v", err)
	}
	_ = second.Release()
}

// A process that dies without releasing must not block the next run for
// ever - the whole reason this is an OS-held lock rather than a file whose
// presence is the lock. Killing our own test process is not something a test
// can do to itself, so this exercises the next best proxy: the file
// descriptor closing without Release ever being asked for, which is what a
// killed process leaves the kernel to clean up too.
func TestLockWorkDirReleasesWhenTheHoldingDescriptorCloses(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "node-1")

	first, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if err := first.file.Close(); err != nil {
		t.Fatalf("close underlying descriptor: %v", err)
	}

	second, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("a lock survived its holder's descriptor closing: %v", err)
	}
	_ = second.Release()
}

// Nil and double-release must both be safe: a caller that never acquired a
// lock (WorkDir left empty) should not have to guard every Release call.
func TestWorkDirLockReleaseIsSafeOnNilAndTwice(t *testing.T) {
	var nilLock *WorkDirLock
	if err := nilLock.Release(); err != nil {
		t.Errorf("releasing a nil lock: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "node-1")
	l, err := LockWorkDir(dir)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("first release: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Errorf("second release: %v", err)
	}
}
