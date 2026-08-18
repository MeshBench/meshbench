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
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/environ"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/scenario"
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

	// RFMode selects which physics decides reception: RFCalculated (the
	// zero value - link budgets and demodulator floors) or RFWaveform (IQ
	// through the channel, verdict by the demodulator). See waveform.go.
	RFMode RFMode

	// Realism is the optional-imperfections switch set - oscillator error,
	// multipath, fading, implementation loss, saturation. All zero by
	// default: the kind simulator, with its kindness now optional.
	Realism Realism
}

// Engine owns the run.
type Engine struct {
	Terrain coverage.Terrain
	Config  Config
	Ledger  capture.Ledger
	// Env is what physically stands on the ground - buildings, from the
	// environment tiles. Nil means bare earth, exactly as before; setting it
	// changes path budgets in both RF modes, because buildings price GainDB
	// rather than verdicts.
	Env environ.Provider

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
	// recent holds transmissions that have already ended but overlapped
	// something still in flight. Without it a short interferer that finished
	// before the wanted packet did was invisible to interference in both RF
	// modes - the collision happened on the air and nowhere else.
	recent []transmission
	// wfCAD caches in-flight transmissions' synthesised baseband for the
	// waveform CAD path, which asks every tick. See cadCache.
	wfCAD modCache
	// obsCache is the observers' modulated baseband, pruned to what is on
	// the air. Guarded by obsMu, not mu: ObserveSpan runs on rtl_tcp client
	// goroutines while the step loop holds the engine's own lock.
	obsMu    sync.Mutex
	obsCache modCache

	// emitterNoise caches each receiver's extra floor from the emitter fleet,
	// invalidated with the link cache — emitters move exactly as often as
	// nodes do.
	emitterNoise map[int]float64

	// linkCache holds path loss between node pairs. Terrain does not move
	// during a run, and recomputing a profile per packet per pair is the
	// difference between a run that takes seconds and one that takes hours.
	linkCache map[[2]int]float64
	// liveProfiles counts pathLoss calls that missed the cache and paid for a
	// terrain profile during play rather than during a warm. A caller reads
	// this to say why a tick just took a while, rather than leaving it looking
	// like nothing happened.
	liveProfiles atomic.Int64
	// firmwareDown is which nodes' firmware has stopped answering - a
	// crashed process, noticed once and then skipped rather than retried
	// every tick. firmwareNewlyDown is the subset a caller has not yet been
	// told about; FirmwareFailures drains it.
	firmwareDown      map[string]bool
	firmwareNewlyDown []string
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
