package updates

import (
	"strconv"
	"time"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/update"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// What a check has to say, in one place, because the same sentences are read
// three ways: as a status line, as a row on the Setup page, and as the answer a
// script gets back over the socket. Three wordings would be three chances for
// one of them to be more optimistic than the others.

// available reports whether the last check found a release worth taking: newer
// than this build, and with something published this machine could actually
// take.
func available(u state.Update) bool {
	return update.Newer(version.Release(), u.Latest) && u.Asset != ""
}

// statusWire is the whole answer, in the shape the socket carries.
func statusWire(u state.Update, allowed, asked bool) map[string]any {
	return map[string]any{
		"build":     version.Release(),
		"latest":    u.Latest,
		"tag":       u.Tag,
		"newer":     update.Newer(version.Release(), u.Latest),
		"available": available(u),
		"notes":     u.Notes,
		"published": u.Published,
		"checked":   u.Checked,
		"asset":     u.Asset,
		"bytes":     u.Bytes,
		"artefact":  u.Artefact,
		"why":       u.Why,
		"staged":    u.Staged,
		"feed":      u.Feed,
		"error":     u.Err,
		"allowed":   allowed,
		"asked":     asked,
	}
}

// checkWords is the quiet line a finished check leaves in the status bar.
//
// A line rather than a dialog. An update is never more urgent than the run it
// would interrupt, and a modal in front of a simulation is exactly the wrong
// shape for news that can wait until the session is over.
func checkWords(u state.Update) string {
	feed := ""
	if u.Feed != "" {
		feed = " (asked " + u.Feed + ", not the published release feed)"
	}
	switch {
	case u.Err != "":
		return "could not find out whether a newer release exists: " + u.Err +
			". Nothing is wrong with this build" + feed
	case u.Latest == "" && u.Why != "":
		return u.Why + feed
	case !update.Newer(version.Release(), u.Latest):
		return "this is the newest release: " + version.Release() + feed
	case u.Asset == "":
		return "MeshBench " + u.Latest + " has been released, and " + u.Why +
			". The release page is " + u.Notes + feed
	default:
		return "MeshBench " + u.Latest + " is out" + releasedWhen(u) +
			"; this build is " + version.Release() + ". Setup, under This " +
			"build, will download it (" + resource.SIBytes(u.Bytes) +
			") beside this one and say how to swap it. Nothing is replaced " +
			"while a session is open" + feed
	}
}

// releasedWhen says how long ago, in the units somebody would say it in, and
// says nothing at all when the feed did not carry a date.
func releasedWhen(u state.Update) string {
	t, err := time.Parse(time.RFC3339, u.Published)
	if err != nil {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return ""
	case d < 2*time.Hour:
		return " (published in the last hour)"
	case d < 48*time.Hour:
		return " (published " + strconv.Itoa(int(d.Hours())) + " hours ago)"
	default:
		return " (published " + strconv.Itoa(int(d.Hours()/24)) + " days ago)"
	}
}
