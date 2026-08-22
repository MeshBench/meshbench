// Example 3 from #209: two repeaters, two companions, one of them a T-Deck,
// and a message to the public channel every twenty seconds.
//
//	go run ./clients/go/examples/small-mesh-with-traffic
//
// Costs about ten minutes of simulated time, and a few of yours.
//
// The repeating traffic needed no new verb: schedule.add has taken every_ms
// all along and nothing said so, which to somebody writing a script is the
// same as it not existing.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

var mesh = []meshbench.Placement{
	{Name: "R1", Kind: meshbench.SimpleRepeater, Lat: 56.20, Lon: -3.20},
	{Name: "R2", Kind: meshbench.SimpleRepeater, Lat: 56.12, Lon: -3.02},
	{Name: "C1", Kind: meshbench.Companion, Lat: 56.19, Lon: -3.17},
	{Name: "C2", Kind: meshbench.Companion, Lat: 56.09, Lon: -3.10},
}

func main() {
	ctx := context.Background()
	wb, err := meshbench.Headless(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	must(wb.Project().New(ctx, "Fife"))
	_, err = wb.Nodes().PlaceMany(ctx, mesh)
	must(err)
	must(wb.WaitIdle(ctx, 10*time.Minute))

	// C1 is the T-Deck. The board goes on before the firmware is pinned,
	// because a host image is not a board image and setting the board clears a
	// pin made for different hardware.
	must(wb.Node("C1").SetBoard(ctx, meshbench.BoardLilyGoTDeck))

	// Whatever this machine holds for each role, rather than a version typed
	// here that goes stale.
	for _, role := range []string{"repeater", "companion"} {
		builds, err := wb.Firmware().OnDisk(ctx)
		must(err)
		var pick *meshbench.Build
		for i := range builds {
			if builds[i].Role == role && builds[i].Board == "" {
				pick = &builds[i]
			}
		}
		if pick == nil {
			log.Fatalf("no %s build on this machine: meshcoresim firmware download %s",
				role, role)
		}
		must(wb.Firmware().UseForRole(ctx, role, *pick))
	}

	// Every twenty seconds, from the plain companion to the public channel.
	// Simulated time - the mesh's own clock, not yours.
	must(wb.Schedule().Add(ctx, meshbench.Send{
		Node: "C2", Command: "send hello",
		At: 5 * time.Second, Every: 20 * time.Second,
	}))

	must(wb.Sim().Start(ctx))
	must(wb.Firmware().WaitStarted(ctx, 10*time.Minute))
	must(wb.Sim().Run(ctx, 10*time.Minute, time.Hour))

	events, err := wb.Events().Recent(ctx, 1000)
	must(err)
	received := 0
	for _, e := range events {
		if e.Class == meshbench.ClassReceived {
			received++
		}
	}
	total, err := wb.Events().Total(ctx)
	must(err)
	p, err := wb.Provenance(ctx)
	must(err)

	fmt.Println(p)
	fmt.Printf("%d events, %d receptions in the tail\n", total, received)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
