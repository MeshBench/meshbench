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
	"github.com/MeshBench/meshbench/internal/app/version"
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
//
// Either end may be the one to notice. When the workbench refuses the
// connection over its declared version, Said carries that refusal and
// Workbench is the zero value, because the workbench stopped the connection
// before it would say what it was.
type ProtocolMismatch struct {
	Client    int
	Workbench Hello
	// Said is the workbench's own refusal, kept whole. Its words are better
	// than a paraphrase: it knows its build and which end is the older one.
	Said string
}

func (p *ProtocolMismatch) Error() string {
	if p.Said != "" {
		return p.Said
	}
	return fmt.Sprintf(
		"this client speaks protocol %d and the workbench at %s speaks %d (%s). "+
			"Upgrade whichever is older",
		p.Client, p.Workbench.Socket, p.Workbench.Protocol, p.Workbench.Version)
}

// VersionMismatch is a released client driving a workbench from a different
// release, reported at connect rather than discovered later.
//
// Distinct from ProtocolMismatch, and from a verb's Refused, because a script
// has to be able to tell "these two were never meant to be used together" from
// "this build declined what I asked". The remedies have nothing in common.
type VersionMismatch struct {
	// Client and Workbench are the releases each end belongs to, as they are
	// spelled on PyPI, npm and the release page.
	Client    string
	Workbench string
	// Said is the workbench's own refusal when it was the end that noticed,
	// kept whole. Empty when this client noticed first, which happens against
	// a build old enough to ignore what the client declared.
	Said string
}

func (v *VersionMismatch) Error() string {
	if v.Said != "" {
		return v.Said
	}
	return fmt.Sprintf(
		"this client is from MeshBench %s and this workbench is MeshBench %s. "+
			"A client and the workbench it drives must be the same release: "+
			"install the %s client, or run the %s workbench",
		v.Client, v.Workbench, v.Workbench, v.Client)
}

// asMismatch turns a workbench's refusal of this client's declared wire
// version into the typed mismatch, and leaves every other failure as it was.
//
// The declaration is refused before any verb runs, so what comes back is a
// refusal wearing the name of whichever verb was on the frame. Reported as
// what it is instead: a verb failure standing in for a version disagreement is
// the confusion the declaration exists to end.
func asMismatch(err error) error {
	var refused *Refused
	if !errors.As(err, &refused) {
		return fmt.Errorf("client: %w", err)
	}
	switch refused.Code {
	case control.ProtocolMismatch:
		return &ProtocolMismatch{Client: control.Protocol, Said: refused.Message}
	case control.VersionMismatch:
		return &VersionMismatch{Client: version.Release(), Said: refused.Message}
	}
	return fmt.Errorf("client: %w", err)
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
