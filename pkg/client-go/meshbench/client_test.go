package meshbench

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
)

// The client against a real workbench, not a stub.
//
// A stub would answer whatever this file said it should, which is worth
// nothing: the whole risk in a client is disagreeing with the thing it drives.
// So every test here starts a headless process, drives it over its own socket,
// and stops it.
//
// The binary is built once for the package. That costs a few seconds and buys
// tests that fail when the verbs move, which is the only reason to have them.

var (
	buildOnce sync.Once
	binary    string
	buildErr  error
)

func meshbench(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "meshbench-bin")
		if err != nil {
			buildErr = err
			return
		}
		binary = filepath.Join(dir, "meshbench")
		// By package path from the module root rather than by counting "../"
		// out of this directory - which is what broke when the client moved
		// under pkg/client-go, and would break again on the next move.
		cmd := exec.Command("go", "build", "-o", binary,
			"github.com/MeshBench/meshbench/cmd/meshbench")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = errors.New(string(out))
		}
	})
	if buildErr != nil {
		t.Fatalf("building the workbench: %v", buildErr)
	}
	return binary
}

// headless starts one, with a socket of its own so tests can run beside each
// other - which is the whole point of #211 and worth exercising here.
func headless(t *testing.T, options ...Option) (*Workbench, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	options = append([]Option{
		Binary(meshbench(t)),
		Socket(filepath.Join(t.TempDir(), "control.sock")),
		StartTimeout(90 * time.Second),
	}, options...)

	wb, err := Headless(ctx, options...)
	if err != nil {
		t.Fatalf("starting a headless workbench: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })
	return wb, ctx
}

func TestItConnectsAndSaysWhatItIs(t *testing.T) {
	wb, _ := headless(t)
	h := wb.Hello()
	if h.Mode != "headless" {
		t.Errorf("mode is %q, want headless", h.Mode)
	}
	if h.Protocol == 0 || h.Version == "" || h.Verbs == 0 || h.PID == 0 {
		t.Errorf("hello is missing something a client needs: %+v", h)
	}
	if !wb.Headless() {
		t.Error("Headless() disagrees with the mode it reported")
	}
}

// Build a network from nothing, which is example 1 from #209 without the
// hardware.
func TestBuildingANetworkFromNothing(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	nodes, err := wb.Nodes().PlaceMany(ctx, []Placement{
		{Name: "R1", Kind: SimpleRepeater, Lat: 56.20, Lon: -3.20},
		{Name: "R2", Kind: SimpleRepeater, Lat: 56.12, Lon: -3.02},
		{Name: "C1", Kind: Companion, Lat: 56.19, Lon: -3.17},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("placed %d nodes, want 3", len(nodes))
	}

	list, err := wb.Nodes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("the network holds %d nodes, want 3", len(list))
	}
	// The values placed, read back - a client that silently dropped a
	// parameter would still have produced three nodes.
	byName := map[string]NodeInfo{}
	for _, n := range list {
		byName[n.Name] = n
	}
	if c := byName["C1"]; c.Kind != Companion || c.Lat < 56.18 || c.Lat > 56.20 {
		t.Errorf("C1 came back as %+v", c)
	}

	if err := wb.Nodes().Delete(ctx, "R2"); err != nil {
		t.Fatal(err)
	}
	if list, err = wb.Nodes().List(ctx); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("after deleting one, %d nodes remain, want 2", len(list))
	}
}

// What every node is doing, which is what every wait is built on. It used to
// be unaskable from outside the window: nodes.stats answered with a count and
// put the rows in the snapshot, where only a panel could reach them.
func TestNodeStatsCarryTheRowsAndNotJustACount(t *testing.T) {
	wb, ctx := headless(t, Fixture("fife-strict"))
	stats, err := wb.NodeStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	list, err := wb.Nodes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != len(list) {
		t.Fatalf("%d stat rows for %d nodes", len(stats), len(list))
	}
	first := stats[0]
	if first.Name == "" {
		t.Error("a stat row with no node name is a row nobody can use")
	}
	// Nothing has been started, so every one of them says so - and says it as
	// a state rather than only as a boolean, because "stopped" and "changing
	// firmware" are not the same answer.
	if first.Running || first.State != "stopped" {
		t.Errorf("nothing was started and %s reports %q", first.Name, first.State)
	}
}

// #216: nodes.place took no board, so a script could build a mesh and not
// build the one it wanted.
func TestPlacingANodeOnABoard(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	deck, err := wb.Nodes().Place(ctx, Placement{
		Name: "Deck", Kind: Companion, Lat: 56.19, Lon: -3.17,
		Board: BoardLilyGoTDeck,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := deck.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Board != BoardLilyGoTDeck {
		t.Fatalf("placed as a T-Deck and came back as %q", info.Board)
	}

	// A board is physics - the transmit ceiling, the noise figure, the
	// battery - so a name nothing matches refuses rather than falling back to
	// something plausible.
	_, err = wb.Nodes().Place(ctx, Placement{
		Name: "Wrong", Lat: 56, Lon: -3, Board: Board("LilyGo T-Deck Pro Max")})
	if !errors.Is(err, ErrBadParams) {
		t.Errorf("a board nobody has gave %v, want ErrBadParams", err)
	}
}

func TestChangingWhatANodeIs(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	n, err := wb.Nodes().Place(ctx, Placement{Name: "Deck", Lat: 56, Lon: -3})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.SetBoard(ctx, BoardHeltecV3); err != nil {
		t.Fatal(err)
	}
	info, err := n.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Board != BoardHeltecV3 {
		t.Fatalf("set to a Heltec and reads as %q", info.Board)
	}
}

// Half a deletion leaves a scenario nobody described.
func TestABadNameDeletesNothing(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B", "C"} {
		if _, err := wb.Nodes().Place(ctx, Placement{Name: name, Lat: 56, Lon: -3}); err != nil {
			t.Fatal(err)
		}
	}
	if err := wb.Nodes().Delete(ctx, "A", "Nowhere"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting a name that is not there gave %v, want ErrNotFound", err)
	}
	list, err := wb.Nodes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("a refused delete removed something anyway: %v", names(list))
	}
}

// The client says so, rather than letting twelve verbs refuse in a row.
func TestAWindowVerbRefusesHeadless(t *testing.T) {
	wb, ctx := headless(t)
	if _, err := wb.Window(ctx, "anything", "Hardware"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("opening a window headless gave %v, want ErrUnavailable", err)
	}
}

func TestKeepDeletesTheComplement(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"A", "B", "C", "D"} {
		if _, err := wb.Nodes().Place(ctx, Placement{
			Name: n, Lat: 56.2, Lon: -3.2}); err != nil {
			t.Fatal(err)
		}
	}
	if err := wb.Nodes().Keep(ctx, "B", "D"); err != nil {
		t.Fatal(err)
	}
	list, err := wb.Nodes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "B" || list[1].Name != "D" {
		t.Fatalf("kept %v, want B and D", names(list))
	}
}

// The clock, and that a run of simulated time is a run of simulated time.
func TestTheClockAdvancesAndStops(t *testing.T) {
	wb, ctx := headless(t, Fixture("fife-strict"))
	sim := wb.Sim()

	before, err := sim.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Playing {
		t.Fatal("a fresh session is already playing")
	}
	if err := sim.Run(ctx, 2*time.Second, time.Minute); err != nil {
		t.Fatal(err)
	}
	after, err := sim.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Playing {
		t.Error("Run returned while the clock was still going")
	}
	if got := after.NowMs - before.NowMs; got < 2000 {
		t.Errorf("two simulated seconds advanced the clock by %d ms", got)
	}
}

// Determinism is a feature, and the client must not be the thing that breaks
// it: the same seed and the same scenario reach the same moment.
func TestTheSameSeedReachesTheSameState(t *testing.T) {
	run := func() SimState {
		wb, ctx := headless(t, Fixture("fife-strict"), Seed(4242))
		if err := wb.Sim().SetSeed(ctx, 4242); err != nil {
			t.Fatal(err)
		}
		if err := wb.Sim().Run(ctx, 3*time.Second, time.Minute); err != nil {
			t.Fatal(err)
		}
		st, err := wb.Sim().State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	a, b := run(), run()
	if a.NowMs != b.NowMs || a.Events != b.Events || a.Seed != b.Seed {
		t.Fatalf("two runs of one seed differ:\n  %+v\n  %+v", a, b)
	}
}

// The refusals, by kind rather than by matching prose.
func TestRefusalsAreTyped(t *testing.T) {
	wb, ctx := headless(t)

	err := wb.Do(ctx, "no.such.verb", nil)
	if !errors.Is(err, ErrUnknownVerb) {
		t.Errorf("an unknown verb gave %v, want ErrUnknownVerb", err)
	}
	// And the workbench's own words survive, which is what a person reads.
	if err == nil || !strings.Contains(err.Error(), "no.such.verb") {
		t.Errorf("the refusal does not name the verb: %v", err)
	}

	_, err = wb.Nodes().Get(ctx, "Nowhere")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing node gave %v, want ErrNotFound", err)
	}

	// A window verb in a session with no window: available, and refused.
	err = wb.Do(ctx, "panel.open", map[string]any{"name": "Map"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("a window verb gave %v, want ErrUnavailable", err)
	}
}

// Attach does not own what it did not start. A script that closed the
// workbench somebody was looking at by returning from a function would be a
// fault with no way back.
func TestAttachDoesNotOwnTheProcess(t *testing.T) {
	wb, ctx := headless(t)
	socket := wb.Hello().Socket

	second, err := Attach(ctx, Socket(socket))
	if err != nil {
		t.Fatalf("attaching beside a running session: %v", err)
	}
	if err := second.Stop(); err == nil {
		t.Error("Attach's connection claimed to own the process")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	// The first is still there.
	if _, err := wb.Describe(ctx); err != nil {
		t.Fatalf("closing an attached client took the workbench with it: %v", err)
	}
}

// Two at once, which is the case one socket per user made impossible.
// The transport Windows uses, driven end to end from here.
//
// Windows has no unix socket a Python client can reach, so it gets loopback
// TCP with a token. That would otherwise be a path nobody on this project ever
// runs - the lab is Linux and so is CI - which is the definition of code that
// is broken and nobody knows.
func TestTheLoopbackTransport(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	// Its own rendezvous file, the way launch gives one to a session it starts:
	// two at once would otherwise overwrite each other's port and token.
	t.Setenv(control.RendezvousEnv, filepath.Join(dir, "control.json"))

	wb, err := Headless(ctx, Binary(meshbench(t)), Socket("tcp"),
		StartTimeout(90*time.Second))
	if err != nil {
		t.Fatalf("starting over loopback: %v", err)
	}
	t.Cleanup(func() { _ = wb.Close() })

	if !strings.HasPrefix(wb.Hello().Socket, "tcp:") {
		t.Fatalf("asked for tcp and it answered on %q", wb.Hello().Socket)
	}
	// And it is a workbench, not merely a socket: the API works over it.
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := wb.Nodes().Place(ctx, Placement{Name: "A", Lat: 56, Lon: -3}); err != nil {
		t.Fatal(err)
	}
	list, err := wb.Nodes().List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("over loopback: %d nodes, %v", len(list), err)
	}
}

func TestTwoSessionsAtOnce(t *testing.T) {
	a, actx := headless(t)
	b, bctx := headless(t)
	if a.Hello().Socket == b.Hello().Socket {
		t.Fatal("two sessions took the same socket")
	}
	if err := a.Project().New(actx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Nodes().Place(actx, Placement{Name: "OnlyInA", Lat: 56, Lon: -3}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Nodes().Get(bctx, "OnlyInA"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a node placed in one session appeared in the other: %v", err)
	}
}

// A wait that runs out says what it was waiting for and what it last saw.
// "timeout" in a CI log tells whoever reads it nothing.
func TestATimeoutSaysWhatItWasWaitingFor(t *testing.T) {
	wb, ctx := headless(t)
	if err := wb.Project().New(ctx, ""); err != nil {
		t.Fatal(err)
	}
	n, err := wb.Nodes().Place(ctx, Placement{Name: "Quiet", Lat: 56, Lon: -3})
	if err != nil {
		t.Fatal(err)
	}
	err = n.WaitRunning(ctx, 600*time.Millisecond)
	var to *Timeout
	if !errors.As(err, &to) {
		t.Fatalf("waiting for firmware that never starts gave %v", err)
	}
	if !strings.Contains(to.What, "Quiet") {
		t.Errorf("the timeout does not say what it waited for: %v", to)
	}
	if to.Last == "" {
		t.Errorf("the timeout does not say what it last saw: %v", to)
	}
}

func names(ns []NodeInfo) []string {
	out := make([]string, 0, len(ns))
	for _, n := range ns {
		out = append(out, n.Name)
	}
	return out
}
