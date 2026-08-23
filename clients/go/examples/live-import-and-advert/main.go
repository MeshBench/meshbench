// A real deployment, pulled live, and one node made to speak.
//
//	go run ./clients/go/examples/live-import-and-advert ["node name"]
//
// Imports ScotMesh from its CoreScope feed - nodes, then a week of traffic to
// work out which regions each one holds - trims it to a workable
// neighbourhood, brings the firmware up and sends an advert from West Lomond.
//
// Two things this exists to show.
//
// The import is four steps and the last two are the ones that get skipped.
// Live.Pull does all four. Stopping after the commit gives you a mesh with
// every node in the right place and no regions on any of them, which
// transmits, relays nothing, and reports no error whatsoever. It reads as bad
// RF and it has cost people days.
//
// And you cannot type these names. The real one carries emoji either side,
// varying by who last edited it, so the script asks the workbench to search and
// takes the best answer - having first checked that the best answer is actually
// good. Taking the top result unconditionally is how you end up adverting from
// a node that merely shared a word with what you asked for.
//
// Costs: the feed is real, so this needs the network. Reading a week of
// ScotMesh traffic is around 150,000 packets and a few minutes. Firmware on the
// trimmed neighbourhood is another few. The simulator is kinder than the air -
// no multipath, no body loss, no oscillator error - so read what arrives as a
// best case.
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"github.com/MeshBench/meshbench/clients/go/meshbench"
)

const feed = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"

// neighbours is how many of its nearest to keep around the node we want.
//
// The whole deployment is around 676 nodes; running firmware on all of them is
// hours and a great deal of memory, and the question here is what one hill can
// reach. Set it to 0 to keep the lot and mean it.
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

	fmt.Println("pulling", feed)
	found, err := wb.Live().Pull(ctx, feed, 0, 30*time.Minute)
	must(err)
	fmt.Println(" ", found)

	// The commit measures every pair over real terrain and does it as a job,
	// so it is still going when Pull returns.
	must(wb.WaitIdle(ctx, 60*time.Minute))

	// Search, then look at what came back. Find refuses rather than guessing
	// when the best answer is not convincing, and says what it did find -
	// which is the difference between "you spelt it wrong" and "that node is
	// not on this mesh today".
	matches, err := wb.Nodes().Search(ctx, want, 5)
	must(err)
	for _, m := range matches {
		fmt.Printf("  %.2f  %s\n", m.Score, m.Name)
	}
	node, err := wb.Nodes().Find(ctx, want)
	must(err)
	fmt.Printf("%s is %q\n", want, node.Name())

	all, err := wb.Nodes().List(ctx)
	must(err)
	if neighbours > 0 {
		here, err := wb.Nodes().Get(ctx, node.Name())
		must(err)
		others := make([]meshbench.NodeInfo, 0, len(all))
		for _, n := range all {
			if n.Name != here.Name {
				others = append(others, n)
			}
		}
		sort.Slice(others, func(i, j int) bool {
			return kilometres(here, others[i]) < kilometres(here, others[j])
		})
		if len(others) > neighbours {
			others = others[:neighbours]
		}
		keep := []string{here.Name}
		for _, n := range others {
			keep = append(keep, n.Name)
		}
		must(wb.Nodes().Keep(ctx, keep...))
		must(wb.WaitIdle(ctx, 60*time.Minute))
		fmt.Printf("kept %d nodes, out to %.1f km\n",
			len(keep), kilometres(here, others[len(others)-1]))
	}

	// Whatever this machine holds for each role the trimmed mesh actually
	// needs. Asking the workbench which roles are unanswered beats guessing:
	// an import brings whatever kinds the deployment has, and a run refuses to
	// start until every one of them is pinned.
	needed, err := wb.Firmware().Needed(ctx)
	must(err)
	onDisk, err := wb.Firmware().OnDisk(ctx)
	must(err)
	for _, role := range needed {
		var pick meshbench.Build
		for _, b := range onDisk {
			if b.Role == role.Role && b.Board == "" {
				pick = b
			}
		}
		if pick.Version == "" {
			log.Fatalf("no %s build on this machine: meshcoresim firmware download %s",
				role.Role, role.Role)
		}
		must(wb.Firmware().UseForRole(ctx, role.Role, pick))
	}

	must(wb.Sim().Start(ctx))
	must(wb.Firmware().WaitStarted(ctx, 0))

	// Ask rather than Send and then Read: a node reads its serial input on its
	// next loop and its loop only runs when the engine steps, so reading
	// straight after sending reads the moment before the command went out.
	// That mistake looks exactly like a console that does not answer.
	reply, err := node.Console().Ask(ctx, "advert", 100)
	must(err)
	fmt.Printf("advert from %q: %q\n", node.Name(), reply)

	must(wb.Sim().Run(ctx, 2*time.Minute, 60*time.Minute))

	events, err := wb.Events().Recent(ctx, 2000)
	must(err)
	heard := map[string]bool{}
	for _, e := range events {
		if e.Class == meshbench.ClassReceived && e.From == node.Name() {
			heard[e.To] = true
		}
	}
	prov, err := wb.Provenance(ctx)
	must(err)
	fmt.Println(prov)
	fmt.Printf("%d of %d neighbours heard it directly\n", len(heard), len(all)-1)
	names := make([]string, 0, len(heard))
	for n := range heard {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println("  " + n)
	}
}

// kilometres is great-circle distance, near enough for ranking neighbours.
//
// The workbench has the accurate one and uses it for every path loss; this is
// only deciding which dozen nodes to keep, where a few metres either way
// changes nothing.
func kilometres(a, b meshbench.NodeInfo) float64 {
	const r = 6371.0
	p1, p2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dp, dl := p2-p1, (b.Lon-a.Lon)*math.Pi/180
	h := math.Sin(dp/2)*math.Sin(dp/2) +
		math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
