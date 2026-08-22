package client_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/MeshBench/meshbench/client"
)

// Build a small network and run it, with no window anywhere.
func Example_headless() {
	ctx := context.Background()
	wb, err := client.Headless(ctx, client.Fixture("fife-strict"), client.Seed(9001))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	// Five minutes of the mesh's own clock. On a large network that is a great
	// deal more than five of yours, which is why the wait is its own argument.
	if err := wb.Sim().Run(ctx, 5*time.Minute, 30*time.Minute); err != nil {
		log.Fatal(err)
	}

	n, err := wb.Events().Total(ctx)
	if err != nil {
		log.Fatal(err)
	}
	p, err := wb.Provenance(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// The caveats go with the number, because the number is what gets pasted
	// into a report.
	fmt.Println(p)
	fmt.Printf("%d events\n", n)
}

// Place a network by hand, the way somebody would on the map.
func Example_placingNodes() {
	ctx := context.Background()
	wb, err := client.Attach(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	if err := wb.Project().New(ctx, "Fife"); err != nil {
		log.Fatal(err)
	}
	// One warm at the end rather than one per node.
	if _, err := wb.Nodes().PlaceMany(ctx, []client.Placement{
		{Name: "R1", Kind: client.SimpleRepeater, Lat: 56.20, Lon: -3.20},
		{Name: "R2", Kind: client.SimpleRepeater, Lat: 56.12, Lon: -3.02},
		{Name: "C1", Kind: client.Companion, Lat: 56.19, Lon: -3.17},
		{Name: "C2", Kind: client.Companion, Lat: 56.09, Lon: -3.10},
	}); err != nil {
		log.Fatal(err)
	}
	if err := wb.Sim().Start(ctx); err != nil {
		log.Fatal(err)
	}
	if err := wb.Firmware().WaitStarted(ctx, 10*time.Minute); err != nil {
		log.Fatal(err)
	}
}

// Pin two builds to two nodes and compare them - the A/B this API was asked
// for, in six lines.
func Example_twoBuildsInOneScenario() {
	ctx := context.Background()
	wb, err := client.Attach(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	stock, err := wb.Firmware().Find(ctx, "repeater-v1.17.0", "")
	if err != nil {
		log.Fatal(err)
	}
	changed, err := wb.Firmware().Import(ctx, "/tmp/my-build", "repeater", "")
	if err != nil {
		log.Fatal(err)
	}
	// Applied, which means each node stops, is provisioned again and starts.
	if err := wb.Node("Abernethy Repeater").SetFirmware(ctx, stock, true); err != nil {
		log.Fatal(err)
	}
	if err := wb.Node("Bishop Hill").SetFirmware(ctx, changed, true); err != nil {
		log.Fatal(err)
	}
}

// Ask a node something and get its answer, rather than the moment before it.
func Example_askingANode() {
	ctx := context.Background()
	wb, err := client.Attach(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	// A node reads its serial input on its next loop, and its loop only runs
	// when the engine steps. Ask sends, gives the mesh its own time, and then
	// reads - which is what every hand-written version of this forgets.
	reply, err := wb.Node("Abernethy Repeater").Console().Ask(ctx, "get region", 100)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(reply)
}
