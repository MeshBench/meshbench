// Package session is the workbench without a user interface.
//
// The store, the verbs, the engine, the firmware, the sweeps, the coverage and
// the control socket: everything a workbench does that is not drawing. It lives
// here rather than in a command so that a front end is a choice rather than a
// commitment - the Gio one, the imgui one, a headless run, or whatever comes
// after them all drive the same session by the same verbs.
//
// The rule that keeps it honest: nothing in this package may import a user
// interface toolkit. If a change wants to, the change belongs on the other side
// of the line.
package session
