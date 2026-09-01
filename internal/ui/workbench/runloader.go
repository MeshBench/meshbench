// The saved runs, read off the frame goroutine.
//
// Reading and unmarshalling every run record is a directory walk and a file
// per run, and nothing prunes what has been saved: the pile grows with the
// project rather than with the session, so the panel that reads it gets slower
// the longer the tool has been useful. Done in the frame the panel first
// draws, that walk is a window that does not paint until the disk answers.
//
// One loader shared by the two panels that want the same records, so a shape
// for "ask the disk once, draw the answer when it lands" is not written twice.
package workbench

import (
	"sync"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// runsKept is how many of the newest runs a panel holds. A table nobody can
// read past the first screen of is not worth the memory of the rest, and the
// count that was found is still reported so the cap is never a silent one.
const runsKept = 200

// runsResult is one finished read.
type runsResult struct {
	runs []session.RunRecord
	// total is how many were found, which is not len(runs) once the cap bites.
	total int
	// gen rises with each finished read, so a panel that has built rows from
	// one can tell when it is looking at another.
	gen int
}

// runLoader reads the saved runs on a goroutine of its own and hands the
// result to whichever frame asks for it next.
type runLoader struct {
	mu      sync.Mutex
	res     runsResult
	loaded  bool
	loading bool
	// gen counts asks rather than reads, so a read that was already in flight
	// when a save happened is recognised as stale and dropped.
	gen int
}

// records is the last finished read, and whether there has been one. The first
// call starts the read and reports false, which is a panel drawing "reading"
// rather than a window waiting on a disk.
func (l *runLoader) records() (runsResult, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loaded {
		return l.res, true
	}
	if !l.loading {
		l.loading = true
		go l.load(l.gen)
	}
	return runsResult{}, false
}

// reload drops what was read and asks again, for after a run is saved.
func (l *runLoader) reload() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loaded = false
	l.gen++
}

func (l *runLoader) load(gen int) {
	runs := session.LoadRuns()
	total := len(runs)
	if len(runs) > runsKept {
		runs = runs[:runsKept]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loading = false
	if gen != l.gen {
		// Asked again while this was in flight, so what it found predates the
		// save that asked for it. Dropped, and the next frame asks afresh.
		return
	}
	l.res = runsResult{runs: runs, total: total, gen: l.res.gen + 1}
	l.loaded = true
}
