package workbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// savedRuns writes n run records where the tool keeps them, newest last.
func savedRuns(t *testing.T, n int) {
	t.Helper()
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("LOCALAPPDATA", cacheHome)
	dir, err := os.UserCacheDir()
	if err != nil {
		t.Skip("no cache directory on this machine, so there is nowhere to save a run")
	}
	dir = filepath.Join(dir, "meshbench", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	for i := range n {
		rec := session.RunRecord{
			Name:    fmt.Sprintf("run-%03d", i),
			At:      at.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Seed:    uint64(i),
			Build:   "v1.7.1",
			Outcome: "saved",
			Metrics: map[string]float64{"receptions": float64(i)},
		}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rec.Name+".json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// waitForRuns is the frames a panel would draw while the disk is answering.
func waitForRuns(t *testing.T, l *runLoader) runsResult {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if res, ok := l.records(); ok {
			return res
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the saved runs never arrived")
	return runsResult{}
}

// The first ask must not be the read.
//
// A panel that reads a directory in the frame it first draws is a window that
// does not paint until the disk answers, and the pile it reads grows with the
// project rather than with the session.
func TestRunsAreReadOffTheFrameGoroutine(t *testing.T) {
	savedRuns(t, 3)
	var l runLoader
	if _, ok := l.records(); ok {
		t.Fatal("the first ask answered with records, so it read the directory itself")
	}
	res := waitForRuns(t, &l)
	if len(res.runs) != 3 || res.total != 3 {
		t.Fatalf("read %d of %d runs, want 3 of 3", len(res.runs), res.total)
	}
	if res.runs[0].Name != "run-002" {
		t.Errorf("newest run is %q, want run-002", res.runs[0].Name)
	}
}

// However old the project is, the panel holds a bounded number of runs, and
// says how many it found.
func TestOldProjectsDoNotLoadEveryRunEverSaved(t *testing.T) {
	savedRuns(t, runsKept+40)
	var l runLoader
	res := waitForRuns(t, &l)
	if len(res.runs) != runsKept {
		t.Errorf("kept %d runs, want %d", len(res.runs), runsKept)
	}
	if res.total != runsKept+40 {
		t.Errorf("found %d runs, want %d", res.total, runsKept+40)
	}
	if got := len(runRows(res.runs)); got != runsKept {
		t.Errorf("%d rows for %d runs", got, runsKept)
	}
}

// A save asks again, and the panel notices the answer is a new one.
func TestSavingARunAsksTheDiskAgain(t *testing.T) {
	savedRuns(t, 2)
	var l runLoader
	first := waitForRuns(t, &l)
	l.reload()
	if _, ok := l.records(); ok {
		t.Fatal("the ask after a save answered from the frame goroutine")
	}
	second := waitForRuns(t, &l)
	if second.gen == first.gen {
		t.Error("the second read is indistinguishable from the first, so rows never rebuild")
	}
}

// Neither panel touches the disk in the frame it first draws.
func TestRunPanelsDrawBeforeTheDiskAnswers(t *testing.T) {
	savedRuns(t, 4)
	runs := &runsPanel{}
	cmp := &comparePanel{}
	snap := &state.Snapshot{}
	for _, p := range []*panelHarness{
		newPanelHarness(runs.Draw, snap),
		newPanelHarness(cmp.Draw, snap),
	} {
		p.frame()
	}
	if runs.shownGen != 0 || len(runs.rows) != 0 {
		t.Error("the runs panel had rows on its first frame, so it read the directory in it")
	}
	if cmp.shownGen != 0 || len(cmp.rows) != 0 {
		t.Error("the comparison panel had rows on its first frame, so it read the directory in it")
	}
	// And the read it started does land, so the panel is not merely empty.
	res := waitForRuns(t, &runs.runs)
	if len(res.runs) != 4 {
		t.Errorf("read %d runs, want 4", len(res.runs))
	}
}
