// Running one cell of the matrix: one arm, one seed, start to finish.
//
// The long one, because a cell is the whole job - provision every node, bring
// the firmware up, send the traffic, wait for it to settle, and count what
// arrived.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/mesh/proto"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// contentionClasses is what an arm counts as a collision: the miss classes
// where traffic lost to traffic rather than to distance. A quiet miss is not
// one of them, and neither is a node deafened by its own transmitter, which
// is timing rather than contention.
var contentionClasses = map[engine.Class]bool{
	engine.ClassInterference: true,
	engine.ClassCollision:    true,
	engine.ClassReceiverBusy: true,
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
	old := os.Getenv("MESHBENCH_NODEFS")
	_ = os.Setenv("MESHBENCH_NODEFS", fs)
	defer func() { _ = os.Setenv("MESHBENCH_NODEFS", old) }()

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
		n = WithFirmware(n, SweepArm{
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
			text := e.cellText(arm, seed)
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
	}

	// Collisions, and the shape of the flood, both off the ledger.
	//
	// Collisions had nothing counting them at all: the scoreboard has no field
	// for them, so Collided stayed zero however hard the arms collided, and a
	// zero that is never written looks exactly like a channel nobody contended
	// for. It is a miss that would have decoded on its own and did not, which
	// is the engine's own account of capture rather than a rule on top of it.
	//
	// All three contention causes, off the class rather than off the detail
	// sentence: a packet beaten by a louder one, one whose symbols a collision
	// destroyed, and one that arrived at a demodulator already following
	// somebody else are all traffic losing to traffic, and matching a phrase
	// counted only the first of them.
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
		case ev.Kind == "miss" && contentionClasses[engine.EventClass(ev)]:
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
