package control

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// A subscribed connection is told what changed; a plain one is not. The two
// facts are the whole contract: session.subscribe turns notifications on, and
// nothing turns them on by accident.

func TestSubscribeReceivesNotification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sock")
	srv := serveAt(t, path)

	sub, err := Subscribe(path, "status")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	srv.Notify("status", map[string]any{"line": "radio ok"})

	select {
	case e := <-sub.Events():
		if e.Topic != "status" {
			t.Fatalf("topic = %q, want status", e.Topic)
		}
		var d struct {
			Line string `json:"line"`
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			t.Fatal(err)
		}
		if d.Line != "radio ok" {
			t.Fatalf("line = %q", d.Line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a subscriber was told nothing")
	}
}

// A topic nobody asked for is not delivered, and a Call on a subscribed
// connection is not derailed by it either - the two streams stay separate.
func TestNotifyOnlyGoesToInterestedSubscribers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	srv := serveAt(t, path)

	sub, err := Subscribe(path, "status")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	srv.Notify("packet", map[string]any{"id": 7}) // not subscribed
	srv.Notify("status", map[string]any{"line": "wanted"})

	select {
	case e := <-sub.Events():
		if e.Topic != "status" {
			t.Fatalf("got an unsubscribed topic %q", e.Topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the wanted topic never arrived")
	}
}

// A plain Client that never subscribes reads exactly its replies and no
// notification ever appears mid-stream. This is the backward-compatibility the
// change turns on: an old script sees no difference.
func TestPlainClientSeesNoNotifications(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	srv := serveAt(t, path)

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	// Push while the plain client is idle; a broken demux would leave this
	// on the wire for the next Call to trip over.
	srv.Notify("status", map[string]any{"line": "ignored"})
	time.Sleep(20 * time.Millisecond)

	raw, err := c.Call("who", nil)
	if err != nil {
		t.Fatalf("call after a notify was pushed: %v", err)
	}
	var got struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("the reply was not the reply: %v", err)
	}
	if got.Path != path {
		t.Fatalf("path = %q, want %q", got.Path, path)
	}
}

// The snapshot topic is coalesced: many published in a window collapse to one,
// and it carries the count that were dropped so a client knows it missed them.
func TestSnapshotCoalesces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	srv := serveAt(t, path)

	// A clock the test moves by hand, so the window is not a race.
	now := time.Unix(0, 0)
	srv.mu.Lock()
	srv.clock = func() time.Time { return now }
	srv.mu.Unlock()

	sub, err := Subscribe(path, "snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sub.Close() }()

	// Five in the same instant: the first goes, four are dropped.
	for i := 0; i < 5; i++ {
		srv.Notify("snapshot", map[string]any{"seq": i})
	}
	first := recv(t, sub)
	if first.Dropped != 0 {
		t.Fatalf("first snapshot dropped = %d, want 0", first.Dropped)
	}

	// Past the window: the next one goes and reports the four it stood in for.
	now = now.Add(snapshotInterval + time.Millisecond)
	srv.Notify("snapshot", map[string]any{"seq": 99})
	second := recv(t, sub)
	if second.Dropped != 4 {
		t.Fatalf("second snapshot dropped = %d, want 4", second.Dropped)
	}
}

// A subscriber whose connection dies while a request is still queued must
// not leave the reader parked forever on a full notification buffer: it is
// the reader's defer chain that removes the entry from s.subs, so a shrunken
// map is direct evidence the goroutine actually returned rather than being
// stuck for the life of the process.
func TestAKilledSubscriberLeavesNoSubscriptionBehind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "killed.sock")
	srv, err := ListenAt(path, func(string, json.RawMessage) (any, error) {
		return map[string]any{"ok": true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	c, err := DialAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call(SubscribeMethod, map[string]any{"topics": []string{"status"}}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Fill the writer's own buffer, which is what a dead writer needs before
	// the send in serve() would have blocked rather than noticed.
	for i := 0; i < 128; i++ {
		srv.Notify("status", map[string]any{"seq": i})
	}

	// A request queued and never answered before the connection dies -
	// Pump has deliberately not run yet.
	go func() { _, _ = c.Call("who", nil) }()
	time.Sleep(20 * time.Millisecond) // give it time to reach the queue
	_ = c.Close()                     // the client is killed mid-request

	// Only now does the workbench notice: Pump answers the queued job, and
	// the connection's reader and writer should unwind on their own.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		srv.Pump()
		srv.mu.Lock()
		n := len(srv.subs)
		srv.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	srv.mu.Lock()
	n := len(srv.subs)
	srv.mu.Unlock()
	t.Fatalf("a killed subscriber is still in s.subs: %d entries", n)
}

func recv(t *testing.T, sub *Subscription) Event {
	t.Helper()
	select {
	case e := <-sub.Events():
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification")
		return Event{}
	}
}

// The acknowledgement must be the first frame on a new subscription even when
// the server is publishing hard: register-before-ack would let a notification
// slip in front and be read as the subscribe reply. Subscribing many times
// under a flood is how that ordering bug shows itself, if it is there.
func TestSubscribeAckIsNotOvertakenByNotifications(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.sock")
	srv := serveAt(t, path)

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				srv.Notify("status", map[string]any{"line": "busy"})
			}
		}
	}()
	defer close(stop)

	for i := 0; i < 50; i++ {
		sub, err := Subscribe(path, "status")
		if err != nil {
			t.Fatalf("subscribe %d under a flood: %v", i, err)
		}
		// The first event, if any, must be a real notification and never the
		// ack leaking through as an empty-topic frame.
		select {
		case e := <-sub.Events():
			if e.Topic != "status" {
				t.Fatalf("first frame had topic %q - the ack was read as an event", e.Topic)
			}
		case <-time.After(200 * time.Millisecond):
		}
		_ = sub.Close()
	}
}
