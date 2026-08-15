// Package engine is the simulation itself: firmware nodes exchanging traffic
// over the RF channel.
//
// Everything else in this project is a component of this or a view onto it. The
// terrain, the link budgets, the coverage rasters and the profiles all exist to
// answer one question — did that packet arrive, and if not, why not — and until
// something drives real firmware over the real channel, none of them is being
// asked it.
//
// The difference from a packet-level mesh simulator is the whole point. HopReach
// ports MeshCore's airtime, packet-score and retransmit formulas and reasons
// about links; this runs the firmware that contains them and puts its
// transmissions through a channel that sums waveforms. A relay decision here is
// MeshCore's own, made on an SNR that came out of a demodulator.
package engine

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// Node is one participant: its place in the world and its running firmware.
type Node struct {
	Spec scenario.Node

	// Firmware is nil for a node that does not run any — an SDR observer, or a
	// custom emitter that is only there to be interfered with.
	Firmware *firmware.Node

	// BootOffsetMs is how far ahead of the run clock this node's own clock runs,
	// standing in for having been powered on earlier than the others.
	BootOffsetMs uint32

	// The board's own figures, kept because Spec's are overwritten with the
	// effective ones as the firmware reports how it has configured its radio.
	// Without a baseline to compute from, every tick would apply the same
	// correction again to the previous tick's answer.
	baseTxPowerDBm, baseNoiseFigDB float64

	// Sent and Heard are counters for the scoreboard.
	Sent           int
	Heard          int
	UniqueDelivery int
	RedundantRelay int
	AirtimeMs      float64
}

// Event is one thing that happened, in simulated time.
//
// The timeline is built from these, and so is every explanation. A simulation
// that reports only final counts cannot answer "why did that not arrive", which
// is the only question anyone actually has.
type Event struct {
	AtMs     uint32
	Kind     string // "tx", "rx", "miss"
	From     string
	To       string
	PacketID uint64
	// MessageID is the same for every hop of one message, where PacketID is one
	// transmission. Following a message across a mesh needs the first; blaming
	// a particular relay needs the second.
	MessageID uint64
	Outcome   capture.Outcome
	SNRdB     float64
	Detail    string

	// Frame is the bytes on the air. Carried on the event so the inspector can
	// dissect what actually flew rather than a reconstruction of it — the two
	// diverge exactly when it matters, which is when something is wrong.
	//
	// Shared, not copied: the engine never mutates a frame after transmission,
	// and a hundred thousand events each owning a copy is real memory.
	Frame []byte
}

// Config is what a run needs that a scenario does not carry.
type Config struct {
	FreqMHz      float64
	SF           int
	BandwidthHz  float64
	CodingRate   int
	NoiseFigDB   float64
	ProfileStepM float64

	// StepMs is the tick. Ten milliseconds is small against a LoRa symbol at
	// any spreading factor and large enough that a hundred nodes do not spend
	// the run in socket round trips.
	StepMs uint32

	// Seed makes a run reproducible, which CLAUDE.md requires.
	Seed uint64

	// ExcessPathLossDB is the calibration term from ADR-0015: extra loss on
	// every path, fitted to real observations rather than chosen. Zero means
	// uncalibrated. Always displayed when set — a silent fudge factor is the
	// thing the validation chain exists to prevent.
	ExcessPathLossDB float64
}

// Engine owns the run.
type Engine struct {
	Terrain coverage.Terrain
	Config  Config
	Ledger  capture.Ledger

	mu    sync.Mutex
	nodes []*Node
	// builds is what the last firmware attach resolved to, so a result can
	// name the binary that produced it rather than the version somebody asked
	// for. The two are not the same thing.
	builds []Build
	events []Event
	nowMs  uint32
	packet uint64

	// eventLog, when set, receives every event as NDJSON as it is recorded.
	eventLog *eventLog

	// inFlight is what is on the air at this instant: a transmission occupies
	// the channel for its own airtime, and anything else transmitting during
	// that window is a collision rather than a separate event.
	inFlight []transmission

	// emitterNoise caches each receiver's extra floor from the emitter fleet,
	// invalidated with the link cache — emitters move exactly as often as
	// nodes do.
	emitterNoise map[int]float64

	// linkCache holds path loss between node pairs. Terrain does not move
	// during a run, and recomputing a profile per packet per pair is the
	// difference between a run that takes seconds and one that takes hours.
	linkCache map[[2]int]float64
	// classCounts is events by class, kept as they are recorded so the cards
	// that show them never walk the log.
	classCounts map[string]int

	// capture, when a run is being recorded to pcapng.
	capture *Capture

	// sens counts how close each reception ran to the demodulator's floor, so a
	// study of receiver sensitivity has something to read that aggregate
	// delivery counts do not destroy.
	sens sensitivity

	// StaggerBoot spreads node start times. On by default: started together,
	// nodes share a timer phase and their adverts collide for ever.
	StaggerBoot bool

	// seen is which payloads each receiver has already had. It is what turns a
	// delivery count into the number that matters: whether a relay reached
	// anybody who had not already heard the message.
	seen map[string]map[uint64]bool
}

type transmission struct {
	from     int
	packetID uint64
	frame    []byte
	// payload identifies the *content*, so the same message relayed by five
	// nodes is recognised as one message. Without it every hop looks like a
	// separate delivery and a flood appears to reach five times as many nodes
	// as it did.
	payload uint64
	startMs uint32
	endMs   uint32
}

// New prepares an engine.
func New(t coverage.Terrain, c Config) *Engine {
	if c.StepMs == 0 {
		c.StepMs = 10
	}
	if c.BandwidthHz == 0 {
		c.BandwidthHz = 250_000
	}
	if c.SF == 0 {
		c.SF = 10
	}
	if c.CodingRate == 0 {
		c.CodingRate = 1
	}
	if c.FreqMHz == 0 {
		c.FreqMHz = 869.525
	}
	if c.NoiseFigDB == 0 {
		c.NoiseFigDB = 6
	}
	if c.ProfileStepM == 0 {
		c.ProfileStepM = 60
	}
	return &Engine{
		Terrain: t, Config: c,
		linkCache:    map[[2]int]float64{},
		emitterNoise: map[int]float64{},
		StaggerBoot:  true,
		seen:         map[string]map[uint64]bool{},
	}
}

// Add puts a node in the world.
func (e *Engine) Add(spec scenario.Node, fw *firmware.Node) *Node {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := &Node{
		Spec:           spec,
		Firmware:       fw,
		baseTxPowerDBm: spec.TxPowerDBm,
		baseNoiseFigDB: spec.NoiseFigureDB,
	}
	e.nodes = append(e.nodes, n)
	// Terrain has not changed, but the set of pairs has.
	e.linkCache = map[[2]int]float64{}
	e.emitterNoise = map[int]float64{}
	return n
}

// Nodes is the current set.
func (e *Engine) Nodes() []*Node {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*Node, len(e.nodes))
	copy(out, e.nodes)
	return out
}

// PinFirmware changes which build a node will run the next time it starts,
// and reports how many nodes it changed.
//
// This exists because the engine keeps its own copy of every node's spec,
// taken when the network was built. A pin set anywhere else - the library
// panel, the control socket, a script - lands in the session's scenario and
// in what the interface draws, and the two of them agreed with each other
// while disagreeing with the copy that actually starts processes. The symptom
// was a library saying 273 nodes run v1.17.1 and a start that asked for
// v1.17.0, which sends whoever sees it looking in the catalogue rather than
// at the pin.
//
// A node that is already running keeps running what it started with; the pin
// applies at its next start, which is what "will run next time" means.
func (e *Engine) PinFirmware(name, version string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, nd := range e.nodes {
		if name != "" && nd.Spec.Name != name {
			continue
		}
		if nd.Spec.Firmware.Version == version {
			continue
		}
		nd.Spec.Firmware.Version = version
		n++
	}
	return n
}

// Events is everything that has happened, in order.
// EventCount is the ledger length, for callers deciding whether to resnapshot.
func (e *Engine) EventCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events)
}

func (e *Engine) Events() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Event, len(e.events))
	copy(out, e.events)
	return out
}

// EventsTail copies only the last n events, and says how many there are.
//
// The tick asked for Events() and threw away all but the tail, which is a
// copy of the whole run's history per tick - quadratic over a run's life,
// and the reason a long run's ticks grew slow.
func (e *Engine) EventsTail(n int) ([]Event, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	total := len(e.events)
	if n > total {
		n = total
	}
	out := make([]Event, n)
	copy(out, e.events[total-n:])
	return out, total
}

// EventsSince copies only the events at or after a simulated moment. Events
// arrive in time order, so the start is found by binary search rather than by
// walking the whole log.
func (e *Engine) EventsSince(fromMs uint32) []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	lo, hi := 0, len(e.events)
	for lo < hi {
		mid := (lo + hi) / 2
		if e.events[mid].AtMs < fromMs {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	out := make([]Event, len(e.events)-lo)
	copy(out, e.events[lo:])
	return out
}

// NowMs is the simulated clock.
func (e *Engine) NowMs() uint32 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nowMs
}

// Step advances the simulation by one tick.
//
// The order matters and is not arbitrary. Transmissions that finished are
// delivered first, so a node's own send completing does not race the frame it
// produced; then the firmware runs; then whatever it transmitted goes on the
// air. Running the firmware first would let a node act on a frame in the same
// instant it arrived, which no radio does.
func (e *Engine) Step(ctx context.Context) error {
	e.mu.Lock()
	e.nowMs += e.Config.StepMs
	now := e.nowMs
	e.mu.Unlock()

	if err := e.completeTransmissions(now); err != nil {
		return err
	}
	if err := e.runFirmware(ctx, now); err != nil {
		return err
	}
	return e.collectTransmissions(now)
}

// Run steps until the simulated clock reaches untilMs.
func (e *Engine) Run(ctx context.Context, untilMs uint32) error {
	for e.NowMs() < untilMs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.Step(ctx); err != nil {
			return err
		}
	}
	return nil
}

// completeTransmissions delivers anything whose airtime has elapsed.
func (e *Engine) completeTransmissions(now uint32) error {
	e.mu.Lock()
	var done, still []transmission
	for _, t := range e.inFlight {
		if now >= t.endMs {
			done = append(done, t)
		} else {
			still = append(still, t)
		}
	}
	e.inFlight = still
	concurrent := append(append([]transmission{}, still...), done...)
	senders := make([]*Node, len(done))
	for i, t := range done {
		senders[i] = e.nodes[t.from]
	}
	e.mu.Unlock()

	// The sender learns its waveform has ended before anyone hears it. This
	// call is the entire meaning of isSendComplete() on a native node — the
	// node cannot time its own transmission — and forgetting it wedged every
	// radio after its first packet: the dispatcher waited forever for a
	// completion nobody was going to send, so each node transmitted exactly
	// once in its life and then went permanently silent. A 300-node flood
	// looked like a single hop, because that is what it was.
	for i, t := range done {
		if fw := senders[i].Firmware; fw != nil {
			if err := fw.Bridge.TransmitFinished(); err != nil {
				return fmt.Errorf("engine: tx done for %s: %w", senders[i].Spec.Name, err)
			}
		}
		if err := e.deliver(t, concurrent); err != nil {
			return err
		}
	}
	return nil
}

// deliver works out who heard a finished transmission, and records why not for
// everyone who did not.
func (e *Engine) deliver(t transmission, concurrent []transmission) error {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	src := nodes[t.from]
	// The transmitter's own radio settings, not the scenario's. Two nodes on
	// different presets are on different channels, and a channel that ignores
	// that lets a UK Narrow repeater decode an Australian one.
	txPHY := e.phyOf(src.Spec)

	for i, dst := range nodes {
		if i == t.from {
			continue
		}
		if !dst.Spec.Kind.RunsFirmware() && dst.Spec.Kind != scenario.SDRObserver {
			// Emitters and their kin radiate; they do not listen.
			continue
		}

		// A receiver tuned elsewhere hears nothing of this. Not an event: it is
		// the same non-event as a signal below the floor, and it is why an
		// operator splitting a mesh across two presets sees two meshes.
		//
		// An SDR observer is exempt — it is wideband by definition, and being
		// able to watch a channel your own nodes are not on is the point of
		// having one.
		rxPHY := e.phyOf(dst.Spec)
		if dst.Spec.Kind != scenario.SDRObserver && !txPHY.sameChannel(rxPHY) {
			continue
		}
		noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec))
		// The emitter fleet's contribution, through the same terrain. This is
		// the per-receiver floor: a node beside a paging mast lives on a
		// different noise floor from one on a quiet hill.
		if extra := e.emitterNoiseAt(i); !math.IsInf(extra, -1) {
			noiseDBm = addDBm(noiseDBm, extra)
		}
		required := requiredSNRdB(txPHY.sf)

		loss, ok := e.pathLoss(t.from, i)
		if !ok {
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, Outcome: capture.OutOfRange,
				Detail: "no terrain data covers this path"})
			continue
		}

		rxDBm := src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(dst.Spec)
		snr := rxDBm - noiseDBm

		// Interference from anything else that was on the air during this
		// transmission. Not a rule that overlapping packets both fail — the
		// stronger one wins if it is far enough ahead, which is capture effect
		// and is what makes a flood behave the way it does.
		interferenceDBm := math.Inf(-1)
		for _, other := range concurrent {
			if other.packetID == t.packetID || other.from == i {
				continue
			}
			if other.endMs <= t.startMs || other.startMs >= t.endMs {
				continue
			}
			// Energy on another channel is not interference. Adding it would
			// make a mesh on a second preset degrade the first one, which is
			// the opposite of why an operator splits them.
			if !e.phyOf(nodes[other.from].Spec).sameChannel(txPHY) {
				continue
			}
			ol, ok := e.pathLoss(other.from, i)
			if !ok {
				continue
			}
			p := nodes[other.from].Spec.TxPowerDBm + gain(nodes[other.from].Spec) - ol + gain(dst.Spec)
			interferenceDBm = addDBm(interferenceDBm, p)
		}

		effective := snr
		if !math.IsInf(interferenceDBm, -1) {
			effective = rxDBm - addDBm(noiseDBm, interferenceDBm)
		}

		// A node transmitting cannot hear. LoRa is half duplex, and this is one
		// of the causes HopReach found worth reporting separately — a listener
		// missing a packet because its own transmitter was keyed is a different
		// problem from a weak signal, and has a different fix.
		deaf := false
		for _, other := range concurrent {
			if other.from == i && other.startMs < t.endMs && other.endMs > t.startMs {
				deaf = true
				break
			}
		}

		rec := capture.Reception{
			PacketID: t.packetID, FromNode: src.Spec.Name, ToNode: dst.Spec.Name,
			RSSIdBm: rxDBm, SNRdB: effective,
			Offered: rxDBm > noiseDBm-10,
		}
		// Recorded before the verdict is turned into words: the capture wants
		// the outcome code, and every receiver's view including the ones that
		// heard nothing worth reporting in the ledger.
		defer func() {
			e.mu.Lock()
			c := e.capture
			e.mu.Unlock()
			if c != nil && rec.Offered {
				c.write(t.endMs, src.Spec.Name, dst.Spec.Name, txPHY,
					rec.RSSIdBm, rec.SNRdB, rec.Outcome, rec.CRCOK, t.frame)
			}
		}()
		switch {
		case !rec.Offered:
			rec.Outcome = capture.OutOfRange
			// Not recorded. "Nothing measurable arrived" is not an event, it is
			// the absence of one — and on a country-sized network it was most of
			// the ledger: every transmission produced hundreds of rows saying
			// that physics still applies. The question it answered ("why does X
			// not hear Y") is the Link tab's job, which answers with the actual
			// budget instead of a flood of negatives. Deafness and interference
			// stay recorded: those are causes, not absences.
		case deaf:
			// Something measurable did arrive, and this node could not hear it
			// because it was transmitting. That is a different problem from a
			// weak signal and has a different fix, which is why it is separate.
			rec.Outcome = capture.NotDemodulated
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: effective, Frame: t.frame,
				Detail: "its own transmitter was keyed; LoRa is half duplex"})
		case effective < required:
			rec.Outcome = capture.NotDemodulated
			// How near it came. Interference is included deliberately: a packet
			// lost to a stronger neighbour is not one a better receiver saves,
			// and counting it as nearly-decoded would overstate what sensitivity
			// buys on exactly the crowded mesh where the question is asked.
			e.sens.note(effective-required, false)
			why := fmt.Sprintf("SNR %.1f dB against %.1f dB needed at SF%d", effective, required, e.Config.SF)
			if !math.IsInf(interferenceDBm, -1) && snr >= required {
				why = fmt.Sprintf("would have decoded at %.1f dB, lost to a stronger interferer", snr)
			}
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: effective, Frame: t.frame, Detail: why})
		default:
			rec.Demod, rec.CRCOK, rec.FirmwareSaw = true, true, true
			rec.Outcome = capture.Accepted
			e.sens.note(effective-required, true)
			dst.Heard++

			// Unique against redundant. A repeater can be busy, legal, and
			// reaching nobody who had not already heard the message — which a
			// duty-cycle figure hides completely.
			e.mu.Lock()
			if e.seen[dst.Spec.Name] == nil {
				e.seen[dst.Spec.Name] = map[uint64]bool{}
			}
			first := !e.seen[dst.Spec.Name][t.payload]
			e.seen[dst.Spec.Name][t.payload] = true
			if first {
				src.UniqueDelivery++
			} else {
				src.RedundantRelay++
			}
			e.mu.Unlock()

			detail := "first time this node heard the message"
			if !first {
				detail = "already had this message; the relay cost airtime and reached nobody new"
			}
			e.record(Event{AtMs: t.endMs, Kind: "rx", From: src.Spec.Name, To: dst.Spec.Name,
				Frame: t.frame, PacketID: t.packetID, MessageID: t.payload,
				Outcome: rec.Outcome, SNRdB: effective, Detail: detail})

			// Only a node running firmware is handed the frame. An observer
			// hears it and does nothing, which is what an observer is.
			if dst.Firmware != nil {
				if err := dst.Firmware.Bridge.Deliver(t.frame); err != nil {
					return fmt.Errorf("engine: deliver to %s: %w", dst.Spec.Name, err)
				}
			}
		}
		e.Ledger.Record(rec)

		// The capture takes every receiver's view, including the ones the
		// ledger does not narrate. That is the point of capturing from a
		// simulator: a real capture has one vantage point, and "A heard it, B
		// did not" is the most informative event in a mesh.
		e.mu.Lock()
		c := e.capture
		e.mu.Unlock()
		if c != nil && rec.Offered {
			c.write(t.endMs, src.Spec.Name, dst.Spec.Name, txPHY,
				rec.RSSIdBm, rec.SNRdB, rec.Outcome, rec.CRCOK, t.frame)
		}
	}
	return nil
}

// runFirmware advances every node's firmware to the current instant.
func (e *Engine) runFirmware(ctx context.Context, now uint32) error {
	nodes := e.Nodes()
	// Every tick on the wire first, then the waits. The nodes advance in
	// parallel on their own cores, and this thread pays for the slowest one
	// instead of the sum — the difference between a 300-node scenario stepping
	// and a 300-node scenario saturating the machine.
	busy := e.channelBusy(now)
	for i, n := range nodes {
		if n.Firmware == nil {
			continue
		}
		// What the channel sounds like here, before the node decides whether to
		// talk. MeshCore asks its radio this in Dispatcher::checkSend, and the
		// answer has to arrive before the tick it applies to - a node told after
		// the fact would be deciding on a channel that has already changed.
		if err := n.Firmware.Bridge.SetChannelBusy(busy[i]); err != nil {
			return fmt.Errorf("engine: %s: %w", n.Spec.Name, err)
		}
		// Each node's own clock: the run's time plus how long it had already
		// been powered on when the run began.
		if err := n.Firmware.Bridge.BeginAdvance(now + n.BootOffsetMs); err != nil {
			return fmt.Errorf("engine: %s: %w", n.Spec.Name, err)
		}
	}
	for i, n := range nodes {
		if n.Firmware == nil {
			continue
		}
		if err := n.Firmware.Bridge.WaitAdvance(ctx, now+n.BootOffsetMs); err != nil {
			return fmt.Errorf("engine: %s: %w", n.Spec.Name, err)
		}
		// The radio reports how the firmware has configured it in the same
		// message that acknowledges the tick, so this is where a gain or
		// transmit-power change becomes the engine's: after the node that made
		// the change has finished making it, and before the next tick's channel
		// decisions are computed against it.
		e.ApplyRadioState(i, n.Firmware.Bridge.Stats())
	}
	return nil
}

// collectTransmissions takes whatever the firmware decided to send.
func (e *Engine) collectTransmissions(now uint32) error {
	nodes := e.Nodes()
	for i, n := range nodes {
		if n.Firmware == nil {
			continue
		}
		for {
			select {
			case frame := <-n.Firmware.Bridge.Transmitted:
				e.startTransmission(i, frame, now)
			default:
				goto next
			}
		}
	next:
	}
	return nil
}

// Inject introduces a message into the network from a node.
//
// Where the node runs firmware, the firmware is asked to originate it and
// builds a real MeshCore packet — which is the only way the rest of the network
// will relay it. A frame fabricated here is not a valid packet, every receiving
// node drops it, and the result is a flood that reaches its neighbours and stops
// dead. That failure is silent and looks exactly like a network with no relays
// configured, which is why it is worth this branch.
//
// Where there is no firmware, the frame goes on the air as-is. That still
// exercises the channel, the collisions and the ledger, and it is how a
// scenario runs at all without a MeshCore build to hand.
func (e *Engine) Inject(nodeIndex int, payload []byte) {
	e.mu.Lock()
	ok := nodeIndex >= 0 && nodeIndex < len(e.nodes)
	var fw *firmware.Node
	if ok {
		fw = e.nodes[nodeIndex].Firmware
	}
	now := e.nowMs
	e.mu.Unlock()
	if !ok {
		return
	}
	if fw != nil {
		if err := fw.Bridge.Originate(payload); err != nil {
			e.record(Event{AtMs: now, Kind: "miss", Detail: err.Error()})
		}
		return
	}
	e.startTransmission(nodeIndex, payload, now)
}

// InjectFrame puts raw bytes on the air from a node, exactly as recorded.
//
// The live-replay path: a packet the real network's origin transmitted is
// re-transmitted here by the same-named simulated node, bytes unaltered, so
// the region scope, path and type the other nodes' firmware will judge are the
// real ones. Deliberately not Originate(): the firmware would wrap the payload
// in a new packet of its own, and the point is to replay the packet that flew.
func (e *Engine) InjectFrame(nodeIndex int, frame []byte) {
	e.mu.Lock()
	ok := nodeIndex >= 0 && nodeIndex < len(e.nodes)
	now := e.nowMs
	e.mu.Unlock()
	if !ok || len(frame) == 0 {
		return
	}
	e.startTransmission(nodeIndex, frame, now)
}

func (e *Engine) startTransmission(from int, frame []byte, now uint32) {
	e.mu.Lock()
	spec := e.nodes[from].Spec
	e.mu.Unlock()
	// Airtime is a property of the transmitter's own modem settings: the same
	// bytes at SF12/62.5 occupy the air some forty times longer than at
	// SF7/250, and a shared figure makes every duty cycle and every collision
	// window wrong for every node not on the default.
	phy := e.phyOf(spec)
	airtime := dsp.AirtimeMillis(phy.sf, phy.bandwidthHz, phy.codingRate, len(frame), true, true)

	e.mu.Lock()
	e.packet++
	id := e.packet
	t := transmission{
		from: from, packetID: id, frame: frame, payload: payloadID(frame),
		startMs: now, endMs: now + uint32(airtime),
	}
	e.inFlight = append(e.inFlight, t)
	e.nodes[from].Sent++
	e.nodes[from].AirtimeMs += airtime
	name := e.nodes[from].Spec.Name
	e.mu.Unlock()

	e.record(Event{AtMs: now, Kind: "tx", From: name, PacketID: id, MessageID: t.payload, Frame: frame,
		Detail: fmt.Sprintf("%d bytes, %.0f ms on air", len(frame), airtime)})
}

// pathLoss is free-space plus terrain diffraction between two nodes, cached.
func (e *Engine) pathLoss(a, b int) (float64, bool) {
	if a > b {
		a, b = b, a
	}
	k := [2]int{a, b}

	e.mu.Lock()
	if v, ok := e.linkCache[k]; ok {
		e.mu.Unlock()
		if math.IsInf(v, 1) {
			return 0, false
		}
		return v, true
	}
	from, to := e.nodes[a].Spec, e.nodes[b].Spec
	e.mu.Unlock()

	distKm := haversineKm(from.Position.Lat, from.Position.Lon, to.Position.Lat, to.Position.Lon)
	if distKm <= 0 {
		return 0, false
	}

	// The free-space cull. Terrain can only ever add loss, so if the pair
	// cannot matter even over flat vacuum — not as a signal, not as
	// interference — there is nothing a terrain profile could change, and the
	// profile is the expensive part. On a country-sized import most pairs are
	// like this, and profiling them anyway is what turned the first flood on a
	// 300-node scenario into a frozen minute: the lazy cache fill walked
	// forty-five thousand pairs of DEM samples on the frame thread.
	fspl := terrain.FSPLdB(distKm, e.phyOf(from).freqMHz)
	bestTx := math.Max(from.TxPowerDBm, to.TxPowerDBm)
	bestRx := bestTx + gain(from) + gain(to) - fspl
	// The quieter of the two receivers. This is a cull, so the question is
	// whether *either* end could hear the other: taking the worse figure would
	// discard a pair the better receiver can close.
	noise := dsp.NoiseFloorDBm(e.phyOf(from).bandwidthHz,
		math.Min(e.noiseFigOf(from), e.noiseFigOf(to)))
	if bestRx < noise-30 {
		e.mu.Lock()
		e.linkCache[k] = fspl // an underestimate, and irrelevant below the floor
		e.mu.Unlock()
		return fspl, true
	}

	profile, ok := e.profile(from, to, distKm)
	loss := math.Inf(1)
	if ok {
		loss = fspl +
			terrain.MultiEdgeLossDB(profile, from.HeightAGLm, to.HeightAGLm, e.phyOf(from).freqMHz) +
			e.Config.ExcessPathLossDB
	}

	e.mu.Lock()
	e.linkCache[k] = loss
	e.mu.Unlock()
	if !ok {
		return 0, false
	}
	return loss, true
}

func (e *Engine) profile(from, to scenario.Node, distKm float64) ([]terrain.Point, bool) {
	n := int(distKm * 1000 / e.Config.ProfileStepM)
	if n < 2 {
		n = 2
	}
	if n > 256 {
		n = 256
	}
	out := make([]terrain.Point, n+1)
	for i := 0; i <= n; i++ {
		f := float64(i) / float64(n)
		h, ok := e.Terrain.ElevationM(
			from.Position.Lat+(to.Position.Lat-from.Position.Lat)*f,
			from.Position.Lon+(to.Position.Lon-from.Position.Lon)*f)
		if !ok {
			return nil, false
		}
		out[i] = terrain.Point{DistM: f * distKm * 1000, HeightM: h}
	}
	return out, true
}

// LinkCacheSnapshot copies the measured matrix out, so a rebuild that changes
// nothing geometric does not have to measure it again.
func (e *Engine) LinkCacheSnapshot() map[[2]int]float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[[2]int]float64, len(e.linkCache))
	for k, v := range e.linkCache {
		out[k] = v
	}
	return out
}

// RestoreLinkCache puts a snapshot back. The caller vouches that the geometry
// it was measured over is this engine's geometry; nothing here can check that,
// which is why the session keys it on a hash of everything the loss depends
// on.
func (e *Engine) RestoreLinkCache(m map[[2]int]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range m {
		e.linkCache[k] = v
	}
}

// PrimeLinks fills the cache from a matrix somebody else computed.
//
// The matrix is the upper triangle of an n by n grid, in the same node order
// the engine holds, carrying free-space plus diffraction and not the excess
// path loss, which is this engine's own setting and is added here. Pairs the
// terrain could not answer for are left out rather than guessed at: they fall
// back to the profile the lazy path would have taken.
//
// It exists so the measurement can be done somewhere other than these cores -
// on a GPU, where forty-eight thousand independent profiles is what the
// hardware is for - without that path having to know anything about the
// engine's locking or its cache.
func (e *Engine) PrimeLinks(n int, loss []float32, noData float32) int {
	if n <= 1 || len(loss) < n*n {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if n != len(e.nodes) {
		// A matrix about a different network is worse than no matrix.
		return 0
	}
	filled := 0
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			v := loss[a*n+b]
			if v == noData || math.IsInf(float64(v), 0) || math.IsNaN(float64(v)) {
				continue
			}
			e.linkCache[[2]int{a, b}] = float64(v) + e.Config.ExcessPathLossDB
			filled++
		}
	}
	return filled
}

// WarmLinks computes the whole path-loss matrix, in parallel.
//
// The cache fills lazily otherwise, which means the first flood pays for every
// pair at once, on whatever thread sent the message. Warming does the same
// work where it belongs: up front, across every core, with a progress figure
// someone can watch. Safe alongside a running engine — pathLoss is locked, and
// a pair warmed twice costs one map hit.
func (e *Engine) WarmLinks(ctx context.Context, progress func(done, total int)) {
	e.mu.Lock()
	n := len(e.nodes)
	e.mu.Unlock()

	type pair struct{ a, b int }
	pairs := make(chan pair, 256)
	go func() {
		defer close(pairs)
		for a := 0; a < n; a++ {
			for b := a + 1; b < n; b++ {
				select {
				case pairs <- pair{a, b}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	total := n * (n - 1) / 2
	var done atomic.Int64
	workers := runtime.NumCPU()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pairs {
				e.pathLoss(p.a, p.b)
				// The first pair as well as every 512th: the first is what
				// moves a status line off the previous phase, and it is the
				// one a throttle otherwise never lets through.
				if d := done.Add(1); progress != nil && (d == 1 || d%512 == 0) {
					progress(int(d), total)
				}
			}
		}()
	}
	wg.Wait()
	if progress != nil {
		progress(int(done.Load()), total)
	}
}

// InvalidateLinks drops the path-loss cache.
//
// A node that moved changes every path it is part of, and a cache keyed on node
// index cannot know which. Dropping all of it is cheaper than being clever, and
// the matrix rewarms in the background.
func (e *Engine) InvalidateLinks() {
	e.mu.Lock()
	e.linkCache = map[[2]int]float64{}
	e.emitterNoise = map[int]float64{}
	e.mu.Unlock()
}

// PathLossForTest exposes the cached link for measurements and tests.
func (e *Engine) PathLossForTest(a, b int) (float64, bool) { return e.pathLoss(a, b) }

func (e *Engine) record(ev Event) {
	e.mu.Lock()
	e.events = append(e.events, ev)
	if e.classCounts == nil {
		e.classCounts = map[string]int{}
	}
	e.classCounts[EventClass(ev.Kind, ev.Detail)]++
	l := e.eventLog
	e.mu.Unlock()
	if l != nil {
		l.write(ev)
	}
}

// EventClass buckets an event by what happened to it, which is what the
// interface's cards and filter chips count: a miss lost to the node's own
// transmitter, a miss lost to a stronger signal, and a miss that was simply
// too quiet are three different problems with three different fixes.
func EventClass(kind, detail string) string {
	switch kind {
	case "tx":
		return "sent"
	case "rx":
		return "received"
	}
	// Prefixes, matching how the details above are written - and not
	// strings.Contains, which a guard test forbids in this package to keep
	// region logic out of the channel.
	switch {
	case strings.HasPrefix(detail, "its own transmitter"):
		return "half-duplex"
	case strings.HasPrefix(detail, "would have decoded"):
		return "interference"
	default:
		return "floor"
	}
}

// EventCounts is how many events of each class the run has produced, counted
// as they are recorded rather than by walking the log - the log is millions
// on a long run, and the cards asking for these ask every tick.
func (e *Engine) EventCounts() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]int, len(e.classCounts))
	for k, v := range e.classCounts {
		out[k] = v
	}
	return out
}

// Scoreboard is the per-node summary, ordered worst-value first.
//
// Unique deliveries against redundant relays is the number HopReach found
// mattered most, and it is the one a duty-cycle figure hides: a repeater can be
// busy, legal, and reaching nobody who had not already heard the packet.
type Score struct {
	Name           string
	Sent           int
	Heard          int
	AirtimeMs      float64
	DutyCyclePct   float64
	UniqueDelivery int
	RedundantRelay int
}

func (e *Engine) Scoreboard() []Score {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]Score, 0, len(e.nodes))
	for _, n := range e.nodes {
		s := Score{
			Name: n.Spec.Name, Sent: n.Sent, Heard: n.Heard,
			AirtimeMs:      n.AirtimeMs,
			UniqueDelivery: n.UniqueDelivery, RedundantRelay: n.RedundantRelay,
		}
		if e.nowMs > 0 {
			s.DutyCyclePct = 100 * n.AirtimeMs / float64(e.nowMs)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AirtimeMs > out[j].AirtimeMs })
	return out
}

// Close shuts every node down.
func (e *Engine) Close() error {
	var err error
	for _, n := range e.Nodes() {
		if n.Firmware == nil {
			continue
		}
		if cerr := n.Firmware.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

func gain(n scenario.Node) float64 {
	if n.Antenna.Pattern == nil {
		return 0
	}
	return n.Antenna.Pattern.PeakDBi() - n.Antenna.FeedlineDB
}

// addDBm sums two powers expressed in dBm. Adding decibels instead is a mistake
// that produces a plausible number and is wrong by tens of dB.
func addDBm(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	return 10 * math.Log10(math.Pow(10, a/10)+math.Pow(10, b/10))
}

// phy is one node's modem settings, resolved.
type phy struct {
	freqMHz     float64
	bandwidthHz float64
	sf          int
	codingRate  int
}

// sameChannel reports whether two radios can hear each other at all.
//
// Frequency and bandwidth must match; spreading factor too, because LoRa's
// orthogonality across SFs is the property the whole modulation is sold on — a
// receiver at SF10 does not demodulate SF7, it sees noise. Coding rate is in
// the explicit header, so it does not have to match.
func (a phy) sameChannel(b phy) bool {
	return math.Abs(a.freqMHz-b.freqMHz) < 0.001 &&
		a.bandwidthHz == b.bandwidthHz && a.sf == b.sf
}

// noiseFigOf resolves a node's receive noise figure, falling back to the
// scenario's default for nodes imported without one.
//
// Per node rather than per run because a repeater with a masthead preamp and a
// handheld in a pocket do not have the same one. scenario.Node has carried the
// field since import and the engine simply never read it, so every node in
// every result so far has been given the run-wide figure regardless of what its
// board profile said.
func (e *Engine) noiseFigOf(n scenario.Node) float64 {
	if n.NoiseFigureDB > 0 {
		return n.NoiseFigureDB
	}
	return e.Config.NoiseFigDB
}

// phyOf resolves a node's radio, falling back to the scenario's defaults for
// nodes imported without one.
func (e *Engine) phyOf(n scenario.Node) phy {
	p := phy{
		freqMHz:     n.Radio.CentreHz / 1e6,
		bandwidthHz: n.Radio.BandwidthHz,
		sf:          n.Radio.SpreadFactor,
		codingRate:  n.Radio.CodingRate,
	}
	if p.freqMHz <= 0 {
		p.freqMHz = e.Config.FreqMHz
	}
	if p.bandwidthHz <= 0 {
		p.bandwidthHz = e.Config.BandwidthHz
	}
	if p.sf <= 0 {
		p.sf = e.Config.SF
	}
	if p.codingRate <= 0 {
		p.codingRate = e.Config.CodingRate
	}
	return p
}

// requiredSNRdB is Semtech's published demodulator floor, which the modem has
// been measured against to within 1.6 dB (docs/shortcomings.md).
func requiredSNRdB(sf int) float64 {
	if v, ok := dsp.RequiredSNRdB[sf]; ok {
		return v
	}
	return -20
}

// payloadID identifies a message by its content, across every hop it takes.
//
// The header and the payload; deliberately *not* the path. A flood packet grows
// a hop hash at every relay, so hashing the whole frame gave the same message a
// new identity at each hop — every relay looked like a brand new message, no
// message could be followed across the mesh, and the unique-versus-redundant
// count was measuring nothing.
//
// The route bits are masked out of the header for the same reason: a node may
// re-route a packet it forwards, and a message that changed identity when it
// switched from flood to direct would break at exactly the interesting moment.
func payloadID(frame []byte) uint64 {
	d := capture.Dissect(frame)
	h := uint64(14695981039346656037)
	mix := func(b byte) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	if d.Truncated {
		// Unparseable: fall back to the whole frame. It cannot be followed, but
		// two identical malformed frames should still be one thing.
		for _, b := range frame {
			mix(b)
		}
		return h
	}
	// Payload type only, not the route bits or the version.
	mix(d.PayloadType)
	for _, b := range d.Payload {
		mix(b)
	}
	return h
}

// PayloadIDForTest exposes the message identity for tests.
func PayloadIDForTest(frame []byte) uint64 { return payloadID(frame) }

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

// channelBusy answers, for every node, whether another station is on the air
// loudly enough to be detected here.
//
// This is the input to MeshCore's listen-before-talk, and it is the whole
// reason the firmware defers instead of transmitting into somebody else's
// packet. Nothing but the engine can work it out: the node's own radio has no
// view of the channel, and a node cannot hear itself.
//
// Detection, not decoding. A LoRa receiver locks onto a preamble several dB
// below the level at which it could demodulate the payload, so the threshold is
// the demodulator floor for the current spreading factor rather than anything
// stricter - a carrier a node can detect but not decode is exactly the case
// listen-before-talk exists for.
func (e *Engine) channelBusy(now uint32) []bool {
	// Snapshot under the lock, compute outside it. pathLoss takes the same
	// mutex, and Go's is not reentrant: holding it across that call deadlocks
	// the frame thread on the first tick with anything in the air, which
	// presents as the window going black.
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	air := make([]transmission, 0, len(e.inFlight))
	for _, t := range e.inFlight {
		if now >= t.startMs && now < t.endMs {
			air = append(air, t)
		}
	}
	e.mu.Unlock()

	busy := make([]bool, len(nodes))
	if len(air) == 0 {
		return busy
	}
	for i, dst := range nodes {
		if dst.Firmware == nil {
			continue
		}
		rxPHY := e.phyOf(dst.Spec)
		for _, t := range air {
			// A node is deaf to the channel while its own transmitter is keyed,
			// and it is not listening for itself in any case.
			if t.from == i {
				continue
			}
			src := nodes[t.from]
			// Activity on another channel is not activity this node can detect
			// - the rule delivery already uses. Without it a node would defer
			// to a mesh it is not part of.
			txPHY := e.phyOf(src.Spec)
			if !txPHY.sameChannel(rxPHY) {
				continue
			}
			loss, ok := e.pathLoss(t.from, i)
			if !ok {
				continue
			}
			rxDBm := src.Spec.TxPowerDBm + gain(src.Spec) - loss + gain(dst.Spec)
			noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec))
			if rxDBm-noiseDBm >= requiredSNRdB(txPHY.sf) {
				busy[i] = true
				break
			}
		}
	}
	return busy
}
