package emulated

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// waitBounded must return once the process is actually reaped, which is the
// ordinary case and the one every board probe depends on completing at all.
func TestWaitBoundedReturnsOnceTheProcessIsReaped(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skipf("no /usr/bin/true on this machine: %v", err)
	}
	if err := waitBounded(cmd, 5*time.Second); err != nil {
		t.Fatalf("a process that exited on its own was reported unreaped: %v", err)
	}
}

// A period is a period. Genuinely unreapable is a process stuck in the
// kernel's own uninterruptible state - blocked on a stalled disk or network
// write - which nothing short of an actually stalled filesystem can produce.
// A live process that simply has not exited yet takes the same code path from
// waitBounded's point of view, and is the practical way to prove the deadline
// is real rather than decorative.
func TestWaitBoundedGivesUpAtThePeriod(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Skipf("no sleep on this machine: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	start := time.Now()
	err := waitBounded(cmd, 50*time.Millisecond)
	if err == nil {
		t.Fatal("a still-running process was reported reaped")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waitBounded took %v to give up on a 50ms period", elapsed)
	}
}

// Stop kills both the emulator and the radio model. Neither failing to reap
// should hide the other: a caller told only one process is stuck goes looking
// for the wrong one.
func TestReapAllReportsEveryUnreapedProcess(t *testing.T) {
	one, two := exec.Command("sleep", "5"), exec.Command("sleep", "5")
	for _, c := range []*exec.Cmd{one, two} {
		if err := c.Start(); err != nil {
			t.Skipf("no sleep on this machine: %v", err)
		}
		defer func(c *exec.Cmd) {
			_ = c.Process.Kill()
			_, _ = c.Process.Wait()
		}(c)
	}

	err := reapAll([]*exec.Cmd{one, two}, 50*time.Millisecond)
	if err == nil {
		t.Fatal("two still-running processes were reported reaped")
	}
	if one.Process.Pid == two.Process.Pid {
		t.Fatal("test setup gave both commands the same process")
	}
	for _, pid := range []int{one.Process.Pid, two.Process.Pid} {
		if !strings.Contains(err.Error(), strconv.Itoa(pid)) {
			t.Errorf("reapAll's error does not mention process %d: %v", pid, err)
		}
	}
}
