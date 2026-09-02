package state

// What the last update check found, if one was ever made.
//
// Kept flat and in strings because it crosses the socket unchanged and is drawn
// unchanged: a script asking "is there a newer release" and a row on the Setup
// page are the same question, and the answer should not have two shapes.

// Update is the last answer about whether a newer release exists.
//
// Every field is empty until something asks. That is deliberate: nothing here
// is filled at startup, because a check nobody asked for is the thing this was
// designed not to be.
type Update struct {
	// Latest is the newest published release, plain X.Y.Z, and Tag is what the
	// release page calls it.
	Latest string
	Tag    string
	// Notes is the release page. Prose outgrows a panel, so it is linked.
	Notes string
	// Published and Checked are RFC3339, as the feed said it and as the clock
	// said it.
	Published string
	Checked   string
	// Asset is the file this machine would take, Bytes what it costs, and
	// Artefact which bundle this build is - because what a download can
	// honestly do afterwards differs per bundle.
	Asset    string
	Bytes    int64
	Artefact string
	// Why is the reason there is no asset to offer: a platform nothing is
	// published for, or a build the package manager owns.
	Why string
	// Staged is where a verified download landed, empty until one has. It is
	// beside this build and never on top of it.
	Staged string
	// Feed names where the check asked when that was not the published release
	// feed. A check pointed somewhere else has to say so every time it
	// answers, or nobody finds out until it matters.
	Feed string
	// Allowed and Asked are the three-state answer to whether this machine
	// may ask at all: allowed, refused, and never asked, which is where a
	// fresh install starts and where it spends nothing.
	Allowed bool
	Asked   bool
	// Err is why the last check could not answer. Held rather than discarded:
	// a machine with no network is a normal thing to be, and "could not ask"
	// and "nothing newer" are different answers.
	Err string
}
