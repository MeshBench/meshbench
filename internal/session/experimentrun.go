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
	"sort"
	"strings"

	"time"

	"github.com/MeshBench/meshbench/internal/companion/proto"
	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/provider"
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

	// The store only ticks - and so only redraws the simulation - while it
	// believes something is playing. A sweep is the longest thing this
	// application does, and it used to run with the clock at zero and the map
	// still, which is indistinguishable from a hung one.
	_, _ = st.Do(context.Background(), "sim.play", nil)
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
		_, _ = st.Do(context.Background(), "sim.pause", nil)
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

// runArm is one cell: one configuration at one seed, on real firmware.
func (s *Sim) runArm(ctx context.Context, e *experiment, arm ExpArm, seed uint64,
	nodes []scenario.Node) ExpResult {

	out := ExpResult{Arm: arm.Label, Seed: seed}
	began := time.Now()

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
	// Published while this cell runs, so the workbench draws the run somebody
	// started: the clock advances, the map shows traffic, the tables fill. It
	// is still this cell's own engine with its own storage - the isolation that
	// keeps one arm from inheriting the previous arm's settings is untouched.
	s.bench.take(eng)
	defer s.bench.take(nil)
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
	e.stage(arm, seed, fmt.Sprintf("%d of %d firmware attached", started, wanted))

	// Provision every node before the run.
	//
	// This is the step that decides whether anything relays. A node that has
	// not been told its regions holds none, so it forwards nothing and reports
	// no error - which is what four cells of zeros looked like. The commands
	// are the same ones ProvisioningFor shows in the node panel, so what the
	// operator reads and what the arm sends cannot drift apart.
	var refused []string
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil {
			continue
		}
		// Not a companion.
		//
		// Provisioning speaks the repeater CLI and a companion build does not
		// take those commands - the old workbench recorded exactly this, and it
		// is why a companion reported 0 MHz rather than the scenario's channel.
		// Worse here than there: typing at one closed its console, the node
		// exited, and every cell of the sweep failed on the first companion it
		// reached. What a companion needs is sent through the protocol that can
		// actually set it, once its session is claimed.
		if n.Kind == scenario.Companion {
			continue
		}
		// The session's settings with this arm written over them.
		//
		// Two faults in one line before this. The settings were the defaults,
		// whatever the Provisioning panel said - the same fault provisionLines
		// was written to fix at start-up, missed here - so a study that turned
		// a setting on compared two cells that both had it off. And the arm's
		// own settings reached nothing at all, so an arm varying loop detection
		// was a label with no effect behind it. Both ran cleanly and reported
		// no difference, which is the worst way for this to fail.
		for _, line := range s.provisionLinesFor(n, arm) {
			if line.Comment {
				continue
			}
			if err := en.Firmware.Bridge.Type([]byte(line.Command + "\r\n")); err != nil {
				// Counted, not fatal.
				//
				// A companion's serial port speaks the binary companion
				// protocol rather than the text CLI, so typing at one can close
				// its console - and that is not a reason to throw the cell away,
				// because a sender is driven through that protocol a moment
				// later anyway. The old workbench recorded the error and carried
				// on; this failed all eight cells of a sweep on the first
				// companion it reached.
				refused = append(refused, n.Name)
				break
			}
		}
	}
	// Said rather than swallowed. A cell where a third of the mesh refused its
	// settings is not a cell to compare against one where none did, and the
	// count is the only place that shows.
	if len(refused) > 0 {
		e.mu.Lock()
		e.logf("%s %d: %d nodes refused provisioning (%s%s)", arm.Label, seed,
			len(refused), strings.Join(refused[:min(3, len(refused))], ", "),
			map[bool]string{true: ", ...", false: ""}[len(refused) > 3])
		e.mu.Unlock()
	}
	// The senders are claimed before anything settles.
	//
	// A companion does not answer a tick until its app session has started, so
	// an engine stepped before that waits on it for ever - which is exactly
	// where this stalled: 159 of 159 attached, provisioned, and then the first
	// settle never returned. The old workbench connects before it settles and
	// records why: "claiming a sender steps the engine".
	e.stage(arm, seed, "claiming senders")

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
	// The handshake has to land before anything else is said.
	//
	// Everything a companion is told waits on the reply to AppStart, and a
	// reply needs time to move: sending the configuration in the same breath
	// put frames in front of a firmware that had not finished starting, and it
	// left the bridge - "no node attached" on the very next step.
	e.stage(arm, seed, "senders claimed, handshaking")
	if err := stepFor(ctx, eng, 500*time.Millisecond); err != nil {
		out.Err = "companion handshake: " + err.Error()
		return out
	}

	// And what each needs to be on the mesh's channel at all.
	//
	// A companion's radio preferences start empty: nothing has told it, because
	// the CLI it cannot take is the only thing that told anything else. An
	// unconfigured sender is not on the scenario's frequency, so it originates
	// onto a channel no repeater is listening to and the cell measures nothing
	// while reporting no error.
	for name := range sessions {
		en, _ := eng.NodeByName(name)
		for _, msg := range companionSetup(nodeNamed(nodes, name), arm, e) {
			if err := en.Firmware.Bridge.Type(compFrame(msg)); err != nil {
				out.Err = "configuring " + name + ": " + err.Error()
				return out
			}
		}
	}
	e.stage(arm, seed, "senders configured, settling")
	// The commands are answered by firmware, which only runs when time moves.
	if err := stepFor(ctx, eng, 2*time.Second); err != nil {
		out.Err = "configuring senders: " + err.Error()
		return out
	}
	e.stage(arm, seed, "provisioned, settling")
	// Let the commands be read before anything is measured: they are answered
	// by the firmware, which only runs when the engine steps.
	if err := stepFor(ctx, eng, 2*time.Second); err != nil {
		out.Err = "settling: " + err.Error()
		return out
	}

	// The arm has to be able to show its own manipulation before anything is
	// measured. A cell that runs to completion having changed nothing is the
	// most expensive result this apparatus can produce, because it looks
	// exactly like a real null.
	if bad := armDidNotReachTheChip(arm, eng, nodes); len(bad) > 0 {
		e.stage(arm, seed, "arm did not reach the chip")
		out.Err = "the arm's settings are not on the radios: " + strings.Join(bad, "; ")
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
	e.stage(arm, seed, "adverts sent, settling")
	if err := stepFor(ctx, eng, 6*time.Second); err != nil {
		out.Err = "adverts: " + err.Error()
		return out
	}
	e.stage(arm, seed, "running to send_at")

	// Where the ledger stood when this cell started, so the counting below is
	// this run's traffic and not the boot chatter of three hundred nodes.
	baseline := len(eng.Events())
	// The margin counters are zeroed here for the same reason: the adverts and
	// boot chatter above are receptions too, and a cell that inherited them
	// would report the mesh coming up rather than the flood it measured.
	eng.ResetSensitivity()
	burstMs := uint32(0)

	// Sends waiting for their moment, when the senders are staggered.
	type pendingSend struct {
		node string
		atMs uint32
		text string
	}
	var pending []pendingSend

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
			text := padTo(fmt.Sprintf("%s seed %d", arm.Label, seed), e.Bytes)
			// Staggered across the burst when asked for. An arm may override
			// the experiment's own, because "does spreading them help" is a
			// question about the arms rather than about the run.
			spread := e.SpreadMs
			if arm.SpreadMs != nil {
				spread = uint32(*arm.SpreadMs)
			}
			names := make([]string, 0, len(sessions))
			for name := range sessions {
				names = append(names, name)
			}
			sort.Strings(names) // one seed, one order
			for i, name := range names {
				at := eng.NowMs()
				if spread > 0 && len(names) > 1 {
					at += uint32(i) * spread / uint32(len(names)-1)
				}
				pending = append(pending, pendingSend{node: name, atMs: at, text: text})
				_ = at
			}
			fired, burstMs = true, eng.NowMs()
			e.stage(arm, seed, fmt.Sprintf("fired at %d ms", burstMs))
		}
		// Anything whose moment has come.
		for i := 0; i < len(pending); i++ {
			if eng.NowMs() < pending[i].atMs {
				continue
			}
			en, _ := eng.NodeByName(pending[i].node)
			if err := en.Firmware.Bridge.Type(compFrame(
				proto.SendChannelText(0, time.Unix(0, 0), pending[i].text))); err != nil {
				out.Err = "send at " + pending[i].node + ": " + err.Error()
				return out
			}
			pending = append(pending[:i], pending[i+1:]...)
			i--
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

	// Collisions, and the shape of the flood, both off the ledger.
	//
	// Collisions had nothing counting them at all: the scoreboard has no field
	// for them, so Collided stayed zero however hard the arms collided, and a
	// zero that is never written looks exactly like a channel nobody contended
	// for. It is a miss that would have decoded on its own and did not, which
	// is the engine's own account of capture rather than a rule on top of it.
	//
	// PerSecond is receptions per second after the burst. A total says which
	// arm delivered more; the shape says whether it did so in one clean wave or
	// in a long tail of retries, and those are different networks.
	if e.RunForMs > burstMs {
		secs := int((e.RunForMs-burstMs)/1000) + 1
		out.PerSecond = make([]int, secs)
	}
	for _, ev := range eng.Events()[min(baseline, len(eng.Events())):] {
		switch {
		case ev.Kind == "miss" && strings.Contains(ev.Detail, "stronger interferer"):
			out.Collided++
		case ev.Kind == "rx" && fired && ev.AtMs >= burstMs:
			if b := int((ev.AtMs - burstMs) / 1000); b >= 0 && b < len(out.PerSecond) {
				out.PerSecond[b]++
			}
		}
	}

	// What this cell's deliveries were worth in decibels. Read from the engine
	// rather than derived from the rows above, because a total cannot say how
	// close any of it came.
	sens := eng.Sensitivity()
	out.AtRisk = make([]float64, len(engine.MarginEdgesDB))
	for i := range engine.MarginEdgesDB {
		out.AtRisk[i] = sens.AtRisk(i)
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
