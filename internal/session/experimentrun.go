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
	"os"
	"path/filepath"

	"time"

	"github.com/A13xB0/meshcoresim/internal/companion/proto"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

func (s *Sim) runExperiment(ctx context.Context, st *state.Store, e *experiment,
	nodes []scenario.Node) {

	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		_, _ = st.Do(context.Background(), "experiment.finished", nil)
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
			_, _ = st.Do(context.Background(), "job.progress", state.Job{
				ID: "experiment", What: "running arms",
				Done: done, Total: e.runsTotal()})
		}
	}
}

// runArm is one cell: one configuration at one seed, on real firmware.
func (s *Sim) runArm(ctx context.Context, e *experiment, arm ExpArm, seed uint64,
	nodes []scenario.Node) ExpResult {

	out := ExpResult{Arm: arm.Label, Seed: seed}

	// Storage of its own, named for the arm and the seed.
	root, err := os.MkdirTemp("", "meshbench-arm-")
	if err != nil {
		out.Err = err.Error()
		return out
	}
	defer os.RemoveAll(root)
	old := os.Getenv("MESHCORESIM_NODEFS")
	_ = os.Setenv("MESHCORESIM_NODEFS", filepath.Join(root, "nodefs"))
	defer os.Setenv("MESHCORESIM_NODEFS", old)

	eng := engine.New(s.terrain(), engine.Config{
		FreqMHz: 869.618, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
		ExcessPathLossDB: s.excessLossDB,
	})
	defer func() { _ = eng.Close() }()

	senders := map[string]bool{}
	for _, n := range nodes {
		n = withFirmware(n, SweepArm{
			RepeaterVersion:  arm.RepeaterVersion,
			CompanionVersion: arm.CompanionVersion,
		})
		eng.Add(n, nil)
		for _, want := range e.Senders {
			if n.Name != want {
				continue
			}
			// A sender has to be a companion. Firing at a repeater by
			// injecting bytes at the radio does not make its firmware
			// originate anything: the first run of this measured four cells
			// of zero and reported no error, which is the shape of failure
			// this whole apparatus exists to avoid.
			if n.Kind != scenario.Companion {
				out.Err = want + " is a " + string(n.Kind) +
					"; a message is originated by a companion"
				return out
			}
			senders[want] = true
		}
	}
	if len(senders) == 0 {
		out.Err = "none of the senders are in this scenario"
		return out
	}

	// Real MeshCore, because an arm that pins a firmware version and then runs
	// the engine's own relay logic measures nothing about that version.
	if err := eng.AttachNativeProgress(ctx, seed, nil); err != nil {
		out.Err = "starting firmware: " + err.Error()
		return out
	}

	// A companion session per sender, which is how a message is originated:
	// the same path a phone takes.
	sessions := map[string]*compSession{}
	for name := range senders {
		en, ok := eng.NodeByName(name)
		if !ok || en.Firmware == nil {
			out.Err = name + " has no firmware after attach"
			return out
		}
		c := &compSession{node: name}
		c.release = en.Firmware.Bridge.Claim(c)
		defer c.release()
		sessions[name] = c
		if err := en.Firmware.Bridge.Type(proto.AppStart("meshbench")); err != nil {
			out.Err = "app start at " + name + ": " + err.Error()
			return out
		}
	}

	fired := false
	for eng.NowMs() < e.RunForMs {
		if ctx.Err() != nil {
			out.Err = "cancelled"
			return out
		}
		// Fired at the same simulated instant in every arm, not at the same
		// wall-clock moment: arms take different amounts of real time to boot
		// and firing on a timer compares different points of the run.
		if !fired && eng.NowMs() >= e.SendAtMs {
			text := fmt.Sprintf("%s seed %d", arm.Label, seed)
			for name := range sessions {
				en, _ := eng.NodeByName(name)
				if err := en.Firmware.Bridge.Type(
					proto.SendChannelText(0, time.Unix(0, 0), text)); err != nil {
					out.Err = "send at " + name + ": " + err.Error()
					return out
				}
			}
			fired = true
		}
		if err := eng.Step(ctx); err != nil {
			out.Err = err.Error()
			break
		}
	}
	if !fired {
		out.Err = fmt.Sprintf("the run ended at %d ms without reaching send_at_ms of %d",
			eng.NowMs(), e.SendAtMs)
	}

	for _, v := range eng.Scoreboard() {
		out.TX += v.Sent
		out.RX += v.Heard
		out.Delivered += v.UniqueDelivery
		out.Redundant += v.RedundantRelay
		out.AirtimeMs += float64(v.AirtimeMs)
	}
	return out
}

func registerExperimentDone(st *state.Store, s *Sim) {
	st.Handle("experiment.finished", func(w *state.World, _ any) (any, error) {
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
