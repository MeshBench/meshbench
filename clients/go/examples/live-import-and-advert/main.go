// Import a real mesh from its live feed, find one node, and make it advert.
//
//	go run ./clients/go/examples/live-import-and-advert ["node name"]
//
// Needs the network. The names on a real mesh carry emoji - "West Lomond" is
// really "🏔️ West Lomond 📡" - so the node is searched for rather than typed.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const feed = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"

// ScotMesh is around 676 nodes and firmware on all of them is hours, so this
// keeps the one we want and its dozen nearest.
const neighbours = 12

func main() {
	want := "West Lomond"
	if len(os.Args) > 1 {
		want = os.Args[1]
	}

	ctx := context.Background()
	wb, err := meshbench.Headless(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	// Fetch the nodes, commit them, read a week of traffic, and apply the
	// regions it implies. Skip that last step and nothing ever relays.
	found, err := wb.Live().Pull(ctx, feed, 0, 0)
	must(err)
	fmt.Println(found)
	must(wb.WaitIdle(ctx, time.Hour))

	node, err := wb.Nodes().Find(ctx, want)
	must(err)
	fmt.Printf("%s is %q\n", want, node.Name())

	near, err := wb.Nodes().Near(ctx, node.Name(), neighbours)
	must(err)
	keep := []string{node.Name()}
	for _, n := range near {
		keep = append(keep, n.Name)
	}
	must(wb.Nodes().Keep(ctx, keep...))
	must(wb.WaitIdle(ctx, time.Hour))

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
	fmt.Printf("%d of %d neighbours heard it directly\n", len(heard), len(keep)-1)

	prov, err := wb.Provenance(ctx)
	must(err)
	fmt.Println(prov)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
