package control

import (
	"encoding/json"
	"net"
	"time"
)

// A subscription turns the request/response socket into one a script can also
// be told things on. After session.subscribe, the server may write notification
// lines on that connection - {"event": ..., "data": ...} with no id, so a
// client that never subscribes sees no change and one that does can tell a
// notification from a reply by construction rather than by guessing.

// SubscribeMethod is the request that opens a subscription on a connection.
const SubscribeMethod = "session.subscribe"

// snapshotInterval coalesces the snapshot topic: a run publishes one per tick,
// which on a large network would be a denial of service against the client, so
// at most one snapshot notification goes to a connection this often. The count
// that were dropped in between rides along on the next one.
const snapshotInterval = 200 * time.Millisecond

// notification is what a subscriber is sent. The absent id is the whole point:
// a reply always carries the id it answered, a notification never does.
type notification struct {
	Event   string `json:"event"`
	Data    any    `json:"data"`
	Dropped int    `json:"dropped,omitempty"`
}

// subscriber is one connection's interest: the writer channel its frames go to,
// the topics it asked for, and the snapshot-coalescing state. lastSnap/dropped
// are touched only by Notify, which runs on one goroutine, so they need no lock.
type subscriber struct {
	out      chan any
	topics   map[string]bool
	lastSnap time.Time
	dropped  int
}

// subscribe registers a connection's interest and returns the handle serve
// unregisters with when the connection goes.
func (s *Server) subscribe(out chan any, topics []string) *subscriber {
	sub := &subscriber{out: out, topics: topicSet(topics)}
	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[*subscriber]struct{}{}
	}
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	return sub
}

// setTopics changes an existing subscription's interest - a second
// session.subscribe on the same connection replaces the set rather than adding
// a second registration.
func (s *Server) setTopics(sub *subscriber, topics []string) {
	s.mu.Lock()
	sub.topics = topicSet(topics)
	s.mu.Unlock()
}

func (s *Server) unsubscribe(sub *subscriber) {
	s.mu.Lock()
	delete(s.subs, sub)
	s.mu.Unlock()
}

func topicSet(topics []string) map[string]bool {
	set := make(map[string]bool, len(topics))
	for _, t := range topics {
		set[t] = true
	}
	return set
}

// Notify fans one event out to the connections that asked for its topic. Called
// on the store's goroutine, after a publish. The send is non-blocking: a client
// that has fallen behind its own buffer is dropped from rather than allowed to
// stall the workbench, and the snapshot topic is coalesced to snapshotInterval.
func (s *Server) Notify(topic string, data any) {
	s.mu.Lock()
	subs := make([]*subscriber, 0, len(s.subs))
	for sub := range s.subs {
		if sub.topics[topic] {
			subs = append(subs, sub)
		}
	}
	s.mu.Unlock()

	for _, sub := range subs {
		n := notification{Event: topic, Data: data}
		if topic == "snapshot" {
			now := s.now()
			if now.Sub(sub.lastSnap) < snapshotInterval {
				sub.dropped++
				continue
			}
			sub.lastSnap = now
			n.Dropped, sub.dropped = sub.dropped, 0
		}
		select {
		case sub.out <- n:
		default: // the connection's writer is behind; skip rather than block.
		}
	}
}

// now is time.Now, made a field only so a test can pin it.
func (s *Server) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

// parseTopics reads the topics array from a session.subscribe request.
func parseTopics(raw json.RawMessage) []string {
	var p struct {
		Topics []string `json:"topics"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.Topics
}

// --- client side ---

// Event is one server-pushed notification a Subscription delivers. Data is the
// event's payload, left raw so the caller unmarshals it into whatever the topic
// carries. Dropped is how many snapshot notifications were coalesced away before
// this one, zero for every other topic.
type Event struct {
	Topic   string
	Data    json.RawMessage
	Dropped int
}

// Subscription is a read-only stream of notifications on a connection of its
// own. It is deliberately not a Client: a Call decodes exactly one reply per
// request, and a stream of events the server sends unbidden does not fit that,
// so a subscription is handed a connection it alone reads.
type Subscription struct {
	conn   net.Conn
	events chan Event
	err    error
	done   chan struct{}
}

// wireNote is a notification as it arrives, with Data kept raw for the caller.
type wireNote struct {
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
	Dropped int             `json:"dropped"`
}

// Subscribe opens a connection, asks to be told about the given topics, and
// streams what arrives. The acknowledgement is read before the reader starts,
// so a caller that gets a Subscription back knows the server accepted it.
func Subscribe(want string, topics ...string) (*Subscription, error) {
	addr, err := Resolve(want)
	if err != nil {
		return nil, err
	}
	return subscribeOn(addr, topics)
}

// Subscribe opens a second connection to the same workbench this client is
// talking to and streams notifications on it, leaving this client's
// request/response stream untouched. The token, for a TCP address, comes along
// because it is carried on the address rather than looked up again.
func (c *Client) Subscribe(topics ...string) (*Subscription, error) {
	return subscribeOn(c.addr, topics)
}

func subscribeOn(addr Address, topics []string) (*Subscription, error) {
	c, err := dialAddr(addr)
	if err != nil {
		return nil, err
	}
	if _, err := c.Call(SubscribeMethod, map[string]any{"topics": topics}); err != nil {
		_ = c.Close()
		return nil, err
	}
	sub := &Subscription{conn: c.conn, events: make(chan Event, 64), done: make(chan struct{})}
	go sub.read(c.dec)
	return sub, nil
}

func (s *Subscription) read(dec *json.Decoder) {
	defer close(s.events)
	for {
		var n wireNote
		if err := dec.Decode(&n); err != nil {
			select {
			case <-s.done: // Close was called; a read error is expected, not news.
			default:
				s.err = err
			}
			return
		}
		s.events <- Event{Topic: n.Event, Data: n.Data, Dropped: n.Dropped}
	}
}

// Events is the stream. It is closed when the connection ends; call Err after
// to tell a clean close from a broken one.
func (s *Subscription) Events() <-chan Event { return s.events }

// Err reports why the stream ended, nil after a Close.
func (s *Subscription) Err() error { return s.err }

// Close hangs up. The reader sees the resulting error but reports nil from Err,
// because a close the caller asked for is not a failure.
func (s *Subscription) Close() error {
	close(s.done)
	return s.conn.Close()
}
