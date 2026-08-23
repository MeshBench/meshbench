// Build a board image somewhere else, and put the new one on.
//
//	WADAMESH=~/src/wadamesh go run ./clients/go/examples/replace-a-board-build
//
// Point WADAMESH at the repository, run it, change something, run it again.
// The second run does not start over: it finds the session still up, pauses
// it, imports the new image beside the old, moves the node onto it, deletes
// the one it replaced and carries on from where the clock was. Then it puts
// the node's Hardware tab on screen, which is where you watch the thing you
// just built actually boot.
//
// Idempotent because a script you run twenty times in an afternoon is a script
// that must not clear everything down each time. Repositioning the node,
// waiting out the link warm and re-importing the terrain on every edit is
// minutes each time, and it is the reason people stop using a tool like this.
//
// The three things it gets right that are easy to get wrong.
//
// Every import is labelled with when it happened. Without a label they were
// all called "imported", each one overwrote the last, and there was no way to
// say which of two builds a node was running - or to delete the older one,
// since both were the same file.
//
// The old build goes only after the node is on the new one. Deleting a build a
// node is still pinned to leaves the pin in place, and a pin nothing can
// honour does not fail until the node next starts.
//
// And replacing firmware restarts the companion. SetFirmware with apply
// restarts it: stop, provision, start. Firmware is chosen when a node
// launches, so recording it and leaving the node on the old image is the
// control you press twice and then stop trusting.
//
// Costs: whatever your own build takes, plus a boot. The board is emulated one
// at a time on purpose - a full fixture of them will take a twelve-core
// machine down - so this example is one node and says so.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const (
	// socket is its own address, so this example and whatever else you have
	// open do not fight over one session.
	socket = "/tmp/meshbench-wadamesh.sock"
	node   = "Bench"
	role   = "companion_radio"
	board  = meshbench.BoardLilyGoTDeck
)

func main() {
	repo := env("WADAMESH", filepath.Join(os.Getenv("HOME"), "src", "wadamesh"))
	pioEnv := env("WADAMESH_ENV", "LilyGo_TDeck_companion_radio")

	ctx := context.Background()
	image, err := build(ctx, repo, pioEnv)
	must(err)

	wb, err := meshbench.AttachOrLaunch(ctx, meshbench.Socket(socket))
	must(err)
	defer func() { _ = wb.Close() }()

	attached := !wb.Owned()
	if attached {
		fmt.Println("attached to the running session")
	} else {
		fmt.Println("started a session")
	}

	// Pause before touching anything. Swapping firmware under a running clock
	// means the node is stopped and restarted while its neighbours carry on
	// transmitting, and what it misses in that window is a gap nothing
	// accounts for later.
	state, err := wb.Sim().State(ctx)
	must(err)
	must(wb.Sim().Pause(ctx))

	if _, err := wb.Nodes().Get(ctx, node); err != nil {
		must(wb.Project().New(ctx, "Fife"))
		_, err := wb.Nodes().Place(ctx, meshbench.Placement{
			Name: node, Kind: meshbench.Companion,
			Lat: 56.20, Lon: -3.20, Board: board,
		})
		must(err)
		must(wb.WaitIdle(ctx, 30*time.Minute))
	}
	n := wb.Node(node)

	// On screen before the swap rather than after it, so what you watch is the
	// board going down and coming back up on the image you just built. The tab
	// that shows it as itself: its screen, its buttons, and what its radio is
	// really doing.
	if _, err := wb.Window(ctx, node, "Hardware"); err != nil {
		// Not fatal. A headless session has no window to open, and the rest of
		// this is still worth doing.
		fmt.Println("no window to open:", err)
	}

	// What it is on now, before anything replaces it. Read first: after the
	// import the library has two and telling them apart is guesswork.
	info, err := wb.Nodes().Get(ctx, node)
	must(err)
	onDisk, err := wb.Firmware().OnDisk(ctx)
	must(err)
	var before []meshbench.Build
	for _, b := range onDisk {
		if b.Board == string(board) && b.Version == info.Firmware {
			before = append(before, b)
		}
	}

	// Stamped with the second, so a second run is a second build in the
	// library rather than a silent overwrite of the first.
	label := "wadamesh-" + time.Now().Format("20060102-150405")
	built, err := wb.Firmware().Import(ctx, image, role, string(board), label)
	must(err)
	fmt.Printf("imported %s (%d bytes)\n", built.Describe(), built.Bytes)

	// Applied, which stops the node, provisions it again and starts it - the
	// companion comes back on the new image.
	must(n.SetFirmware(ctx, built, true))
	must(n.WaitRunning(ctx, 0))
	now, err := wb.Nodes().Get(ctx, node)
	must(err)
	fmt.Printf("%s is running %s\n", node, now.Firmware)

	// Only now, and only what this run replaced. A build somebody else is
	// using keeps its own pin and is none of this script's business.
	for _, old := range before {
		if old.Version == built.Version || old.InUse != 0 {
			continue
		}
		where, err := wb.Firmware().Delete(ctx, old)
		must(err)
		fmt.Printf("deleted %s at %s\n", old.Describe(), where)
	}

	// Carry on from where the clock was rather than from zero: playing again
	// only if it was playing, or if this run is the one that built the session
	// in the first place.
	if state.Playing || !attached {
		must(wb.Sim().Play(ctx))
	}

	must(wb.Sim().Run(ctx, 30*time.Second, 30*time.Minute))
	reply, err := n.Console().Ask(ctx, "advert", 100)
	must(err)
	fmt.Printf("advert: %q\n", reply)

	prov, err := wb.Provenance(ctx)
	must(err)
	fmt.Println(prov)

	if attached {
		fmt.Printf("the session stays up at %s; run this again to replace it\n", socket)
	}
}

// build compiles the repository and hands back the image it produced.
//
// The build is the repository's business, not ours - it owns the toolchain,
// the board definitions and the flags. All this needs is the artefact, and if
// that has moved, this is the one function to change.
func build(ctx context.Context, repo, pioEnv string) (string, error) {
	// The path comes from the environment because naming the repository is what
	// this example is for. It is the operator's own machine and their own
	// checkout, and there is nothing here to escalate to.
	if st, err := os.Stat(repo); err != nil || !st.IsDir() { //nolint:gosec // WADAMESH names the checkout on purpose
		return "", fmt.Errorf("%s is not there; set WADAMESH to the repository", repo)
	}
	fmt.Printf("building %s in %s\n", pioEnv, repo)
	cmd := exec.CommandContext(ctx, "pio", "run", "-e", pioEnv) //nolint:gosec // the environment names the repository to build
	cmd.Dir = repo
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("the build failed (%w); nothing was changed", err)
	}
	out := filepath.Join(repo, ".pio", "build", pioEnv, "firmware.bin")
	if _, err := os.Stat(out); err != nil { //nolint:gosec // under the checkout the caller named
		return "", fmt.Errorf("the build reported success but %s is not there; "+
			"set WADAMESH_ENV if this repository names its environment differently", out)
	}
	return out, nil
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
