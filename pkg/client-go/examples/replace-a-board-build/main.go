// Build a board image from its own repository and put the new one on a node.
//
//	WADAMESH=~/src/wadamesh go run ./pkg/client-go/examples/replace-a-board-build
//
// Run it again after a change and it reuses the session already open: pause,
// swap the firmware, delete the build it replaced, carry on. Needs a display,
// and leaves the window up on the node's Hardware tab so you can watch it boot.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/pkg/client-go/meshbench"
)

const (
	// Every environment wadamesh defines ends in _touch. WADAMESH_ENV picks
	// a different board.
	defaultPIOEnv = "LilyGo_TDeck_companion_radio_touch"
	node          = "Bench"
	board         = meshbench.BoardLilyGoTDeck
	role          = meshbench.RoleCompanionRadio
)

func main() {
	repo := os.Getenv("WADAMESH")
	if repo == "" {
		repo = filepath.Join(os.Getenv("HOME"), "src", "wadamesh")
	}
	pioEnv := os.Getenv("WADAMESH_ENV")
	if pioEnv == "" {
		pioEnv = defaultPIOEnv
	}
	image := filepath.Join(repo, ".pio", "build", pioEnv, "firmware.bin")

	ctx := context.Background()
	build := exec.CommandContext(ctx, "pio", "run", "-e", pioEnv) //nolint:gosec // WADAMESH names the checkout on purpose
	build.Dir, build.Stdout, build.Stderr = repo, os.Stdout, os.Stderr
	must(build.Run())

	// Not closed on the way out: Close owns the process it started, and the
	// point here is to leave the window up.
	wb, err := meshbench.AttachOrLaunch(ctx)
	must(err)

	// Pause first. Swapping firmware under a running clock stops the node
	// while its neighbours carry on transmitting, and nothing accounts for
	// the gap.
	must(wb.Sim().Pause(ctx))

	if _, err := wb.Nodes().Get(ctx, node); err != nil {
		must(wb.Project().New(ctx, "Fife"))
		_, err := wb.Nodes().Place(ctx, meshbench.Placement{
			Name: node, Kind: meshbench.Companion,
			Lat: 56.20, Lon: -3.20, Board: board,
		})
		must(err)
	}
	n := wb.Node(node)
	old, hadOne, err := n.Build(ctx)
	must(err)

	// No label, so it is stamped with the time: two runs are two builds rather
	// than one quietly overwriting the other.
	fresh, err := wb.Firmware().Import(ctx, image, role, board, "")
	must(err)
	must(n.SetFirmware(ctx, fresh, true)) // stops it, provisions it, starts it
	must(n.WaitRunning(ctx, 0))

	// Only now it is on the new one. A pin nothing can honour does not fail
	// until the node next starts.
	if hadOne && old.Version != fresh.Version {
		_, err := wb.Firmware().Delete(ctx, old)
		must(err)
	}

	_, err = wb.Window(ctx, node, meshbench.TabHardware)
	must(err)
	must(wb.Sim().Play(ctx))
	must(wb.Sim().Run(ctx, 30*time.Second, 30*time.Minute))
	fmt.Printf("%s is running %s\n", node, fresh.Version)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
