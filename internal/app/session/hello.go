// What a client is talking to, asked once before anything else.
//
// A connection could not previously find out. No version, no protocol number,
// no way to tell a windowed session from a headless one - a client discovered
// all of it by calling a verb and reading prose back, which means a client
// older or newer than the build fails halfway through a script rather than at
// the door. In a CI run that reads as a firmware regression.
package session

import (
	"os"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// Hello is the answer: enough for a client to decide whether to carry on.
type Hello struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	// Mode is "workbench" or "headless". A script checks it before touching
	// anything that needs a window, and gets a sentence from its own client
	// rather than a refusal from twelve verbs in a row.
	Mode string `json:"mode"`
	// Socket is the path being answered on, which is no longer one per user.
	Socket string `json:"socket"`
	Verbs  int    `json:"verbs"`
	// PID and StartedAt are how a reconnecting script tells a restart from a
	// reconnect. The scenario does not survive a restart - nodes, boundary,
	// inference and firmware assignments live in the process - and the first
	// ScotMesh study ran its inference against a workbench that had been
	// rebuilt after the import, with nothing in the state to say so.
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	// Project and Nodes are what this session is running, which is how
	// somebody looking at a list of them tells one from another. Asked of the
	// session rather than written down anywhere, because both move: a fixture
	// is opened, nodes are added, and a note of them made when the session
	// started would be a confident answer that had stopped being true.
	Project string `json:"project"`
	Nodes   int    `json:"nodes"`
}

// startedAt is when this process began answering. Taken once at init rather
// than at first call, so a client that connects late still learns when the
// session started rather than when it happened to ask.
var startedAt = time.Now()

// Mode is what kind of session this is, set by whoever builds it.
//
// A variable rather than a parameter threaded through ServeControl, because
// the alternative was every caller of every constructor learning about it.
// Written once at startup and read from the socket's goroutine; the race
// detector is satisfied because the write happens before Listen.
var Mode = "workbench"

// hello answers session.hello.
//
// The snapshot rather than the world: this is answered on the socket's own
// goroutine, and a session listing the others has a connection of its own open
// waiting for the reply. Going through the store for it would have that reply
// queue behind whatever the store is already doing.
func hello(verbs []string, socket string, snap *state.Snapshot) Hello {
	h := Hello{
		Protocol:  control.Protocol,
		Version:   version.Detail(),
		Mode:      Mode,
		Socket:    socket,
		Verbs:     len(verbs),
		PID:       os.Getpid(),
		StartedAt: startedAt,
	}
	if snap != nil {
		h.Project, h.Nodes = snap.Project, len(snap.Nodes)
	}
	return h
}

// sortedVerbs is the list, in an order a person can scan.
func sortedVerbs(v []string) []string {
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}
