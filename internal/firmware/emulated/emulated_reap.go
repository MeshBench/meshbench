package emulated

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Stop ends both processes.
func (e *EmulatedNode) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopLocked()
}

func (e *EmulatedNode) stopLocked() error {
	if e.serial != nil {
		_ = e.serial.Close()
		e.serial = nil
	}
	var procs []*exec.Cmd
	for _, c := range []*exec.Cmd{e.qemu, e.radio} {
		if c == nil || c.Process == nil {
			continue
		}
		_ = c.Process.Kill()
		procs = append(procs, c)
	}
	e.qemu, e.radio = nil, nil
	if e.renodeStdin != nil {
		_ = e.renodeStdin.Close()
		e.renodeStdin = nil
	}
	// The receiver stops with the board. It is the one of these that keeps a
	// clock running of its own, so leaving it would have a stopped node still
	// reporting where it is.
	if e.GPS != nil {
		_ = e.GPS.Close()
		e.GPS = nil
	}
	sock := e.sock

	// Reaped with the lock released: a process killed while blocked on a
	// stalled disk or network write does not die the instant it is killed, it
	// dies when that write returns, and waiting for it here - as this used to -
	// queued every other method on this node behind however long the mount
	// stayed stuck. ConsoleIn and HasConsole take the same lock this wait once
	// held for however long that took, with no bound at all.
	e.mu.Unlock()
	err := reapAll(procs, reapPeriod)
	e.mu.Lock()

	_ = os.Remove(sock)
	// Released only once every process that might still be touching Dir is
	// confirmed gone. A reap that timed out leaves the lock held: a node this
	// backend can no longer account for might still be writing to it, and
	// letting a second node start on top of that is the exact corruption the
	// lock exists to rule out.
	if err == nil && e.workLock != nil {
		_ = e.workLock.Release()
		e.workLock = nil
	}
	return err
}

// reapPeriod is how long a killed process is given to actually exit. It
// exists only so that a teardown ends; a node behaving normally never reaches
// it, and one that does is reported rather than waited on for ever.
const reapPeriod = 5 * time.Second

// reapAll waits for every already-killed process to exit, each bounded by
// period, and reports which - if any - never did.
//
// SIGKILL does not make a process disappear the instant it is sent: a process
// blocked on a stalled disk or network write is in the kernel's own
// uninterruptible state and only dies once that write returns. Waiting for it
// without a deadline took the whole teardown down with it, and did so while
// holding the node's own lock - which every other method here, ConsoleIn and
// HasConsole included, also needs.
//
// period is a parameter rather than always reapPeriod so a test can ask this
// to give up in milliseconds instead of five seconds - the only practical way
// to exercise the timeout path without an actually stuck process.
func reapAll(procs []*exec.Cmd, period time.Duration) error {
	var errs []error
	for _, c := range procs {
		if err := waitBounded(c, period); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func waitBounded(c *exec.Cmd, period time.Duration) error {
	done := make(chan struct{})
	go func() {
		_, _ = c.Process.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(period):
		return fmt.Errorf("firmware: process %d has not been reaped %v after being killed",
			c.Process.Pid, period)
	}
}
