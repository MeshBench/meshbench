package meshbench_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

// Build a small network and run it, with no window anywhere.
func Example_headless() {
	ctx := context.Background()
	wb, err := meshbench.Headless(ctx, meshbench.Fixture("fife-strict"), meshbench.Seed(9001))
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
	wb, err := meshbench.Attach(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	if err := wb.Project().New(ctx, "Fife"); err != nil {
		log.Fatal(err)
	}
	// One warm at the end rather than one per node.
	if _, err := wb.Nodes().PlaceMany(ctx, []meshbench.Placement{
		{Name: "R1", Kind: meshbench.SimpleRepeater, Lat: 56.20, Lon: -3.20},
		{Name: "R2", Kind: meshbench.SimpleRepeater, Lat: 56.12, Lon: -3.02},
		// One of them is a T-Deck, which decides its transmit ceiling, its
		// noise figure and the battery the energy model uses.
		{Name: "C1", Kind: meshbench.Companion, Lat: 56.19, Lon: -3.17,
			Board: meshbench.BoardLilyGoTDeck},
		{Name: "C2", Kind: meshbench.Companion, Lat: 56.09, Lon: -3.10},
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
	wb, err := meshbench.Attach(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	stock, err := wb.Firmware().Find(ctx, "repeater-v1.17.0", "")
	if err != nil {
		log.Fatal(err)
	}
	// Or build it here, which is what a comparison against a checkout wants:
	//   built, err := wb.Firmware().BuildAndWait(ctx, "~/src/MeshCore", 0)
	changed, err := wb.Firmware().Import(ctx, "/tmp/my-build", "repeater", "", "")
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

// Repeating traffic, and a verdict. What CI actually runs.
func Example_regression() {
	ctx := context.Background()
	wb, err := meshbench.Headless(ctx, meshbench.Fixture("fife-strict"), meshbench.Seed(9001))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = wb.Close() }()

	// Simulated seconds - the mesh's own clock, not yours.
	if err := wb.Schedule().Add(ctx, meshbench.Send{
		Node: "AngusOutlaw1", Command: "send hello",
		At: 5 * time.Second, Every: 20 * time.Second,
	}); err != nil {
		log.Fatal(err)
	}
	if err := wb.Assertions().Delivered(ctx, 40); err != nil {
		log.Fatal(err)
	}
	if err := wb.Sim().Run(ctx, 5*time.Minute, time.Hour); err != nil {
		log.Fatal(err)
	}

	report, err := wb.Assertions().Check(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// The report prints the caveats above the numbers itself: this is the
	// output somebody pastes into a pull request, and the caveats are the half
	// that gets dropped.
	fmt.Println(report)
	if err := report.WriteJUnit("results.xml", ""); err != nil {
		log.Fatal(err)
	}
	if !report.OK() {
		log.Fatal("the mesh stopped delivering")
	}
}

// Ask a node something and get its answer, rather than the moment before it.
func Example_askingANode() {
	ctx := context.Background()
	wb, err := meshbench.Attach(ctx)
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
