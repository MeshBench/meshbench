// A blank setup, one companion, and its screen on show.
//
//	go run ./clients/go/examples/blank-setup-with-a-board
//
// Needs a display: it opens the node's own window on the Hardware tab at the
// end, which is the point of it. Run the headless examples instead if you have
// none.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const (
	wadamesh = "wadamesh"
	board    = meshbench.BoardLilyGoTDeck
)

func main() {
	ctx := context.Background()
	wb, err := meshbench.Launch(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	must(wb.Project().New(ctx, "Fife"))
	deck, err := wb.Nodes().Place(ctx, meshbench.Placement{
		Name: "Deck", Kind: meshbench.Companion,
		Lat: 56.19, Lon: -3.17, Board: board,
	})
	must(err)

	// Whatever the catalogue has, so this does not go stale against a version
	// number typed here.
	must(wb.Firmware().Scan(ctx))
	build, err := wb.Firmware().Find(ctx, wadamesh, string(board))
	if errors.Is(err, meshbench.ErrNotFound) {
		must(wb.Firmware().Download(ctx, "companion", wadamesh, string(board)))
		must(wb.WaitIdle(ctx, 10*time.Minute))
		build, err = wb.Firmware().Find(ctx, wadamesh, string(board))
	}
	must(err)

	// Applied: stop, provision, start. On a board that means an emulator,
	// which is why the wait below is generous.
	must(deck.SetFirmware(ctx, build, true))
	must(wb.Sim().Start(ctx))
	must(deck.WaitRunning(ctx, 5*time.Minute))

	// The Hardware tab is where the board draws its own screen, which is the
	// whole reason for making this node a T-Deck.
	tab, err := wb.Window(ctx, deck.Name(), "Hardware")
	must(err)

	p, err := wb.Provenance(ctx)
	must(err)
	fmt.Printf("%s is up on %s; its window is open on %s\n",
		deck.Name(), build.Describe(), tab)
	fmt.Println(p)
	fmt.Println("press enter to close the workbench")
	_, _ = fmt.Scanln()
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
