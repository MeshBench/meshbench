// Two builds, two nodes, one scenario - the A/B #192 was filed from.
//
//	go run ./clients/go/examples/two-builds-in-one-scenario <stock version> <local build path>
//
// The most common real use of this API, and the reason the node window grew a
// firmware control: comparing a stock build against one with a single changed
// constant, on the same mesh, at the same seed.
//
// Needs a display: it opens the workbench so you can watch both arms run.
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
	if len(os.Args) < 3 {
		log.Fatal("usage: two-builds-in-one-scenario <stock version> <local build path>")
	}
	stockVersion, localPath := os.Args[1], os.Args[2]

	ctx := context.Background()
	wb, err := meshbench.Launch(ctx,
		meshbench.Fixture("fife-strict"), meshbench.Seed(seed))
	must(err)
	defer func() { _ = wb.Close() }()

	stock, err := wb.Firmware().Find(ctx, stockVersion, "")
	must(err)
	changed, err := wb.Firmware().Import(ctx, localPath, meshbench.RoleSimpleRepeater, "", "")
	must(err)

	// Two nodes far enough apart to be independently interesting, one on each
	// build. Applied, which restarts each of them.
	nodes, err := wb.Nodes().List(ctx)
	must(err)
	if len(nodes) < 2 {
		log.Fatal("this scenario has fewer than two nodes to compare")
	}
	a, b := wb.Node(nodes[0].Name), wb.Node(nodes[1].Name)
	must(a.SetFirmware(ctx, stock, true))
	must(b.SetFirmware(ctx, changed, true))

	must(wb.Sim().Start(ctx))
	must(wb.Firmware().WaitStarted(ctx, 15*time.Minute))
	must(wb.Sim().Run(ctx, 5*time.Minute, time.Hour))

	// Per node, because the whole point is which of the two behaved
	// differently - a total would hide it.
	p, err := wb.Provenance(ctx)
	must(err)
	fmt.Println(p)
	stats, err := wb.NodeStats(ctx)
	must(err)
	for _, want := range []struct {
		node  meshbench.Node
		build meshbench.Build
	}{{a, stock}, {b, changed}} {
		for _, s := range stats {
			if s.Name != want.node.Name() {
				continue
			}
			fmt.Printf("%-24s %-32s sent %4d  heard %4d\n",
				s.Name, want.build.Describe(), s.Sent, s.Heard)
		}
	}

	// One run of one seed is one draw. A difference here is a hypothesis, not
	// a result: vary the seed before believing anything.
	fmt.Println("\none seed, one draw - vary the seed before calling this a difference")
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
