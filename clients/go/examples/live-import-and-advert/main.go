// Import a real mesh from its live feed, find one node, and make it advert.
//
//	go run ./clients/go/examples/live-import-and-advert [area] [node]
//
//	go run ./clients/go/examples/live-import-and-advert Fife
//	go run ./clients/go/examples/live-import-and-advert bounds/tay-catchment.geojson
//
// The area is a place name or a path to GeoJSON, and it is set before the
// import because the import filters at fetch time - the whole feed is around
// 676 nodes and this is how you study a corner of it.
//
// The node is searched for rather than typed: the names on a real mesh carry
// emoji, so "West Lomond" is really "🏔️ West Lomond 📡".
//
// Needs the network.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const feed = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"

func main() {
	area, wanted := "Fife", "West Lomond"
	if len(os.Args) > 1 {
		area = os.Args[1]
	}
	if len(os.Args) > 2 {
		wanted = os.Args[2]
	}

	ctx := context.Background()
	wb, err := meshbench.Headless(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	studying, err := wb.Boundary().Use(ctx, area)
	must(err)
	fmt.Println("studying", strings.Join(studying, ", "))

	// Fetch the nodes, commit them, read a week of traffic, and apply the
	// regions it implies. Skip that last step and nothing ever relays.
	found, err := wb.Live().Pull(ctx, feed, 0, 0)
	must(err)
	fmt.Println(found)
	must(wb.WaitIdle(ctx, time.Hour))

	node, err := wb.Nodes().Find(ctx, wanted)
	must(err)
	all, err := wb.Nodes().List(ctx)
	must(err)
	fmt.Printf("%s is %q, one of %d\n", wanted, node.Name(), len(all))

	_, err = wb.Firmware().UseWhatIsHere(ctx)
	must(err)
	must(wb.Sim().Start(ctx))
	must(wb.Firmware().WaitStarted(ctx, 0))

	// Ask, rather than Send and then Read: reading straight after sending
	// reads the moment before the command went out.
	reply, err := node.Console().Ask(ctx, "advert", 100)
	must(err)
	fmt.Println(reply)
	must(wb.Sim().Run(ctx, 2*time.Minute, time.Hour))

	events, err := wb.Events().Recent(ctx, 2000)
	must(err)
	heard := map[string]bool{}
	for _, e := range events {
		if e.Class == meshbench.ClassReceived && e.From == node.Name() {
			heard[e.To] = true
		}
	}
	fmt.Printf("%d of %d others heard it directly\n", len(heard), len(all)-1)

	prov, err := wb.Provenance(ctx)
	must(err)
	fmt.Println(prov)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
