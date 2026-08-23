// What kind of failure a verb reported, as a small closed set.
//
// A client that has to tell "no node named X" from "the workbench is closing"
// was matching prose written in two different files by two different authors.
// The prose stays - it is good prose, and it is what a person reads - and the
// code sits beside it for the thing reading it programmatically.
//
// Deliberately closed and deliberately short. A taxonomy is a second thing to
// keep correct; seven answers cover every refusal in the tree, and anything
// that does not fit is Internal, which is honest.
package control

import "errors"

// Code classifies a failed request.
type Code string

const (
	// Unknown is no code at all: a success, or an error raised before
	// anything classified it.
	Unknown Code = ""
	// UnknownVerb is a method this build does not have. Nearly always a
	// client newer or older than the workbench, which is what session.hello
	// exists to catch first.
	UnknownVerb Code = "unknown_verb"
	// BadParams is a verb refusing what it was given.
	BadParams Code = "bad_params"
	// NotFound is no node, build, area or job of that name.
	NotFound Code = "not_found"
	// Conflict is the right request in the wrong state: no simulation loaded,
	// nothing running to send to, no import preview to commit.
	Conflict Code = "conflict"
	// Unavailable is a request this session cannot serve at all - a window
	// verb with no window, hardware that is not here.
	Unavailable Code = "unavailable"
	// Internal is a fault rather than a refusal.
	Internal Code = "internal"
	// Closing is the workbench shutting down. Separate from Internal because
	// a client should retry against a new session rather than report a bug.
	Closing Code = "closing"
	// Unauthorised is a loopback connection that did not present the token.
	//
	// Only reachable on the TCP transport, where the port is open to any local
	// process and the token is what a unix socket's permissions would have
	// done. A client seeing this is reading the wrong address file, or none.
	Unauthorised Code = "unauthorised"
)

// Coded is an error that knows what kind it is.
//
// Verbs are free to return a plain error and usually do; this is for the ones
// whose kind a caller genuinely acts on differently.
type Coded struct {
	Code Code
	Err  error
}

func (c *Coded) Error() string { return c.Err.Error() }
func (c *Coded) Unwrap() error { return c.Err }

// WithCode labels an error. Nil in, nil out, so a caller can wrap
// unconditionally at a return site.
func WithCode(code Code, err error) error {
	if err == nil {
		return nil
	}
	return &Coded{Code: code, Err: err}
}

// CodeOf reads the code off an error, or classifies it.
//
// Classification is by errors.Is against the sentinels a caller registers, not
// by reading the message: a code inferred from prose would break the moment
// somebody improved the prose, which is exactly the coupling this replaces.
func CodeOf(err error) Code {
	if err == nil {
		return Unknown
	}
	var c *Coded
	if errors.As(err, &c) {
		return c.Code
	}
	for _, k := range known {
		if errors.Is(err, k.err) {
			return k.code
		}
	}
	return Internal
}

type sentinel struct {
	err  error
	code Code
}

var known []sentinel

// Classify registers a sentinel error as meaning one code.
//
// Here rather than in the packages that own those errors, because control is
// the bottom of the tree and cannot import upwards. The layering rule is the
// reason for the indirection, not taste.
func Classify(err error, code Code) {
	known = append(known, sentinel{err: err, code: code})
}
