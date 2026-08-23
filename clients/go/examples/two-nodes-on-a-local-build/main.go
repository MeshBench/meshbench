// A fixture trimmed to two, both on a build from a MeshCore checkout - and
// re-runnable without clearing anything down.
//
//	go run ./clients/go/examples/two-nodes-on-a-local-build ~/src/MeshCore
//
// The interesting half is the second run. It attaches to the workbench the
// first one left, stops the clock, rebuilds, repoints the nodes and starts
// again, rather than opening a fresh session and paying for the fixture twice.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

// Outskirts of Glasgow, and Glenrothes.
var keep = map[string][2]float64{
	"Glasgow-Outskirts": {55.8720, -4.3300},
	"Glenrothes":        {56.1980, -3.1780},
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: two-nodes-on-a-local-build <path to MeshCore>")
	}
	checkout := os.Args[1]

	ctx := context.Background()
	// The one the last run left, or a new one.
	wb, err := meshbench.AttachOrHeadless(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	// Stop the clock before anything else: a no-op on a fresh session, and the
	// thing that makes the second run safe on a live one.
	must(wb.Sim().Pause(ctx))

	nodes, err := wb.Nodes().List(ctx)
	must(err)
	if len(nodes) == 0 {
		must(wb.Project().Open(ctx, "fife-strict"))
		// Everything the two names do not cover, in one rebuild.
		want := make([]string, 0, len(keep))
		for name := range keep {
			want = append(want, name)
		}
		sort.Strings(want)
		must(wb.Nodes().Keep(ctx, want...))
		// Whichever of them the fixture already had is moved; the rest are
		// placed. Keep refuses a name it does not hold, so this asks first.
		have, err := wb.Nodes().List(ctx)
		must(err)
		for _, name := range want {
			at := keep[name]
			if containsName(have, name) {
				must(wb.Node(name).Move(ctx, at[0], at[1]))
				continue
			}
			_, err := wb.Nodes().Place(ctx, meshbench.Placement{
				Name: name, Kind: meshbench.Companion, Lat: at[0], Lon: at[1]})
			must(err)
		}
		must(wb.WaitIdle(ctx, 10*time.Minute))
	}

	// Both roles from one build, deliberately. A locally built repeater
	// compiled against a stale shim once answered console output with 0x06
	// where the host expects 0x07: it connected, misbehaved and exited. Two
	// arms built at different moments measure the build process, not the
	// firmware.
	built, err := wb.Firmware().BuildAndWait(ctx, checkout, 30*time.Minute)
	must(err)
	if len(built) == 0 {
		log.Fatal("the build produced nothing the library can see")
	}

	nodes, err = wb.Nodes().List(ctx)
	must(err)
	for _, n := range nodes {
		role := meshbench.RoleSimpleRepeater
		if n.Kind == meshbench.Companion {
			role = meshbench.RoleCompanionRadio
		}
		for _, b := range built {
			if b.Role == role {
				must(wb.Node(n.Name).SetFirmware(ctx, b, true))
			}
		}
	}

	must(wb.Sim().Start(ctx))
	must(wb.Firmware().WaitStarted(ctx, 10*time.Minute))
	p, err := wb.Provenance(ctx)
	must(err)
	fmt.Printf("%d nodes on a build from %s\n", len(nodes), checkout)
	fmt.Println(p)
}

func containsName(ns []meshbench.NodeInfo, name string) bool {
	for _, n := range ns {
		if n.Name == name {
			return true
		}
	}
	return false
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
