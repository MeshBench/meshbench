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
	"time"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/rf/terrain"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/sim/capture"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

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

	// UnverifiedWiring runs a board whose emulation wiring nobody has watched
	// boot yet.
	//
	// The gate it lifts is a curation claim, not a safety one: a board is on
	// board.EmulationVerified once someone has seen its own image boot here.
	// Something has to do the seeing, and until this existed nothing could -
	// the probe that establishes the fact was refused by the fact's absence.
	// Off by default, because a scenario that quietly ran unwatched wiring
	// would report a board as silent when it was only mis-wired.
	UnverifiedWiring bool

	// FirmwareTickTimeout is how long a node gets to acknowledge one tick.
	// Zero takes DefaultFirmwareTickTimeout, which is what every run uses; a
	// test that has to watch the deadline arrive says so here rather than
	// waiting out half a minute.
	FirmwareTickTimeout time.Duration
}

// Engine owns the run.
type Engine struct {
	Terrain propagation.Terrain
	Config  Config
	Ledger  capture.Ledger
	// Env is what physically stands on the ground - buildings, from the
	// environment tiles. Nil means bare earth, exactly as before; setting it
	// changes path budgets in both RF modes, because buildings price GainDB
	// rather than verdicts.
	Env environ.Provider

	// The building index over Env, built lazily by envIndex and remembered
	// with what it was built from, so swapping the provider or growing the
	// fleet rebuilds it rather than pricing yesterday's town.
	envMu      sync.Mutex
	envIx      *environ.PathIndex
	envIxFor   environ.Provider
	envIxNodes int

	// attachMu serialises whole-mesh firmware attaches against each other.
	// AttachNativeProgress filters to nodes that are not yet running, then
	// starts them - and a node's Firmware field is only set once its backend
	// is up. Two attaches racing (a single-node reflash and the play button's
	// start-everything, fired close together) would both see the same node as
	// idle and both boot it, leaving two emulators on one node's flash, card
	// and input sockets. Held for the whole attach, the second waits and then
	// finds nothing left to start.
	attachMu sync.Mutex

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

	// gainCache is each ordered pair's two antenna gains, in the direction
	// each end is really in. Kept because the arithmetic is trigonometry and
	// the same pair is asked several times per transmission - once for the
	// wanted signal, again for the demodulator contest, again for every
	// interferer that lands on it. Its own lock rather than e.mu: it is read
	// from the parallel waveform judges, which already queue on that one.
	// Only a node moving changes a look angle, which is where it is dropped.
	gainMu    sync.RWMutex
	gainCache map[[2]int][2]float64

	// emitterNoise caches each receiver's extra floor from the emitter fleet,
	// invalidated with the link cache — emitters move exactly as often as
	// nodes do.
	emitterNoise map[int]float64

	// profCache holds terrain profiles between node pairs - the DEM walk
	// that dominates a cold pathLoss. Kept apart from linkCache because
	// their lifetimes differ: a radio report invalidates the loss, but only
	// the ground moving invalidates the profile, and re-walking the DEM
	// because a node reported its FEM state is how a busy network stuttered
	// to a stop. Bounded FIFO: the pairs actually talking are few.
	profCache map[[2]int][]terrain.Point
	profOrder [][2]int

	// linkCache holds path loss between node pairs. Terrain does not move
	// during a run, and recomputing a profile per packet per pair is the
	// difference between a run that takes seconds and one that takes hours.
	linkCache map[[2]int]float64
	// culled marks the linkCache entries that are below-floor underestimates
	// rather than full losses. Only these read the nodes' effective RF
	// figures, so only these fall when a radio report changes them - a full
	// loss is propagation, and propagation does not care about a FEM bit.
	culled map[[2]int]bool
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
	firmwareNewlyDown []FirmwareFailure
	// classCounts is events by class, kept as they are recorded so the cards
	// that show them never walk the log.
	classCounts map[Class]int

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
func New(t propagation.Terrain, c Config) *Engine {
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
		culled:       map[[2]int]bool{},
		profCache:    map[[2]int][]terrain.Point{},
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
		Firmware:       fw,
		baseTxPowerDBm: spec.TxPowerDBm,
		baseNoiseFigDB: spec.NoiseFigureDB,
	}
	n.state.Store(&nodeState{spec: spec})
	e.nodes = append(e.nodes, n)
	// Terrain has not changed, but the set of pairs has. The profiles keep:
	// they are keyed by pair index, and existing indices still mean the
	// same ground.
	e.linkCache = map[[2]int]float64{}
	e.culled = map[[2]int]bool{}
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
		if name != "" && nd.specRef().Name != name {
			continue
		}
		if nd.specRef().Firmware.Version == version {
			continue
		}
		nd.changeSpec(func(s *scenario.Node) { s.Firmware.Version = version })
		n++
	}
	return n
}

// SetCard changes what is in a node's card slot, so the engine's own copy of
// the scenario agrees with the one the panels draw.
//
// Held here as well as in the session because the engine is what actually
// starts a machine: without this the slot would be changed in the interface,
// agree with itself everywhere, and the next start would fit the card the run
// was opened with.
func (e *Engine) SetCard(name string, fitted bool, file string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, nd := range e.nodes {
		if nd.specRef().Name != name {
			continue
		}
		nd.changeSpec(func(s *scenario.Node) {
			s.Card = scenario.CardFitted
			if !fitted {
				s.Card = scenario.CardEmpty
			}
			s.CardFile = file
		})
	}
}

// PinBoard changes which hardware a node's build is for, alongside the pin
// above.
//
// Separate from PinFirmware because a version can change without the hardware
// doing so, and because the engine's copy of the spec is what decides which
// backend a node gets: an emulated board or a build for this machine. A pin
// that moved the version and left the board behind produced exactly that
// mismatch - a host build asked to run under an emulator, or the reverse.
func (e *Engine) PinBoard(name, board string, role scenario.Role) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, nd := range e.nodes {
		if name != "" && nd.specRef().Name != name {
			continue
		}
		nd.changeSpec(func(s *scenario.Node) {
			s.Firmware.Board = board
			if role != "" {
				s.Firmware.Role = role
			}
		})
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

// Close shuts every node down, and closes what the run was writing to.
//
// The recorders are part of shutting down, not an afterthought: a FIFO
// capture owns a goroutine draining onto the pipe, and an engine that closed
// its nodes and left the capture open leaked that goroutine and its open file
// for the life of the process. A workbench that opens a scenario, records,
// closes it and opens another accumulates one of each per run.
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
	if _, _, cerr := e.StopCapture(); err == nil {
		err = cerr
	}
	if _, _, cerr := e.StopEventLog(); err == nil {
		err = cerr
	}
	return err
}
