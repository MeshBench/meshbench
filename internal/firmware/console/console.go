// Package console is the operator's terminal onto running nodes.
//
// A MeshCore node has a serial console: a line-based command interface that is
// how anyone configures a real repeater. A simulator that cannot reach it can
// build a mesh but cannot administer one, so the console is not a debugging
// convenience — it is how a scenario gets set up in the first place.
//
// Two things here that a naive implementation gets wrong.
//
// Commands to many nodes at once are the normal case, not a power feature. A
// twenty-node scenario is configured by sending the same line to twenty
// consoles, and doing that one at a time is the difference between a usable
// tool and a tedious one.
//
// And a broadcast is not atomic. Some nodes will answer, some will time out,
// and reporting only the successes makes a half-configured mesh look configured.
package console

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Port is one node's serial console.
//
// An interface because a console reaches an emulated node through a PTY, a
// native node through a pipe, and a test through neither.
//
// SetReadDeadline is what lets a timed-out or cancelled command free the
// goroutine reading it rather than abandon that goroutine on a node that may
// never answer. Every real implementation already has one for free: a PTY and
// a pipe both wrap an *os.File, which has supported read deadlines since Go
// 1.10, and a deadline changed while a Read is already blocked takes effect
// on that call, not just the next one - the same guarantee net.Conn makes.
type Port interface {
	Name() string
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
}

// Reply is what one node said.
type Reply struct {
	Node string
	// Lines are what came back before the prompt or the deadline.
	Lines []string
	// Err is set when the node did not answer, or answered unusably. A node
	// that timed out is a result, not an absence: it is running and not
	// responding, which is worth knowing.
	Err error
	// Took is how long it took, kept because a node that answers slowly is
	// usually about to stop answering.
	Took time.Duration
}

// OK reports whether the node answered.
func (r Reply) OK() bool { return r.Err == nil }

// Console multiplexes several node ports.
type Console struct {
	mu    sync.Mutex
	ports map[string]Port
	order []string

	// Timeout is how long to wait for a node to finish answering.
	Timeout time.Duration
}

func New() *Console {
	return &Console{ports: map[string]Port{}, Timeout: 2 * time.Second}
}

// Attach adds a node's port. A duplicate name is an error rather than a silent
// replacement — two nodes sharing a console name means every command after that
// goes somewhere nobody intended.
func (c *Console) Attach(p Port) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := p.Name()
	if name == "" {
		return errors.New("console: a port needs a name")
	}
	if _, exists := c.ports[name]; exists {
		return fmt.Errorf("console: %q is already attached", name)
	}
	c.ports[name] = p
	c.order = append(c.order, name)
	sort.Strings(c.order)
	return nil
}

// Detach removes a port and closes it.
func (c *Console) Detach(name string) error {
	c.mu.Lock()
	p, ok := c.ports[name]
	if ok {
		delete(c.ports, name)
		for i, n := range c.order {
			if n == name {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("console: %q is not attached", name)
	}
	return p.Close()
}

// Nodes lists what is attached.
func (c *Console) Nodes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.order))
	copy(out, c.order)
	return out
}

// Send runs one command on one node.
func (c *Console) Send(ctx context.Context, node, cmd string) Reply {
	c.mu.Lock()
	p, ok := c.ports[node]
	timeout := c.Timeout
	c.mu.Unlock()
	if !ok {
		return Reply{Node: node, Err: fmt.Errorf("console: %q is not attached", node)}
	}
	return exchange(ctx, p, cmd, timeout)
}

// Broadcast runs one command on every node, concurrently.
//
// Concurrently because twenty nodes answering in sequence at a two-second
// timeout is forty seconds of waiting for a command that takes milliseconds.
// The replies come back sorted by node so two runs of the same broadcast are
// comparable — goroutine completion order is not a useful ordering for anything.
func (c *Console) Broadcast(ctx context.Context, cmd string) []Reply {
	c.mu.Lock()
	targets := make([]Port, 0, len(c.ports))
	for _, n := range c.order {
		targets = append(targets, c.ports[n])
	}
	timeout := c.Timeout
	c.mu.Unlock()

	replies := make([]Reply, len(targets))
	var wg sync.WaitGroup
	for i, p := range targets {
		wg.Add(1)
		go func(i int, p Port) {
			defer wg.Done()
			replies[i] = exchange(ctx, p, cmd, timeout)
		}(i, p)
	}
	wg.Wait()

	sort.Slice(replies, func(a, b int) bool { return replies[a].Node < replies[b].Node })
	return replies
}

// outcome is what the goroutine reading a port hands back. A channel rather
// than a Reply the goroutine appends to directly: a line arriving as the
// timeout fires was a write to a Reply the caller was already reading, with
// no synchronisation between the two - a data race the caller never saw
// coming because it looked, from here, like an ordinary read.
type outcome struct {
	lines []string
	err   error
}

func exchange(ctx context.Context, p Port, cmd string, timeout time.Duration) Reply {
	start := time.Now()
	r := Reply{Node: p.Name()}

	if _, err := io.WriteString(p, cmd+"\n"); err != nil {
		r.Err = fmt.Errorf("write: %w", err)
		r.Took = time.Since(start)
		return r
	}

	// The read gets its own deadline instead of relying on whoever is waiting
	// to notice a timeout and walk away. Without it, a node that never answers
	// left the goroutine below blocked on Scan for good, once per command sent
	// to it - Reply.Err's own documentation treats a silent node as routine,
	// which made this a leak on the ordinary path rather than an edge case.
	deadline := start.Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	// Whether the deadline took decides how this function may wait below. A
	// port that cannot carry one is not hypothetical: os.File returns
	// ErrNoDeadline for anything the runtime poller will not take, so this has
	// to keep working rather than assume.
	deadlined := p.SetReadDeadline(deadline) == nil

	done := readReply(p)

	// Bounded here only when nothing will interrupt the read, because a nil
	// channel never fires. Waiting on both would double the timeout's meaning.
	var expired <-chan time.Time
	if !deadlined {
		t := time.NewTimer(timeout)
		defer t.Stop()
		expired = t.C
	}

	select {
	case o := <-done:
		r.Lines = o.lines
		if o.err != nil {
			// A timeout is a result, not an absence. The node is running and
			// not answering, and any lines it did manage are kept.
			r.Err = fmt.Errorf("no prompt within %v: %w", timeout, o.err)
		}
	case <-expired:
		// The read is still blocked and cannot be freed, so the goroutine
		// behind it outlives this call. Reported the same way regardless, so a
		// caller cannot tell the two ports apart by their answers.
		r.Err = fmt.Errorf("no prompt within %v", timeout)
	case <-ctx.Done():
		r.Err = ctx.Err()
		// Cancelled rather than abandoned: freeing the read is what lets the
		// goroutine finish and this function return the moment the caller
		// gives up, instead of leaving one running behind every command whose
		// caller stopped waiting. Only safe to wait for when the deadline took;
		// otherwise nothing would ever end that read and this would hang.
		if deadlined {
			_ = p.SetReadDeadline(time.Now())
			<-done
		}
	}
	r.Took = time.Since(start)
	return r
}

// readReply reads one reply off a port, ending at the bare prompt.
//
// Buffered by one so the goroutine can always deliver and exit, even when
// nobody is left to receive - which is what stops a cancelled or timed-out
// command leaking the goroutine as well as the read.
func readReply(p Port) <-chan outcome {
	done := make(chan outcome, 1)
	go func() {
		var lines []string
		sc := bufio.NewScanner(p)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			// A bare prompt ends the reply. Waiting for the deadline on every
			// command instead would make a twenty-node broadcast take the
			// timeout regardless of how fast the nodes actually are.
			if line == ">" || line == "> " {
				done <- outcome{lines: lines}
				return
			}
			lines = append(lines, line)
		}
		done <- outcome{lines: lines, err: sc.Err()}
	}()
	return done
}

// Summarise reports a broadcast in the terms that matter.
//
// Failures first and counted. A broadcast that reports only its successes makes
// a half-configured mesh look configured, and the node that did not answer is
// the one that will behave unexpectedly later.
func Summarise(cmd string, replies []Reply) string {
	var ok, failed []Reply
	for _, r := range replies {
		if r.OK() {
			ok = append(ok, r)
		} else {
			failed = append(failed, r)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%q: %d of %d nodes answered", cmd, len(ok), len(replies))
	if len(failed) > 0 {
		fmt.Fprintf(&b, "\n\n%d did NOT answer — they are running and unresponsive, "+
			"so anything configured by this command is not configured on them:", len(failed))
		for _, r := range failed {
			fmt.Fprintf(&b, "\n  %-16s %v", r.Node, r.Err)
		}
	}
	if len(ok) > 0 {
		b.WriteString("\n")
		for _, r := range ok {
			fmt.Fprintf(&b, "\n  %-16s %s", r.Node, strings.Join(r.Lines, " | "))
		}
	}
	return b.String()
}
