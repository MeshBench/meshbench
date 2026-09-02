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

// The chip beside the play button follows the same rule as the bar.
//
// The bar was taught to name the newest running job and the transport strip
// was not, so through a whole fresh install's download the chip in the busiest
// corner of the window said "measuring every link" while the measurement had
// done none of them and half a gigabyte was arriving unmentioned.
func TestTheWarmingChipNamesTheDownloadItIsWaitingOn(t *testing.T) {
	jobs := []state.Job{
		{ID: "links", What: "measuring every link", Done: 0, Total: 71253},
		{ID: "tiles", What: "fetching terrain, 197 MB of about 499 MB",
			Done: 2480, Total: 6233},
	}
	s := &state.Snapshot{Jobs: jobs}
	warm := warmingJob(s)
	if warm == nil {
		t.Fatal("the measurement is running and the strip did not find it")
	}
	got := warmingWords(warmingNow(s, warm))
	if !strings.Contains(got, "fetching terrain") {
		t.Errorf("the chip does not mention the download: %q", got)
	}
	if !strings.Contains(got, "197 MB") {
		t.Errorf("the chip does not say what the download has cost: %q", got)
	}
}

// With nothing else running the measurement still owns its own chip.
func TestTheWarmingChipNamesTheMeasurementWhenItIsTheWork(t *testing.T) {
	s := &state.Snapshot{Jobs: []state.Job{
		{ID: "tiles", What: "fetching terrain", Done: 6233, Total: 6233, Finished: true},
		{ID: "links", What: "measuring every link", Done: 35000, Total: 71253},
	}}
	warm := warmingJob(s)
	if warm == nil {
		t.Fatal("the measurement is running and the strip did not find it")
	}
	got := warmingWords(warmingNow(s, warm))
	if !strings.Contains(got, "measuring every link") {
		t.Errorf("a finished download still owns the chip: %q", got)
	}
	if !strings.Contains(got, "49%") {
		t.Errorf("the chip lost the measurement's own percentage: %q", got)
	}
}
