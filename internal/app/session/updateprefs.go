// Whether this machine may ask whether a newer release exists.
//
// The same three states terrain uses, and for the same reason: allowed and
// refused are both answers, and never asked is the state a fresh install is in.
// What differs is the default underneath them. Terrain is off until asked
// because a study measured without it is quietly wrong; an update check is off
// until asked because nothing in the simulation depends on it at all. A machine
// with no network, or one whose owner does not want the question, loses nothing
// by never asking it - which is why nobody is asked twice, and why a refusal is
// remembered rather than re-put on every launch.
package session

import (
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// UpdateConsent is the three-state answer: whether the release feed may be
// asked, and whether anybody has said either way.
func (s *Sim) UpdateConsent() (allowed, asked bool) {
	if s.prefs.UpdateChecks == nil {
		return false, false
	}
	return *s.prefs.UpdateChecks, true
}

// SetUpdateChecks records the answer. Saving it is the caller's, because only
// the caller has the world to say into when the disk refuses.
func (s *Sim) SetUpdateChecks(on bool) { s.prefs.UpdateChecks = &on }

// UpdateChecked is when the release feed was last asked, or the zero time.
//
// Remembered across launches so that a check on a schedule is a check on a
// schedule: without it, every launch is a first launch and "once a day" becomes
// "every time the application opens".
func (s *Sim) UpdateChecked() time.Time {
	t, err := time.Parse(time.RFC3339, s.prefs.UpdateChecked)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SetUpdateChecked records when the feed answered, and persists it.
//
// Written even when the check failed: a machine with no network would otherwise
// retry on every timer, which is the difference between a quiet application and
// one that spends the morning talking to a socket that is not there.
func (s *Sim) SetUpdateChecked(w *state.World, t time.Time) {
	s.prefs.UpdateChecked = t.UTC().Format(time.RFC3339)
	_ = s.savePrefs(w)
}

// UpdateFeed is where the release check asks, empty for the published feed.
func (s *Sim) UpdateFeed() string { return s.updateFeed }
