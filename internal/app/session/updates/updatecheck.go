package updates

import (
	"context"
	"runtime"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/update"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// checkJob is the jobs-strip entry a check runs under. A network call with
// nothing on screen is indistinguishable from a hang, and this one can take as
// long as a socket takes to time out.
const checkJob = "update"

// checkTimeout bounds the whole check. Longer than a healthy answer by a wide
// margin, and short enough that a machine behind a captive portal finds out
// today.
const checkTimeout = 30 * time.Second

func registerCheck(st *state.Store, s *session.Sim) {
	st.Handle("update.allow", func(w *state.World, p any) (any, error) {
		// Allow by default: this verb exists to grant permission, and a caller
		// who wrote no argument at all wrote the common case.
		on := true
		if v, ok := session.BoolField(p, "on"); ok {
			on = v
		}
		_, asked := s.UpdateConsent()
		s.SetUpdateChecks(on)
		// The world learns the answer as well as the settings file, so the
		// switch in Configuration is drawn from what is in force rather than
		// from what the panel last thought.
		w.Update.Allowed, w.Update.Asked = on, true
		sayConsent(w, s.SavePrefs(w) == nil, on)
		// The check the operator has just asked for, rather than one on the
		// next launch: nothing is watching for permission to arrive, and a
		// switch that promises something and does it tomorrow is a switch
		// nobody believes.
		checking := on && version.Release() != ""
		if checking {
			startCheck(st, s)
		}
		return map[string]any{"on": on, "asked": asked, "checking": checking}, nil
	})

	// Read-only, and the one a script polls: a check is a network call and
	// cannot be made to answer on the store's own goroutine without stopping
	// everything else the store is doing.
	st.Handle("update.status", func(w *state.World, _ any) (any, error) {
		w.Update.Allowed, w.Update.Asked = s.UpdateConsent()
		return statusWire(w.Update, w.Update.Allowed, w.Update.Asked), nil
	})

	st.Handle("update.check", func(w *state.World, _ any) (any, error) {
		// An explicit call is its own consent for this one check. The
		// preference governs the check nobody asked for - the one on a timer -
		// which is the one that has to be permitted in advance.
		if version.Release() == "" {
			allowed, asked := s.UpdateConsent()
			w.Update = workingCopy()
			w.Update.Allowed, w.Update.Asked = allowed, asked
			w.Say(w.Update.Why)
			return statusWire(w.Update, allowed, asked), nil
		}
		startCheck(st, s)
		return map[string]any{"checking": true, "build": version.Release()}, nil
	})

	// The answer arriving from the worker. Internal because it is the check
	// reporting back to itself, and a caller who invoked it by hand would be
	// writing a result nothing measured.
	st.HandleInternal("update.checked", func(w *state.World, p any) (any, error) {
		u, _ := p.(state.Update)
		u.Allowed, u.Asked = s.UpdateConsent()
		w.Update = u
		s.SetUpdateChecked(w, time.Now())
		w.Say(checkWords(u))
		return statusWire(u, u.Allowed, u.Asked), nil
	})
}

// startCheck asks the feed on a worker and posts the answer back.
func startCheck(st *state.Store, s *session.Sim) {
	feed := s.UpdateFeed()
	go func() {
		ctx, stop := context.WithTimeout(context.Background(), checkTimeout)
		defer stop()
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: checkJob, What: "asking whether a newer release exists",
			Done: 0, Total: 1})
		u := ask(ctx, feed)
		done, release := session.Finishing(ctx)
		defer release()
		_, _ = st.Do(done, "job.done", checkJob)
		_, _ = st.Do(done, "update.checked", u)
	}()
}

// ask is the check itself: what the feed says, against what this build is.
func ask(ctx context.Context, feed string) state.Update {
	c := update.Checker{Feed: feed}
	u := state.Update{
		Checked:  time.Now().UTC().Format(time.RFC3339),
		Artefact: string(update.This()),
	}
	if c.Redirected() {
		u.Feed = feed
	}
	// The cheap route first: a redirect, not an API call. The API allows an
	// unauthenticated caller sixty requests an hour per address, and an address
	// is a household, an office or an ISP doing carrier-grade NAT - so a check
	// on every launch would spend everybody's on that address.
	rel, err := c.Latest(ctx)
	if err != nil {
		// Held as an error rather than folded into "nothing newer". A rate
		// limit, a captive portal and an up-to-date build are three different
		// answers, and only one of them is about this build.
		u.Err = err.Error()
		return u
	}
	u.Tag, u.Latest, u.Notes = rel.Tag, rel.Version, rel.Notes
	if !update.Newer(version.Release(), rel.Version) {
		return u
	}
	return detail(ctx, c, u)
}

// detail fills in what only the API knows, and is asked once per release found
// rather than once per check.
//
// A refusal here is not a refusal of the whole check: the newer release is
// already known to exist, and saying so without its size is a better answer
// than pretending nothing was found.
func detail(ctx context.Context, c update.Checker, u state.Update) state.Update {
	rel, err := c.Detail(ctx, u.Tag)
	if err != nil {
		u.Why = "its details could not be read (" + err.Error() +
			"), so there is nothing here to price or fetch yet"
		return u
	}
	if rel.Notes != "" {
		u.Notes = rel.Notes
	}
	if !rel.Published.IsZero() {
		u.Published = rel.Published.UTC().Format(time.RFC3339)
	}
	if rel.Prerelease {
		// Never offered. The redirect this followed is GitHub's own "latest",
		// which never names one, so this is only reachable from a feed somebody
		// pointed here by hand - and a pre-release installed by a machine that
		// was asking about releases is not what anybody meant.
		u.Why = rel.Tag + " is a pre-release, and pre-releases are not offered here"
		u.Latest = ""
		return u
	}
	a, why := update.AssetFor(rel, update.This(), runtime.GOOS, runtime.GOARCH, update.ThisVariant())
	u.Asset, u.Bytes, u.Why = a.Name, a.Bytes, why
	return u
}

// workingCopy is the answer for a build nobody released.
//
// Not an error and not a version comparison: a working copy is not behind, it
// is unreleased, and telling the person building the thing that they are out of
// date is how an updater earns its reputation.
func workingCopy() state.Update {
	return state.Update{
		Checked:  time.Now().UTC().Format(time.RFC3339),
		Artefact: string(update.This()),
		Why: "this build is " + version.String() + " rather than a release, so " +
			"there is nothing for it to be behind. Update checks apply to a " +
			"tagged build",
	}
}

// sayConsent is what the switch promises, said only where it can be kept.
func sayConsent(w *state.World, saved, on bool) {
	switch {
	case !saved:
		w.Say("update checks are " + onOff(on) + " for this session")
	case on:
		w.Say("update checks are on, here and on the next launch: once a day " +
			"the release page is asked whether a newer version exists. Nothing " +
			"is downloaded and nothing is replaced without being asked for")
	default:
		w.Say("update checks are off, here and on the next launch. Nothing " +
			"will ask, so nothing will say a newer release exists; the switch " +
			"is in Configuration > System, and Setup has it under This build")
	}
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
