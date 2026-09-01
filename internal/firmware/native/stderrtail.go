package native

import "sync"

// stderrTailCap bounds how much of a child's stderr is kept.
//
// A few KB holds every diagnostic line a boot ever needs - the dynamic
// loader's rejection of a binary is one line - while still refusing to grow
// for a node that writes without stopping. Node.Log, when the caller set one,
// still gets everything; this is only the account kept when nobody did.
const stderrTailCap = 4096

// stderrTail keeps the last stderrTailCap bytes a child wrote, however much
// it wrote in total.
//
// Written from the goroutine os/exec uses to copy the child's output, read
// from whichever goroutine is asking why the child is gone - concurrently,
// on a node that dies mid-boot while something else is already polling for
// it. Its own lock rather than Native's: the copy goroutine must never wait
// on anything the engine's polling loop might be holding.
type stderrTail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *stderrTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTailCap {
		// The tail is kept, not the head: whatever the child said right
		// before it went is the line that explains it, and the earliest
		// bytes are the ones a bounded buffer has to let go of first.
		t.buf = t.buf[len(t.buf)-stderrTailCap:]
	}
	return len(p), nil
}

func (t *stderrTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
