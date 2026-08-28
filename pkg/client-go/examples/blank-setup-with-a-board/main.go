// A blank setup, one companion, and its screen on show.
//
//	go run ./pkg/client-go/examples/blank-setup-with-a-board
//
// wadamesh is imported, not downloaded: it must be in the library or
// reachable through WADAMESH_IMAGE. Needs a display. It opens the node's own window on the Hardware tab at the
// end, which is the point of it.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/MeshBench/meshbench/pkg/client-go/meshbench"
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
	build, err := wb.Firmware().Find(ctx, wadamesh, board)
	if errors.Is(err, meshbench.ErrNotFound) {
		// wadamesh is imported, not in the download catalogue - import a
		// built image, named by WADAMESH_IMAGE.
		image := os.Getenv("WADAMESH_IMAGE")
		if image == "" {
			log.Fatal("wadamesh is not in the library; set WADAMESH_IMAGE " +
				"to a built image, or import one in the workbench first")
		}
		build, err = wb.Firmware().Import(ctx, image,
			meshbench.RoleCompanionRadioUSB, board, wadamesh)
	}
	must(err)

	// Applied: stop, provision, start. On a board that means an emulator,
	// which is why the wait below is generous.
	must(deck.SetFirmware(ctx, build, true))
	must(wb.Sim().Start(ctx))
	must(deck.WaitRunning(ctx, 5*time.Minute))

	// The Hardware tab is where the board draws its own screen, which is the
	// whole reason for making this node a T-Deck.
	tab, err := wb.Window(ctx, deck.Name(), meshbench.TabHardware)
	must(err)

	p, err := wb.Provenance(ctx)
	must(err)
	fmt.Printf("%s is up on %s; its window is open on %s\n",
		deck.Name(), build.Describe(), tab)
	fmt.Println(p)
	fmt.Println("press enter to close the workbench")
	// Held open for somebody looking at it, and only then. Piped or run from
	// CI there is nobody to press enter, and the read returns immediately - so
	// say which happened rather than appearing to have been dismissed.
	if st, err := os.Stdin.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
		_, _ = fmt.Scanln()
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
