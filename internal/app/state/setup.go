// What this machine has, what it has not, and what each one would cost.
//
// A new install could reach every one of these facts and no single place put
// them together: firmware was absent, terrain was a question nobody had
// answered, and the emulator toolchain announced itself only by failing to
// start a node. The rows below are that inventory, in a shape a panel draws and
// the socket carries unchanged.
package state

// SetupState is what one dependency can be, and the five words are chosen so
// that a reader can act on the difference between them.
//
// Missing and Undecided are the two that are not faults. Missing is something
// nothing here needs yet; Undecided is a question the application is waiting on
// rather than answering on somebody's behalf, and it is the state a fresh
// install's terrain preference is in.
type SetupState string

const (
	// SetupReady means it is here and a node could use it now.
	SetupReady SetupState = "ready"
	// SetupNeeded means something in this session cannot run without it.
	SetupNeeded SetupState = "needed"
	// SetupMissing means it is absent and nothing here is blocked on it yet.
	SetupMissing SetupState = "missing"
	// SetupUndecided means the application is waiting to be told, and has
	// deliberately spent nothing in the meantime.
	SetupUndecided SetupState = "undecided"
	// SetupBlocked means it cannot be had on this machine at all, and Do says
	// what to do instead rather than offering a button that would fail.
	SetupBlocked SetupState = "blocked"
)

// SetupRow is one dependency's whole answer.
//
// Cost is stated whether or not anything is going to be fetched, because the
// point of the page is that a size is read before it is spent. Do is plain
// words and stands alone: a row whose only instruction is a button is a row
// that says nothing to somebody reading this over a socket.
type SetupRow struct {
	Name  string
	State string
	// What this dependency is for, in one line.
	What string
	// Cost is what having it costs, in words - "about 59 MB, once" - or empty
	// where it costs nothing.
	Cost string
	// Where it is, when it is here: the path a node would actually use.
	Where string
	// Do is what to do about it, always in words, even when Verb can do it.
	Do string
	// Verb and Params are the one action this row offers, or empty. Rows are
	// rebuilt whole by every check, so the map is never written after it is
	// published.
	Verb   string
	Params map[string]any
}

// SetupGroup is a set of rows with one thing to say about all of them.
//
// Note carries what is true of the group rather than of any row in it - that
// tools are looked for beside the binary and in the cache and never on PATH,
// for one, which is the fact that sends people to a package manager when it is
// missing.
type SetupGroup struct {
	Name string
	Note string
	Rows []SetupRow
}
