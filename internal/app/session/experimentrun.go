// Running the arms.
//
// Each cell is its own engine with its own node storage, because a node keeps
// its settings between runs exactly as hardware does: an arm that shares
// storage with the previous one loads the previous one's settings and never
// reaches the changed default. Both arms then return identical numbers and the
// change looks inert, which is the failure this whole apparatus exists to
// prevent.
package session

import (
	"context"
	"fmt"
	"strings"

	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func (s *Sim) runExperiment(ctx context.Context, st *state.Store, e *experiment,
	nodes []scenario.Node) {

	// The store only ticks - and so only redraws the simulation - while it
	// believes something is playing. A sweep is the longest thing this
	// application does, and it used to run with the clock at zero and the map
	// still, which is indistinguishable from a hung one.
	_, _ = st.Do(ctx, "sim.play", nil)
	defer func() {
		e.mu.Lock()
		e.running = false
		// Last thing this goroutine does with the matrix: closed here so that
		// "the previous run has let go of the results" is a fact a start can
		// test rather than a guess it has to make.
		if e.done != nil {
			close(e.done)
		}
		e.mu.Unlock()
		// A stopped sweep must still pause the clock and say it finished, or
		// the map keeps running against an experiment that is over.
		done, release := finishing(ctx)
		defer release()
		_, _ = st.Do(done, "sim.pause", nil)
		_, _ = st.Do(done, "experiment.finished", nil)
	}()

	done := 0
	for _, arm := range e.Arms {
		for _, seed := range e.Seeds {
			if ctx.Err() != nil {
				return
			}
			e.mu.Lock()
			e.status = fmt.Sprintf("%s, seed %d", arm.Label, seed)
			e.logf("running %s at seed %d", arm.Label, seed)
			e.mu.Unlock()

			r := s.runArm(ctx, e, arm, seed, nodes)
			e.mu.Lock()
			e.results = append(e.results, r)
			e.mu.Unlock()

			done++
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "experiment", What: "running arms",
				Done: done, Total: e.runsTotal()})
			// Publish as it goes: an experiment that shows nothing until the
			// last cell is one nobody can tell is working.
			_, _ = st.Do(ctx, "experiment.results", nil)
		}
	}
}

// nodeNamed finds a node in the scenario by name.
func nodeNamed(nodes []scenario.Node, name string) scenario.Node {
	for _, n := range nodes {
		if n.Name == name {
			return n
		}
	}
	return scenario.Node{}
}

// companionSetup is what a sender has to be told before it can originate.
//
// Through the companion protocol, because that is the only interface a
// companion build has: the repeater CLI that configures everything else does
// not reach it. The clock first, since a message sent before it is set carries
// a timestamp from an epoch nobody else is in.
func companionSetup(n scenario.Node, arm ExpArm, e *experiment) [][]byte {
	r := n.Radio
	out := [][]byte{
		proto.SetDeviceTime(uint32(scenarioEpoch)),
		proto.SetRadioParams(uint32(r.CentreHz/1000), uint32(r.BandwidthHz),
			uint8(r.SpreadFactor), uint8(r.CodingRate+4)),
		proto.SetTxPower(uint8(n.TxPowerDBm)),
	}
	if n.Name != "" {
		out = append(out, proto.SetAdvertName(n.Name))
	}
	// The scope every message this sender originates goes out under.
	//
	// Without it a sweep sends unscoped, which is not a cosmetic difference:
	// unscoped traffic is carried by a different set of repeaters, so the run
	// measures a different network from the one that was asked for. workbench1
	// set this per send and workbench2 inherited the loop without it.
	//
	// The name is canonicalised first because a region is spelled two ways and
	// both are right - the repeater CLI takes `region put sco` while the key on
	// the wire is derived from "#sco". Send under the bare name and every
	// repeater receives the packet, computes a different key, and declines to
	// forward it, with no error at either end.
	if s := canonicalScope(e.Scope); s != "" {
		out = append(out, proto.SetDefaultScope(s, provider.RegionKey(s)))
	}
	// The arm's own path hash mode, which is a companion setting: what a
	// message carries is stamped by whoever originated it and honoured at
	// every hop, so this is the one that decides the experiment.
	if arm.PathHashMode != nil {
		out = append(out, proto.SetPathHashMode(uint8(*arm.PathHashMode)))
	}
	return out
}

// stage records where a cell has got to.
//
// Every line carries the arm and the seed, because the failure this is for is a
// cell that stops moving: without a stage the log says only which cell started,
// and a stall in attach looks exactly like a stall in the run loop.
func (e *experiment) stage(arm ExpArm, seed uint64, what string) {
	e.mu.Lock()
	e.logf("%s %d: %s", arm.Label, seed, what)
	e.mu.Unlock()
}

func registerExperimentDone(st *state.Store, s *Sim) {
	st.HandleInternal("experiment.finished", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		n := len(e.results)
		warn := e.notAResultYet()
		e.mu.Unlock()
		w.Jobs = finishJob(w.Jobs, "experiment")
		if warn != "" {
			w.Say(fmt.Sprintf("experiment finished, %d runs - %s", n, warn))
		} else {
			w.Say(fmt.Sprintf("experiment finished: %d runs", n))
		}
		return map[string]any{"runs": n, "warning": warn}, nil
	})
}

// stepFor advances the engine for a stretch of real time, which is what a
// process needs to boot: simulated time is free and wall time is not.
func stepFor(ctx context.Context, eng *engine.Engine, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if err := eng.Step(ctx); err != nil {
			return err
		}
		time.Sleep(2 * time.Millisecond)
	}
	return nil
}

// canonicalScope is the "#name" form the scope key is derived from.
//
// Empty stays empty: no scope asked for means send unscoped, which is a
// legitimate choice and not the same as sending under "#".
func canonicalScope(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return "#" + strings.TrimPrefix(s, "#")
}
