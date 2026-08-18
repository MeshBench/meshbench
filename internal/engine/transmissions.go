// What is on the air, and what became of it.
//
// A transmission occupies the channel for its airtime, and anything else
// transmitting inside that window is a collision rather than a separate
// event. deliver is where the simulator earns its keep: it hands the
// demodulator a sum of waveforms and asks what came out, rather than deciding
// in advance which packet wins.
package engine

import (
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/scenario"
)

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
	// Everything that could have shared the air with a transmission ending
	// now: what is still up, what ended this tick, and what ended earlier
	// but overlapped something still running. Missing that last set made a
	// short interferer invisible the moment it stopped.
	concurrent := append(append([]transmission{}, still...), done...)
	concurrent = append(concurrent, e.recent...)
	e.recent = append(e.recent, done...)
	if len(still) == 0 {
		e.recent = e.recent[:0]
	} else {
		minStart := still[0].startMs
		for _, s := range still[1:] {
			if s.startMs < minStart {
				minStart = s.startMs
			}
		}
		kept := e.recent[:0]
		for _, r := range e.recent {
			if r.endMs > minStart {
				kept = append(kept, r)
			}
		}
		e.recent = kept
	}
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
	// One modulation cache per batch: in waveform mode every receiver of
	// every finished transmission shares the same synthesised baseband.
	e.mu.Lock()
	mode := e.Config.rfMode()
	e.mu.Unlock()
	var cache modCache
	if mode == RFWaveform {
		cache = modCache{}
	}
	for i, t := range done {
		if fw := senders[i].Firmware; fw != nil {
			if err := fw.Bridge.TransmitFinished(); err != nil {
				return fmt.Errorf("engine: tx done for %s: %w", senders[i].Spec.Name, err)
			}
		}
		if mode == RFWaveform {
			if err := e.deliverWaveform(t, concurrent, cache); err != nil {
				return err
			}
			continue
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
