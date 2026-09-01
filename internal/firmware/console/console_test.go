package console_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/console"
)

// errFakeDeadline is what a fakeNode's Read returns once its deadline passes,
// standing in for the timeout error a real file or socket would give back.
var errFakeDeadline = errors.New("fakeNode: read deadline exceeded")

// fakeNode answers a command with canned lines and a prompt, optionally slowly
// or not at all.
//
// SetReadDeadline is the part a real Port gets for free from the operating
// system and this one has to build by hand: a deadline changed while Read is
// already blocked must interrupt that call, not just the next one, which is
// what wake exists for.
type fakeNode struct {
	name  string
	reply []string
	delay time.Duration
	mute  bool

	mu       sync.Mutex
	out      strings.Builder
	in       chan string
	got      []string
	deadline time.Time
	wake     chan struct{}

	// readReturned fires every time Read returns, so a test can confirm the
	// goroutine reading this port actually stopped rather than being left
	// blocked and merely ignored.
	readReturned chan struct{}
}

func newNode(name string, reply ...string) *fakeNode {
	return &fakeNode{
		name:         name,
		reply:        reply,
		in:           make(chan string, 8),
		wake:         make(chan struct{}),
		readReturned: make(chan struct{}, 64),
	}
}

func (f *fakeNode) Name() string { return f.name }

func (f *fakeNode) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	f.deadline = t
	old := f.wake
	f.wake = make(chan struct{})
	f.mu.Unlock()
	close(old)
	return nil
}

func (f *fakeNode) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.got = append(f.got, strings.TrimSpace(string(p)))
	f.mu.Unlock()
	if f.mute {
		return len(p), nil
	}
	go func() {
		time.Sleep(f.delay)
		f.mu.Lock()
		for _, l := range f.reply {
			f.out.WriteString(l + "\n")
		}
		f.out.WriteString(">\n")
		f.mu.Unlock()
		f.in <- "ready"
	}()
	return len(p), nil
}

func (f *fakeNode) Read(p []byte) (int, error) {
	defer func() {
		select {
		case f.readReturned <- struct{}{}:
		default:
		}
	}()
	for {
		f.mu.Lock()
		dl, wake := f.deadline, f.wake
		f.mu.Unlock()

		var after <-chan time.Time
		var timer *time.Timer
		if !dl.IsZero() {
			d := time.Until(dl)
			if d <= 0 {
				return 0, errFakeDeadline
			}
			timer = time.NewTimer(d)
			after = timer.C
		}

		select {
		case <-f.in:
			if timer != nil {
				timer.Stop()
			}
			f.mu.Lock()
			s := f.out.String()
			f.out.Reset()
			f.mu.Unlock()
			if s == "" {
				return 0, io.EOF
			}
			return copy(p, s), nil
		case <-after:
			return 0, errFakeDeadline
		case <-wake:
			// The deadline changed while this call was blocked - recompute
			// and keep waiting rather than returning, the same as a real
			// SetReadDeadline would for a Read already in flight.
			if timer != nil {
				timer.Stop()
			}
		}
	}
}

func (f *fakeNode) Close() error { return nil }

func (f *fakeNode) received() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.got))
	copy(out, f.got)
	return out
}

// Configuring a twenty-node scenario means sending the same line to twenty
// consoles. Doing it in sequence at a two-second timeout is forty seconds for a
// command that takes milliseconds.
func TestBroadcastIsConcurrent(t *testing.T) {
	c := console.New()
	c.Timeout = 2 * time.Second
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		node := newNode(n, "ok")
		node.delay = 200 * time.Millisecond
		if err := c.Attach(node); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	replies := c.Broadcast(context.Background(), "get freq")
	elapsed := time.Since(start)

	if len(replies) != 5 {
		t.Fatalf("got %d replies from 5 nodes", len(replies))
	}
	// Five nodes at 200 ms each: concurrent is ~200 ms, serial is ~1 s.
	if elapsed > 700*time.Millisecond {
		t.Errorf("broadcast took %v; five 200 ms nodes were answered in sequence", elapsed)
	}
}

// A broadcast that reports only its successes makes a half-configured mesh look
// configured, and the node that did not answer is the one that behaves
// unexpectedly later.
func TestSilentNodesAreReportedNotOmitted(t *testing.T) {
	c := console.New()
	c.Timeout = 150 * time.Millisecond

	if err := c.Attach(newNode("talker", "freq 869.525")); err != nil {
		t.Fatal(err)
	}
	silent := newNode("silent")
	silent.mute = true
	if err := c.Attach(silent); err != nil {
		t.Fatal(err)
	}

	replies := c.Broadcast(context.Background(), "get freq")
	if len(replies) != 2 {
		t.Fatalf("got %d replies, want 2 — the silent node was dropped", len(replies))
	}

	var okCount, failCount int
	for _, r := range replies {
		if r.OK() {
			okCount++
		} else {
			failCount++
		}
	}
	if okCount != 1 || failCount != 1 {
		t.Errorf("ok=%d failed=%d, want 1 and 1", okCount, failCount)
	}

	summary := console.Summarise("get freq", replies)
	if !strings.Contains(summary, "did NOT answer") || !strings.Contains(summary, "silent") {
		t.Errorf("the summary hides the unresponsive node:\n%s", summary)
	}
	if !strings.Contains(summary, "1 of 2") {
		t.Errorf("the summary does not count what answered:\n%s", summary)
	}

	// Broadcast has already returned, so the goroutine reading the silent
	// node's port must have too - not been left blocked on Scan behind a
	// command nobody is waiting on any more.
	select {
	case <-silent.readReturned:
	case <-time.After(time.Second):
		t.Fatal("the read behind a timed-out command was never cancelled; its goroutine leaked")
	}
}

// The same leak, reached through Send instead of Broadcast, and through
// context cancellation rather than a timeout - both are ways a caller can stop
// waiting on a node that never answers, and both must free the read rather
// than abandon it.
func TestSendToASilentNodeCancelsTheReadRatherThanLeakingIt(t *testing.T) {
	c := console.New()
	c.Timeout = 10 * time.Second

	silent := newNode("silent")
	silent.mute = true
	if err := c.Attach(silent); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan console.Reply, 1)
	go func() { done <- c.Send(ctx, "silent", "get freq") }()

	// Give the goroutine above time to actually reach the blocked read before
	// cancelling, so this exercises interrupting a read already in flight
	// rather than one that has not started yet.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case r := <-done:
		if r.OK() {
			t.Fatal("a cancelled command was reported as answered")
		}
	case <-time.After(time.Second):
		t.Fatal("Send never returned after its context was cancelled")
	}
	select {
	case <-silent.readReturned:
	case <-time.After(time.Second):
		t.Fatal("cancelling Send did not free the goroutine reading the port; it leaked")
	}
}

// Replies come back sorted, so two runs of the same broadcast are comparable.
// Goroutine completion order is not a useful ordering for anything.
func TestRepliesAreOrderedNotRaced(t *testing.T) {
	c := console.New()
	for i, n := range []string{"delta", "alpha", "charlie", "bravo"} {
		node := newNode(n, "ok")
		node.delay = time.Duration(40-i*10) * time.Millisecond
		if err := c.Attach(node); err != nil {
			t.Fatal(err)
		}
	}
	first := c.Broadcast(context.Background(), "x")
	second := c.Broadcast(context.Background(), "x")

	for i := range first {
		if first[i].Node != second[i].Node {
			t.Fatalf("two identical broadcasts came back in different orders: %v vs %v",
				names(first), names(second))
		}
	}
	if names(first)[0] != "alpha" {
		t.Errorf("replies are not sorted: %v", names(first))
	}
}

// A bare prompt ends a reply. Without that, every command waits the full
// timeout however fast the node actually is.
func TestPromptEndsTheReplyEarly(t *testing.T) {
	c := console.New()
	c.Timeout = 3 * time.Second
	if err := c.Attach(newNode("fast", "line one", "line two")); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	r := c.Send(context.Background(), "fast", "status")
	if !r.OK() {
		t.Fatalf("reply failed: %v", r.Err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("a fast node took %v; the prompt is not ending the reply", r.Took)
	}
	if len(r.Lines) != 2 {
		t.Errorf("got %d lines, want 2: %v", len(r.Lines), r.Lines)
	}
	// The prompt itself is not content.
	for _, l := range r.Lines {
		if strings.TrimSpace(l) == ">" {
			t.Error("the prompt was returned as a reply line")
		}
	}
}

// Two nodes sharing a console name means every command after that goes
// somewhere nobody intended.
func TestDuplicateNamesAreRefused(t *testing.T) {
	c := console.New()
	if err := c.Attach(newNode("node-1")); err != nil {
		t.Fatal(err)
	}
	if err := c.Attach(newNode("node-1")); err == nil {
		t.Fatal("two ports quietly shared a name")
	}
	if err := c.Attach(newNode("")); err == nil {
		t.Fatal("an unnamed port was attached")
	}
}

func TestSendToUnknownNodeIsAnError(t *testing.T) {
	c := console.New()
	r := c.Send(context.Background(), "nobody", "status")
	if r.OK() {
		t.Fatal("a command to an unattached node succeeded")
	}
}

func TestCommandReachesTheNode(t *testing.T) {
	c := console.New()
	n := newNode("target", "ok")
	if err := c.Attach(n); err != nil {
		t.Fatal(err)
	}
	c.Send(context.Background(), "target", "set freq 869.525")
	got := n.received()
	if len(got) != 1 || got[0] != "set freq 869.525" {
		t.Errorf("node received %v", got)
	}
}

func names(rs []console.Reply) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Node
	}
	return out
}
