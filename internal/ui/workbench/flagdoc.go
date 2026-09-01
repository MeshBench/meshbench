package workbench

import (
	"flag"

	"github.com/MeshBench/meshbench/internal/app/flagdump"
)

// describedInstead hands the workbench's flag set to whatever is generating the
// CLI reference, and reports that there is nothing else to do.
//
// The workbench declares its thirty-eight flags in Run and nowhere else, so
// this is the only place they can be read from. Called before Parse and before
// a window, because a process asked to describe the command line must not open
// one.
func describedInstead() bool {
	if !flagdump.Wanted() {
		return false
	}
	// No describing line: the workbench's -h has none, and the index's summary
	// stands for both rather than a second sentence being invented here to go
	// stale beside the first.
	flagdump.Record("workbench", "", flag.CommandLine)
	return true
}
