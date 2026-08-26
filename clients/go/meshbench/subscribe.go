package meshbench

import "github.com/MeshBench/meshbench/internal/app/control"

// Notification is one server-pushed event on a Subscription. Named apart from
// the log's Event, which is a different thing entirely: an Event is a record of
// something that happened in the simulation, a Notification is the socket
// telling this connection about a change as it lands.
type Notification = control.Event

// Subscription is a live stream of notifications, on a connection of its own so
// it never interleaves with this workbench's request/response calls.
type Subscription = control.Subscription

// Subscribe streams the given topics - status, snapshot, and whichever else the
// server publishes - without polling. It opens a second connection to the same
// workbench, so Close on the returned Subscription hangs up only that stream.
//
// Topics known today: "status" (a new console line) and "snapshot" (a compact
// summary after each publish, coalesced by the server so a busy run cannot
// flood a slow reader).
func (w *Workbench) Subscribe(topics ...string) (*Subscription, error) {
	return w.conn.Subscribe(topics...)
}
