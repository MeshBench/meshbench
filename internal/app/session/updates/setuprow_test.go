package updates_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session/updates"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// The Setup row is the one place a person meets any of this, so its states are
// pinned: which action it offers, and what it is prepared to call urgent.

func TestAWorkingCopyRowOffersNothingAndSaysWhy(t *testing.T) {
	row := updates.SetupRow(state.Update{}, false, false)
	if row.Verb != "" {
		t.Errorf("a working copy offers %q; it is unreleased, not behind", row.Verb)
	}
	if !strings.Contains(row.Do, "working copy") {
		t.Errorf("the row says %q, want it to say this is a working copy", row.Do)
	}
}

func TestTheRowsAStampedBuildCanBeIn(t *testing.T) {
	released(t, "v0.1.0")
	cases := []struct {
		name            string
		u               state.Update
		allowed, asked  bool
		wantState, verb string
	}{
		{"nobody has answered", state.Update{}, false, false,
			string(state.SetupUndecided), "update.allow"},
		{"answered no", state.Update{}, false, true,
			string(state.SetupReady), "update.allow"},
		{"allowed, nothing asked yet", state.Update{}, true, true,
			string(state.SetupReady), "update.check"},
		{"the feed could not be reached",
			state.Update{Checked: "2026-09-01T00:00:00Z", Err: "no route to host"},
			true, true, string(state.SetupReady), "update.check"},
		{"already the newest",
			state.Update{Checked: "2026-09-01T00:00:00Z", Latest: "0.1.0"},
			true, true, string(state.SetupReady), "update.check"},
		{"a newer one to take",
			state.Update{Checked: "2026-09-01T00:00:00Z", Latest: "0.2.0",
				Asset: "meshbench-linux-x86_64.tar.gz", Bytes: 44_000_000},
			true, true, string(state.SetupUndecided), "update.download"},
		{"already downloaded",
			state.Update{Checked: "2026-09-01T00:00:00Z", Latest: "0.2.0",
				Asset: "meshbench-linux-x86_64.tar.gz", Bytes: 44_000_000,
				Staged: "/cache/meshbench/updates/v0.2.0/meshbench-linux-x86_64.tar.gz"},
			true, true, string(state.SetupReady), "update.reveal"},
	}
	for _, c := range cases {
		row := updates.SetupRow(c.u, c.allowed, c.asked)
		if row.State != c.wantState {
			t.Errorf("%s: state %q, want %q", c.name, row.State, c.wantState)
		}
		if row.Verb != c.verb {
			t.Errorf("%s: offers %q, want %q", c.name, row.Verb, c.verb)
		}
		if row.Do == "" {
			t.Errorf("%s: the row says nothing, and a row whose only "+
				"instruction is a button says nothing over a socket", c.name)
		}
	}
}

// An offer states its size before it is spent, and warns about the one
// consequence people meet later: a pinned client will be refused by the
// workbench it used to drive.
func TestAnOfferPricesItselfAndWarnsAboutPinnedClients(t *testing.T) {
	released(t, "v0.1.0")
	row := updates.SetupRow(state.Update{
		Checked: "2026-09-01T00:00:00Z", Latest: "0.2.0",
		Asset: "meshbench-linux-x86_64.tar.gz", Bytes: 44_000_000,
	}, true, true)
	if !strings.Contains(row.Cost, "MB") {
		t.Errorf("the cost is %q, want a size said before it is spent", row.Cost)
	}
	if !strings.Contains(row.Do, "0.2.0 workbench") &&
		!strings.Contains(row.Do, "pinned to "+version.Release()) {
		t.Errorf("the row says %q, want it to say a pinned client will be "+
			"refused by the new workbench", row.Do)
	}
}

// A newer release this machine cannot take from here is still said. Hiding it
// would leave somebody on an old build with nothing to explain it.
func TestAReleaseThisMachineCannotTakeIsStillReported(t *testing.T) {
	released(t, "v0.1.0")
	row := updates.SetupRow(state.Update{
		Checked: "2026-09-01T00:00:00Z", Latest: "0.2.0",
		Why: "this build was installed by a package manager",
	}, true, true)
	if !strings.Contains(row.Do, "0.2.0") {
		t.Errorf("the row says %q, want it to name the release that exists", row.Do)
	}
	if !strings.Contains(row.Do, "package manager") {
		t.Errorf("the row says %q, want it to say why it cannot be taken here", row.Do)
	}
}
