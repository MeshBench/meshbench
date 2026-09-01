package shell

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The download the measurement is waiting on is the one worth naming.
//
// A warm opens its own job first and only then starts the terrain fetch, so
// taking the oldest running job put "measuring every link - 0%" on screen for
// the whole of a fresh install's first download and never once mentioned that
// half a gigabyte was arriving.
func TestTheStatusLineNamesTheNewestRunningJob(t *testing.T) {
	got := JobWords([]state.Job{
		{ID: "links", What: "measuring every link", Done: 0, Total: 71253},
		{ID: "tiles", What: "fetching terrain, 84 MB of about 366 MB",
			Done: 1420, Total: 6233},
	})
	if !strings.Contains(got, "fetching terrain") {
		t.Errorf("the line does not mention the download: %q", got)
	}
	if !strings.Contains(got, "84 MB") {
		t.Errorf("the line does not say what the download has cost: %q", got)
	}
	if !strings.Contains(got, "and 1 more running") {
		t.Errorf("the line hides the other running job rather than counting it: %q", got)
	}
}

// A download started while something else is running still gets said.
func TestADownloadIsNeverSilent(t *testing.T) {
	got := JobWords([]state.Job{
		{ID: "links", What: "measuring every link", Done: 4873, Total: 71253},
		{ID: "fw-2.7.26-repeater", What: "downloading repeater 2.7.26",
			Done: 3400, Total: 6800},
	})
	if !strings.Contains(got, "downloading repeater") {
		t.Errorf("a firmware download in flight had nowhere to appear: %q", got)
	}
}

func TestAFinishedJobDoesNotOwnTheLine(t *testing.T) {
	if got := JobWords([]state.Job{
		{ID: "tiles", What: "fetching terrain", Done: 1, Total: 1, Finished: true},
	}); got != "" {
		t.Errorf("a job that ended still owns the status line: %q", got)
	}
	if got := JobWords(nil); got != "" {
		t.Errorf("an empty job list produced a line: %q", got)
	}
}

// A job with no total says it is working rather than dividing by zero.
func TestAJobWithNoTotalStillSaysSomething(t *testing.T) {
	got := JobWords([]state.Job{{ID: "environ-fetch", What: "buildings: overpass"}})
	if !strings.Contains(got, "buildings: overpass") || !strings.Contains(got, "working") {
		t.Errorf("a job with no total said %q", got)
	}
}
