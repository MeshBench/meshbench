package updates

import (
	"os"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/update"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// The Setup page's row for this build's own version.
//
// It lives beside the verbs rather than with the rest of the readiness check
// because the words are the same words: the row, the status line and the
// socket's answer are three readings of one state, and the moment they are
// written twice one of them starts being more optimistic than the other.
//
// One action at a time, progressing: a question to answer, then a check to
// make, then a download, then where it went. The Setup row carries a single
// verb by design - a row that offered three buttons would be a wizard, and this
// page is deliberately a report.

// SetupRow is what the readiness check shows about this build's version.
func SetupRow(u state.Update, allowed, asked bool) state.SetupRow {
	row := state.SetupRow{
		Name: "version",
		What: "which release this is, and whether a newer one exists. A " +
			"release pins the emulator toolchain, the per-role firmware tags " +
			"and the fixtures, so an old build fetching today's published " +
			"firmware is a combination nobody tested",
	}
	if version.Release() == "" {
		return workingCopyRow(row)
	}
	switch {
	case !asked:
		return undecidedRow(row)
	case !allowed:
		return refusedRow(row)
	case u.Checked == "":
		return uncheckedRow(row)
	case u.Err != "":
		return failedRow(row, u)
	case available(u):
		return offerRow(row, u)
	default:
		return currentRow(row, u)
	}
}

func workingCopyRow(row state.SetupRow) state.SetupRow {
	row.State = string(state.SetupReady)
	row.Do = "this build is " + version.String() + ", which is a working copy " +
		"rather than a release. It is not behind anything, so nothing here " +
		"checks: update checks apply to a tagged build."
	return row
}

func undecidedRow(row state.SetupRow) state.SetupRow {
	row.State = string(state.SetupUndecided)
	row.Do = "nothing has asked whether a newer release exists, and nothing " +
		"will until this is answered. Allowed, the release page is asked once " +
		"a day, in the background, and never as a condition of the application " +
		"opening; refused, it is never asked again and nothing here mentions " +
		"it. No download happens either way without being asked for."
	row.Cost = "a few kilobytes a day"
	row.Verb, row.Params = "update.allow", map[string]any{"on": true}
	return row
}

func refusedRow(row state.SetupRow) state.SetupRow {
	row.State = string(state.SetupReady)
	row.Do = "update checks are off, so this build will not be told a newer " +
		"release exists. Releases are on " + releasesPage + " when you want to " +
		"look, and the switch is also in Configuration > System."
	row.Verb, row.Params = "update.allow", map[string]any{"on": true}
	return row
}

func uncheckedRow(row state.SetupRow) state.SetupRow {
	row.State = string(state.SetupReady)
	row.Do = "checks are allowed and none has answered yet. It happens once a " +
		"day in the background; this asks now."
	row.Verb = "update.check"
	return row
}

func failedRow(row state.SetupRow, u state.Update) state.SetupRow {
	// Not a fault of this machine's setup, so not a warning: a machine with no
	// network is a normal thing to be, and a red row would teach somebody that
	// red means nothing.
	row.State = string(state.SetupReady)
	row.Do = "the release page could not be reached, so whether a newer " +
		"release exists is simply unknown: " + u.Err + ". Nothing is wrong " +
		"with this build."
	row.Verb = "update.check"
	return row
}

func offerRow(row state.SetupRow, u state.Update) state.SetupRow {
	row.State = string(state.SetupUndecided)
	row.Cost = resource.SIBytes(u.Bytes) + " to download"
	if u.Staged != "" {
		row.State = string(state.SetupReady)
		row.Where = u.Staged
		exe, err := os.Executable()
		if err != nil {
			exe = ""
		}
		row.Do = "MeshBench " + u.Latest + " is downloaded and checked, and " +
			"nothing has been replaced. " +
			update.Swap(update.Artefact(u.Artefact), u.Staged, exe) + " " +
			clientWords(u.Latest) + " Release notes: " + u.Notes
		row.Verb = "update.reveal"
		return row
	}
	row.Do = "MeshBench " + u.Latest + " is out" + releasedWhen(u) + " and this " +
		"build is " + version.Release() + ". Downloading does not replace " +
		"anything: it lands beside this build, checked against the release's " +
		"own SHA256SUMS, and the row then says how to swap it. " +
		clientWords(u.Latest) + " Release notes: " + u.Notes
	row.Verb = "update.download"
	return row
}

func currentRow(row state.SetupRow, u state.Update) state.SetupRow {
	row.State = string(state.SetupReady)
	switch {
	case u.Why != "":
		// A newer release that this machine cannot take from here - a package
		// manager's copy, or a platform nothing is published for. Said rather
		// than hidden: the release still exists.
		row.Do = "MeshBench " + u.Latest + " is out and this build is " +
			version.Release() + ", but " + u.Why + ". Release notes: " + u.Notes
	default:
		row.Do = "this is the newest release. Checked " + u.Checked + "."
	}
	row.Verb = "update.check"
	return row
}

// releasesPage is where a person looks when the application is not looking for
// them.
const releasesPage = "https://github.com/MeshBench/meshbench/releases"
