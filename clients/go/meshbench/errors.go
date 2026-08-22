// What went wrong, in a shape a caller can branch on.
//
// The workbench answers with a sentence and a code. The sentence is the good
// part - "no node is running firmware, so there is nothing to send to" - and
// it survives untouched; the code is what turns into a Go error a caller can
// match with errors.Is.
package meshbench

import (
	"errors"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// The refusals, one per code. Match with errors.Is.
var (
	// ErrUnknownVerb means this build does not have that method. Nearly
	// always a client older or newer than the workbench - which greet is
	// supposed to have caught first, so seeing this is worth investigating.
	ErrUnknownVerb = errors.New("unknown verb")
	// ErrBadParams is the verb refusing what it was given.
	ErrBadParams = errors.New("bad parameters")
	// ErrNotFound is no node, build, area or job of that name.
	ErrNotFound = errors.New("not found")
	// ErrConflict is the right request in the wrong state: nothing loaded,
	// nothing running, no preview to commit.
	ErrConflict = errors.New("wrong state for that")
	// ErrUnavailable is a request this session cannot serve at all - a window
	// verb with no window, hardware that is not here.
	ErrUnavailable = errors.New("not available in this session")
	// ErrClosing is the workbench shutting down. Retry against a new session
	// rather than report a bug.
	ErrClosing = errors.New("the workbench is closing")
)

// Refused is one verb's refusal: what was asked, what came back, and its kind.
type Refused struct {
	Verb string
	Code control.Code
	// Message is the workbench's own words, unaltered.
	Message string
	kind    error
}

func (r *Refused) Error() string {
	return fmt.Sprintf("%s: %s", r.Verb, r.Message)
}

// Unwrap gives the sentinel, so errors.Is(err, ErrNotFound) works.
func (r *Refused) Unwrap() error { return r.kind }

// ProtocolMismatch is a client and a workbench that cannot speak to each
// other, reported at connect rather than discovered later.
type ProtocolMismatch struct {
	Client    int
	Workbench Hello
}

func (p *ProtocolMismatch) Error() string {
	return fmt.Sprintf(
		"this client speaks protocol %d and the workbench at %s speaks %d (%s). "+
			"Upgrade whichever is older",
		p.Client, p.Workbench.Socket, p.Workbench.Protocol, p.Workbench.Version)
}

// wrap turns a socket error into one of the above, keeping the message.
func wrap(verb string, err error) error {
	if err == nil {
		return nil
	}
	code := control.CodeOf(err)
	kind, ok := kinds[code]
	if !ok {
		// Internal, or a code from a build newer than this client. Neither is
		// something to swallow, and neither has a sentinel worth inventing.
		return &Refused{Verb: verb, Code: code, Message: err.Error(), kind: nil}
	}
	return &Refused{Verb: verb, Code: code, Message: err.Error(), kind: kind}
}

var kinds = map[control.Code]error{
	control.UnknownVerb: ErrUnknownVerb,
	control.BadParams:   ErrBadParams,
	control.NotFound:    ErrNotFound,
	control.Conflict:    ErrConflict,
	control.Unavailable: ErrUnavailable,
	control.Closing:     ErrClosing,
}

// CodeOf reads the workbench's classification off an error, for a caller that
// wants the code rather than the sentinel.
func CodeOf(err error) control.Code {
	var r *Refused
	if errors.As(err, &r) {
		return r.Code
	}
	return control.Unknown
}
