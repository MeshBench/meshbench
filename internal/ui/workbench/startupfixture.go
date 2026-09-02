// What -fixture means at launch, and the two answers it used to give silently.
//
// The flag has two failure modes and they want opposite treatment. A name that
// resolves to no network at all is a command line that cannot do what it says,
// and the workbench should not start; a deliberately empty one is somebody
// asking for a workbench with nothing in it, which is a reasonable thing to
// want and a state the session has to actually be put into.
//
// Neither was true before. Both printed a line to stderr, behind the splash and
// gone by the time the window appeared, and then carried on into a session that
// came up, answered the control socket, and could not run anything: nothing was
// installed, so no engine, no tick, and no network. The first sign of it was
// some later verb failing, several steps past the cause.
package workbench

import (
	"context"
	"fmt"
	"os"
	"time"

	fixturelib "github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// refuseUnknownFixture stops the launch when -fixture named something this copy
// cannot find.
//
// Before the window and before the session, because the point is that nothing
// is left running afterwards. Resolving a name is a handful of stats and a look
// inside the binary, which is cheap enough to do on the way past; loading it is
// not, which is why the load itself still happens on a worker.
func refuseUnknownFixture(name string) {
	if err := unknownFixture(name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

// unknownFixture is that check with the exit taken out, so a test can ask it
// the question without ending the process.
func unknownFixture(name string) error {
	if name == "" {
		return nil
	}
	if _, _, err := fixturelib.Find(name); err != nil {
		return fmt.Errorf("-fixture %q: %w", name, err)
	}
	return nil
}

// startupNetwork is what the command line asked to be in place before anybody
// touches the window: the network, its seed, and whatever it was told to do to
// it once it was open.
//
// Ordered, and the order matters: sim.seed rebuilds the scenario, so a seed set
// before the open is a seed applied to an empty network and then thrown away by
// it.
type startupNetwork struct {
	st          *state.Store
	ctx         context.Context
	fixture     string
	seed        uint
	play        bool
	inject      string
	injectEvery time.Duration
}

func (n startupNetwork) run() {
	openStartupNetwork(n.ctx, n.st, n.fixture)
	if n.seed != 0 {
		if _, err := n.st.Do(n.ctx, "sim.seed",
			map[string]any{"seed": float64(n.seed)}); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
		}
	}
	if n.play {
		_, _ = n.st.Do(n.ctx, "sim.play", nil)
	}
	if n.inject == "" {
		return
	}
	_, _ = n.st.Do(n.ctx, "sim.inject", n.inject)
	if n.injectEvery > 0 {
		// Wall-clock rather than simulated time, because this exists to put
		// something on the map while a person is looking at it.
		go n.keepInjecting()
	}
}

func (n startupNetwork) keepInjecting() {
	t := time.NewTicker(n.injectEvery)
	defer t.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-t.C:
			_, _ = n.st.Do(n.ctx, "sim.inject", n.inject)
		}
	}
}

// openStartupNetwork puts a network in place, or a blank one, and says which.
//
// A blank network is installed the same way an opened one is - through
// project.new, which builds the engine and starts the tick - rather than left
// as an unbuilt session. That is the difference between a workbench somebody
// can place a node in and one where every verb refuses.
func openStartupNetwork(ctx context.Context, st *state.Store, name string) {
	if name == "" {
		startBlank(ctx, st, "-fixture was empty, so nothing was opened")
		return
	}
	if _, err := st.Do(ctx, "project.open", name); err != nil {
		// Resolvable and still unreadable: a file that is not a fixture, or one
		// with no nodes in it. Said where a person is looking rather than only
		// on stderr, and followed by a blank network, because a session that
		// refuses everything teaches nothing about which file was wrong.
		fmt.Fprintln(os.Stderr, "loading:", err)
		startBlank(ctx, st, err.Error())
	}
}

// startBlank installs the empty network and puts the reason in the status line.
func startBlank(ctx context.Context, st *state.Store, why string) {
	if _, err := st.Do(ctx, "project.new", nil); err != nil {
		fmt.Fprintln(os.Stderr, "blank network:", err)
		return
	}
	_, _ = st.Do(ctx, "ui.said", "no network loaded: "+why+
		" - open one with File, or place nodes with the map's place tool")
}
