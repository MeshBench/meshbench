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
	wb, err := meshbench.AttachOrLaunch(ctx)
	must(err)
	defer func() { _ = wb.Close() }()

	// Stop the clock before anything else: a no-op on a fresh session, and the
	// thing that makes the second run safe on a live one.
	must(wb.Sim().Pause(ctx))

	nodes, err := wb.Nodes().List(ctx)
	must(err)
	// Whether the mesh is already the one this example is about, not whether
	// the session is empty. A launched workbench is never empty: it opens its
	// own default fixture, which is 311 nodes - so "is it empty" was always
	// false, the trim below never ran, and this put a local build on a
	// national network and reported it as though that had been the plan.
	already := len(nodes) == len(keep)
	for _, n := range nodes {
		if _, want := keep[n.Name]; !want {
			already = false
			break
		}
	}
	if !already {
		must(wb.Project().Open(ctx, "fife-strict"))
		// Everything the two names do not cover, in one rebuild.
		want := make([]string, 0, len(keep))
		for name := range keep {
			want = append(want, name)
		}
		sort.Strings(want)
		// Put them where they belong first, then delete the rest. Keep is
		// all-or-none by design, so naming a node that is not there yet refuses
		// and removes nothing - and one of these two is never in the fixture, so
		// the trim refused on every run that reached it. The comment here used to
		// say it asked first, directly under the line that did not.
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
		must(wb.Nodes().Keep(ctx, want...))
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
