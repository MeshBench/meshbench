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
	"sort"
	"sync"

	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// Node is one participant: its place in the world and its running firmware.
type Node struct {
	Spec scenario.Node

	// Firmware is nil for a node that does not run any — an SDR observer, or a
	// custom emitter that is only there to be interfered with.
	Firmware *firmware.Node

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
	Outcome  capture.Outcome
	SNRdB    float64
	Detail   string
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
}

// Engine owns the run.
type Engine struct {
	Terrain coverage.Terrain
	Config  Config
	Ledger  capture.Ledger

	mu     sync.Mutex
	nodes  []*Node
	events []Event
	nowMs  uint32
	packet uint64

	// inFlight is what is on the air at this instant: a transmission occupies
	// the channel for its own airtime, and anything else transmitting during
	// that window is a collision rather than a separate event.
	inFlight []transmission

	// linkCache holds path loss between node pairs. Terrain does not move
	// during a run, and recomputing a profile per packet per pair is the
	// difference between a run that takes seconds and one that takes hours.
	linkCache map[[2]int]float64

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
		linkCache: map[[2]int]float64{},
		seen:      map[string]map[uint64]bool{},
	}
}

// Add puts a node in the world.
func (e *Engine) Add(spec scenario.Node, fw *firmware.Node) *Node {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := &Node{Spec: spec, Firmware: fw}
	e.nodes = append(e.nodes, n)
	// Terrain has not changed, but the set of pairs has.
	e.linkCache = map[[2]int]float64{}
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

// Events is everything that has happened, in order.
func (e *Engine) Events() []Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Event, len(e.events))
	copy(out, e.events)
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
	e.mu.Unlock()

	for _, t := range done {
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
	noiseDBm := dsp.NoiseFloorDBm(e.Config.BandwidthHz, e.Config.NoiseFigDB)
	// The SNR a LoRa receiver can decode at, from the same measurement the
	// sensitivity test makes against Semtech's figures.
	required := requiredSNRdB(e.Config.SF)

	for i, dst := range nodes {
		if i == t.from {
			continue
		}
		if !dst.Spec.Kind.RunsFirmware() && dst.Spec.Kind != scenario.SDRObserver {
			continue
		}

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
		switch {
		case deaf:
			rec.Outcome = capture.NotDemodulated
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, Outcome: rec.Outcome, SNRdB: effective,
				Detail: "its own transmitter was keyed; LoRa is half duplex"})
		case !rec.Offered:
			rec.Outcome = capture.OutOfRange
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, Outcome: rec.Outcome, SNRdB: effective,
				Detail: fmt.Sprintf("%.0f dB of path loss; nothing measurable arrived", loss)})
		case effective < required:
			rec.Outcome = capture.NotDemodulated
			why := fmt.Sprintf("SNR %.1f dB against %.1f dB needed at SF%d", effective, required, e.Config.SF)
			if !math.IsInf(interferenceDBm, -1) && snr >= required {
				why = fmt.Sprintf("would have decoded at %.1f dB, lost to a stronger interferer", snr)
			}
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
				PacketID: t.packetID, Outcome: rec.Outcome, SNRdB: effective, Detail: why})
		default:
			rec.Demod, rec.CRCOK, rec.FirmwareSaw = true, true, true
			rec.Outcome = capture.Accepted
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
				PacketID: t.packetID, Outcome: rec.Outcome, SNRdB: effective, Detail: detail})

			// Only a node running firmware is handed the frame. An observer
			// hears it and does nothing, which is what an observer is.
			if dst.Firmware != nil {
				if err := dst.Firmware.Bridge.Deliver(t.frame); err != nil {
					return fmt.Errorf("engine: deliver to %s: %w", dst.Spec.Name, err)
				}
			}
		}
		e.Ledger.Record(rec)
	}
	return nil
}

// runFirmware advances every node's firmware to the current instant.
func (e *Engine) runFirmware(ctx context.Context, now uint32) error {
	nodes := e.Nodes()
	for _, n := range nodes {
		if n.Firmware == nil {
			continue
		}
		if err := n.Firmware.Bridge.Advance(ctx, now); err != nil {
			return fmt.Errorf("engine: %s: %w", n.Spec.Name, err)
		}
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

// Inject puts a frame on the air from a node, as though its firmware had sent
// it.
//
// The way to drive a scenario that has no firmware attached, and the way a test
// exercises the channel without a MeshCore build. A node with firmware
// transmits because its own code decided to; this is for everything else.
func (e *Engine) Inject(nodeIndex int, frame []byte) {
	e.mu.Lock()
	ok := nodeIndex >= 0 && nodeIndex < len(e.nodes)
	now := e.nowMs
	e.mu.Unlock()
	if !ok {
		return
	}
	e.startTransmission(nodeIndex, frame, now)
}

func (e *Engine) startTransmission(from int, frame []byte, now uint32) {
	airtime := dsp.AirtimeMillis(e.Config.SF, e.Config.BandwidthHz, e.Config.CodingRate,
		len(frame), true, true)

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

	e.record(Event{AtMs: now, Kind: "tx", From: name, PacketID: id,
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
	profile, ok := e.profile(from, to, distKm)
	loss := math.Inf(1)
	if ok {
		loss = terrain.FSPLdB(distKm, e.Config.FreqMHz) +
			terrain.MultiEdgeLossDB(profile, from.HeightAGLm, to.HeightAGLm, e.Config.FreqMHz)
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

func (e *Engine) record(ev Event) {
	e.mu.Lock()
	e.events = append(e.events, ev)
	e.mu.Unlock()
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

// requiredSNRdB is Semtech's published demodulator floor, which the modem has
// been measured against to within 1.6 dB (docs/shortcomings.md).
func requiredSNRdB(sf int) float64 {
	if v, ok := dsp.RequiredSNRdB[sf]; ok {
		return v
	}
	return -20
}

// payloadID identifies a message by its content, so the same one relayed by
// several nodes is recognised as one message rather than several.
func payloadID(frame []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, b := range frame {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}
