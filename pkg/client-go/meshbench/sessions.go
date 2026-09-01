// Finding the workbenches that are running, when there is more than one.
//
// Attach goes to one address, which is enough while there is one session per
// user. Two runs side by side - a soak beside the workbench somebody is
// watching, two jobs on one CI runner - need a second address, and until this
// existed the only record of it was in the head of whoever typed it.
//
// Sessions is deliberately a package function and not a method. The question
// comes *before* a connection: a script asks what is running in order to
// decide what to attach to.
package meshbench

import (
	"context"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// Session is one running workbench: where it answers, what process it is,
// when it started, and what it has open. Snapshot, read once.
//
// The description - version, mode, project, node count - is asked of the
// session while the list is being built, so it is what was true a moment ago
// rather than what was true when the session started. It is empty for a
// session too busy to answer in the moment it was asked; the session is still
// listed, because it is still running.
type Session = control.Session

// Sessions lists the workbenches running on this machine, oldest first.
//
// A session that has died is not listed, however it died. The check is a dial
// of the address itself rather than a look at a socket file or a pid: a unix
// socket outlives the process that bound it, and a pid is reused, so both can
// name something that is not there.
func Sessions() ([]Session, error) { return control.Sessions("") }

// AttachTo connects to a session from Sessions, rather than to an address
// typed out again.
//
// It exists because a TCP session cannot be reattached to by address alone:
// its token is in its own file and not in the per-user rendezvous file, which
// two of them share. The row carries the token, so this works where
// Attach(ctx, Socket(row.Address)) would reach the wrong session or none.
//
// Deliberately no "attach to whatever is running": where several are up and
// none was named, guessing is how a script ends up driving the session
// somebody else was watching. Ask Sessions, choose, and say which.
func AttachTo(ctx context.Context, s Session) (*Workbench, error) {
	conn, err := s.Dial()
	if err != nil {
		return nil, err
	}
	wb := &Workbench{conn: conn}
	return wb, wb.greet(ctx)
}

// Sessions asks this workbench what else is running beside it, for a script
// that is already attached to one.
//
// The same list Sessions returns, with two differences: the row for this
// session has Self set and is described from the inside rather than by being
// asked over its own socket, and no row carries a token - a secret that
// belongs in the 0600 file it came from, not in a reply. So the rows are for
// choosing by; AttachTo wants one from Sessions.
func (w *Workbench) Sessions(ctx context.Context) ([]Session, error) {
	var out struct {
		Sessions []Session `json:"sessions"`
	}
	return out.Sessions, w.CallInto(ctx, "session.list", nil, &out)
}
