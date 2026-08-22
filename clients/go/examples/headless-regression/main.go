// The one CI runs.
//
//	go run ./clients/go/examples/headless-regression [fixture] [junit.xml]
//
// No display, no GPU, no toolkit. Opens a fixture, runs it, checks its
// assertions, writes JUnit, and exits non-zero if the mesh stopped delivering.
// This is the shape a MeshCore pull request would use.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const seed = 9001

func main() {
	os.Exit(run())
}

func run() int {
	fixture := "fife-strict"
	if len(os.Args) > 1 {
		fixture = os.Args[1]
	}
	junit := ""
	if len(os.Args) > 2 {
		junit = os.Args[2]
	}

	ctx := context.Background()
	wb, err := meshbench.Headless(ctx,
		meshbench.Fixture(fixture), meshbench.Seed(seed))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	if err := wb.Sim().Run(ctx, 5*time.Minute, time.Hour); err != nil {
		log.Fatal(err)
	}

	report, err := wb.Assertions().Check(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// The report prints the caveats above the numbers itself, because this is
	// the output somebody pastes into a pull request and the caveats are the
	// half that gets dropped.
	fmt.Println(report)
	if total, err := wb.Events().Total(ctx); err == nil {
		fmt.Printf("%d events\n", total)
	}
	if junit != "" {
		if err := report.WriteJUnit(junit, ""); err != nil {
			log.Fatal(err)
		}
	}

	if report.Total == 0 {
		// Not a pass. A fixture with no assertions can report but cannot pass
		// or fail, and a green tick that checked nothing is the worst outcome
		// available here.
		return 2
	}
	if report.OK() {
		return 0
	}
	return 1
}
