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

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// expCell is one arm at one seed - the unit both a fresh sweep and an
// extension run in.
type expCell struct {
	Arm  ExpArm
	Seed uint64
}

func (s *Sim) runExperiment(ctx context.Context, st *state.Store, e *experiment,
	nodes []scenario.Node) {
	var cells []expCell
	for _, arm := range e.Arms {
		for _, seed := range e.Seeds {
			cells = append(cells, expCell{arm, seed})
		}
	}
	s.runCells(ctx, st, e, nodes, cells, "running arms")
}

// runExperimentExtend runs only the cells named, appending to whatever
// results already exist rather than clearing them - "run 4 more seeds"
// narrows the interval without re-running the cells that already answered.
func (s *Sim) runExperimentExtend(ctx context.Context, st *state.Store, e *experiment,
	nodes []scenario.Node, cells []expCell) {
	s.runCells(ctx, st, e, nodes, cells, "extending")
}

func (s *Sim) runCells(ctx context.Context, st *state.Store, e *experiment,
	nodes []scenario.Node, cells []expCell, what string) {

	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		_, _ = st.Do(context.Background(), "experiment.finished", nil)
	}()

	done := 0
	for _, c := range cells {
		if ctx.Err() != nil {
			return
		}
		e.mu.Lock()
		e.status = fmt.Sprintf("%s, seed %d", c.Arm.Label, c.Seed)
		e.logf("running %s at seed %d", c.Arm.Label, c.Seed)
		e.mu.Unlock()

		r := s.runArm(ctx, e, c.Arm, c.Seed, nodes)
		e.mu.Lock()
		e.results = append(e.results, r)
		e.mu.Unlock()

		done++
		_, _ = st.Do(context.Background(), "job.progress", state.Job{
			ID: "experiment", What: what,
			Done: done, Total: len(cells)})
		// Publish as it goes: an experiment that shows nothing until the
		// last cell is one nobody can tell is working.
		_, _ = st.Do(context.Background(), "experiment.results", nil)
	}
}

// runArm is one cell: one configuration at one seed, on real firmware.
func (s *Sim) runArm(ctx context.Context, e *experiment, arm ExpArm, seed uint64,
	nodes []scenario.Node) ExpResult {

	out := ExpResult{Arm: arm.Label, Seed: seed}
	began := time.Now()

	// Density varies the layout, not the firmware: same node count, same
	// radio settings, same deployment area as every other arm - see
	// PlaceForDensity's own note on how it holds those while the mean
	// neighbour count moves. Arm and seed together pick the layout, so this
	// arm at this seed always places the same way.
	if arm.NeighbourDensity > 0 {
		nodes = PlaceForDensity(nodes, arm.NeighbourDensity, seed)
	}

	// Storage of its own, named for the arm and the seed.
	root, err := os.MkdirTemp("", "meshbench-arm-")
	if err != nil {
		out.Err = err.Error()
		return out
	}
	defer func() { _ = os.RemoveAll(root) }()
	// Created, not just named: a node whose storage directory does not exist
	// starts and exits, and the attach still reports success because the
	// socket handshake happened first.
	fs := filepath.Join(root, "nodefs")
	if err := os.MkdirAll(fs, 0o755); err != nil {
		out.Err = err.Error()
		return out
	}
	old := os.Getenv("MESHCORESIM_NODEFS")
	_ = os.Setenv("MESHCORESIM_NODEFS", fs)
	defer func() { _ = os.Setenv("MESHCORESIM_NODEFS", old) }()

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
	started, wanted := 0, 0
	if err := eng.AttachNativeProgress(ctx, seed, func(done, total int) {
		started, wanted = done, total
	}); err != nil {
		out.Err = "starting firmware: " + err.Error()
		return out
	}
	// Nothing attached is not a run. It reported four cells of zeros without
	// this, which reads as a measurement rather than as a cell that never
	// started.
	if wanted == 0 || started == 0 {
		out.Err = fmt.Sprintf(
			"no firmware attached (%d of %d): the arm measured nothing", started, wanted)
		return out
	}
	out.Firmware = started

	// Provision every node before the run.
	//
	// This is the step that decides whether anything relays. A node that has
	// not been told its regions holds none, so it forwards nothing and reports
	// no error - which is what four cells of zeros looked like. The commands
	// are the same ones ProvisioningFor shows in the node panel, so what the
	// operator reads and what the arm sends cannot drift apart.
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		for _, line := range ProvisioningFor(n) {
			if line.Comment {
				continue
			}
			if err := en.Firmware.Bridge.Type([]byte(line.Command + "\r\n")); err != nil {
				out.Err = "provisioning " + n.Name + ": " + err.Error()
				return out
			}
		}
	}
	// Let the commands be read before anything is measured: they are answered
	// by the firmware, which only runs when the engine steps.
	if err := stepFor(ctx, eng, 2*time.Second); err != nil {
		out.Err = "settling: " + err.Error()
		return out
	}

	// Advert every node before the flood.
	//
	// Two reasons, and the second is why every seed of an arm returned
	// identical numbers. Adverts populate each node's idea of its neighbours,
	// so a flood into a silent mesh is not the mesh anybody runs. And they are
	// the traffic that collides: which adverts survive depends on the boot
	// stagger, which is derived from the seed, so without them the whole run
	// is deterministic and the spread being reported is one draw repeated.
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		if err := en.Firmware.Bridge.Type([]byte("advert\r\n")); err != nil {
			out.Err = "advert at " + n.Name + ": " + err.Error()
			return out
		}
	}
	if err := stepFor(ctx, eng, 6*time.Second); err != nil {
		out.Err = "adverts: " + err.Error()
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
		if err := en.Firmware.Bridge.Type(compFrame(proto.AppStart("meshbench"))); err != nil {
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
					compFrame(proto.SendChannelText(0, time.Unix(0, 0), text))); err != nil {
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
		// Paced to real time.
		//
		// Real firmware is a real process: it boots, waits and retries on the
		// wall clock, not on simulated time. Stepping as fast as the engine
		// will go ran ninety seconds of simulated time in three seconds of
		// real time, so every node was still booting when the run ended - and
		// the cell reported zero transmissions and no error, which reads as a
		// result rather than as a race.
		if d := time.Duration(eng.NowMs())*time.Millisecond - time.Since(began); d > 0 {
			time.Sleep(min(d, 50*time.Millisecond))
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
		out.PayloadAirtimeMs += v.AirtimePayloadMs
		out.OverheadAirtimeMs += v.AirtimeOverheadMs
		out.RedundantAirtimeMs += v.AirtimeRedundantMs
	}
	// Wall time, not simulated time: the seeds-needed estimate has to say how
	// long more seeds actually take to run, and firmware paced to real time is
	// real time, not the run's own clock.
	out.WallMs = float64(time.Since(began).Milliseconds())
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
		// Same ID as the one stamped at start: the inputs have not changed, so
		// the manifest an operator copied before the run still names this file,
		// now with the results it was missing.
		id := stampExperimentID(w, s, e)
		if _, err := e.saveManifest(s); err != nil {
			w.Say("experiment: could not write its manifest: " + err.Error())
		}
		if warn != "" {
			w.Say(fmt.Sprintf("experiment %s finished, %d runs - %s", id, n, warn))
		} else {
			w.Say(fmt.Sprintf("experiment %s finished: %d runs", id, n))
		}
		return map[string]any{"runs": n, "warning": warn, "id": id}, nil
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
