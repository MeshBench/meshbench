// The update check nobody asked for, which is the only one that has to be
// permitted in advance.
//
// Three rules, and each of them is something an updater usually gets wrong.
// It never happens as a condition of the application opening: the window is up
// and the fixture is loading long before this runs, and a machine with no
// network waits for nothing. It never happens without permission, which is a
// remembered three-state answer rather than a dialog on every launch. And it
// happens once a day rather than once a session, because the last check's time
// survives a restart - without that, "daily" quietly becomes "every time the
// application opens".
//
// It lives in the command rather than in the session because it is a decision
// about this way of running: a script driving a headless session did not ask
// for a network call, and does not get one.
package workbench

import (
	"context"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// updateCheckDelay is how long the check waits before asking.
//
// After the readiness page has had its say, so a first launch meets one page
// rather than a page and a status line arriving on top of each other.
const updateCheckDelay = 5 * time.Second

// updateEvery is how often the release feed is asked at most.
const updateEvery = 24 * time.Hour

// scheduleUpdateCheck asks, once, if this build is a release, permission was
// given, and the last answer is a day old. Forced by a flag, so the path can be
// driven without waiting a day for it.
func scheduleUpdateCheck(ctx context.Context, st *state.Store, sm *session.Sim, force bool) {
	go func() {
		// What this machine has already answered, so the switch in
		// Configuration opens saying what is in force rather than its zero
		// value. Read-only and offline: it asks the settings file, never the
		// network, and it happens whether or not a check is due.
		_, _ = st.Do(ctx, "update.status", nil)
		select {
		case <-ctx.Done():
			return
		case <-time.After(updateCheckDelay):
		}
		if !force && !dueForCheck(sm) {
			return
		}
		// The verb says why when it will not: a working copy is told it is
		// unreleased rather than out of date, and that sentence is the same one
		// whether a person or this timer asked.
		_, _ = st.Do(ctx, "update.check", nil)
	}()
}

// dueForCheck is the whole schedule: a release build, permission granted, and a
// day since the last answer.
func dueForCheck(sm *session.Sim) bool {
	if version.Release() == "" {
		return false
	}
	if allowed, _ := sm.UpdateConsent(); !allowed {
		return false
	}
	return time.Since(sm.UpdateChecked()) >= updateEvery
}
