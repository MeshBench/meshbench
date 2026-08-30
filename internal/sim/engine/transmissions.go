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

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/sim/capture"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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
	cache := modCache{}
	for i := range done {
		if fw := senders[i].Firmware; fw != nil {
			if err := fw.Bridge.TransmitFinished(); err != nil {
				return fmt.Errorf("engine: tx done for %s: %w", senders[i].Spec().Name, err)
			}
		}
	}
	if mode == RFWaveform {
		// All of this tick's finishers judged as one batch, so several
		// transmissions ending together fill the machine instead of
		// queueing behind each other's small candidate sets.
		return e.deliverWaveformBatch(done, concurrent, cache)
	}
	for _, t := range done {
		if err := e.deliver(t, concurrent, cache); err != nil {
			return err
		}
	}
	return nil
}

// deliver works out who heard a finished transmission, and records why not for
// everyone who did not.
func (e *Engine) deliver(t transmission, concurrent []transmission, cache modCache) error {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	src := nodes[t.from]
	// The transmitter's own radio settings, not the scenario's. Two nodes on
	// different presets are on different channels, and a channel that ignores
	// that lets a UK Narrow repeater decode an Australian one.
	txPHY := e.phyOf(src.Spec())

	for i, dst := range nodes {
		if i == t.from {
			continue
		}
		if !dst.Spec().Kind.RunsFirmware() && dst.Spec().Kind != scenario.SDRObserver {
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
		rxPHY := e.phyOf(dst.Spec())
		if dst.Spec().Kind != scenario.SDRObserver && !txPHY.sameChannel(rxPHY) {
			continue
		}
		// The hybrid: a receiver flagged TrueRF gets the waveform judge even
		// in a calculated run - a big mesh priced fast, full fidelity where
		// somebody asked for it. Same gates first, same ledger after.
		if dst.Spec().TrueRF {
			if e.judgeHybrid(t, i, concurrent, nodes, txPHY, cache) {
				continue
			}
		}
		noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec()))
		// The emitter fleet's contribution, through the same terrain. This is
		// the per-receiver floor: a node beside a paging mast lives on a
		// different noise floor from one on a quiet hill.
		if extra := e.emitterNoiseAt(i); !math.IsInf(extra, -1) {
			noiseDBm = addDBm(noiseDBm, extra)
		}
		required := requiredSNRdB(txPHY.sf)

		loss, ok := e.pathLoss(t.from, i)
		if !ok {
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec().Name, To: dst.Spec().Name,
				PacketID: t.packetID, Outcome: capture.OutOfRange,
				Detail: "no terrain data covers this path"})
			continue
		}

		rxDBm := src.Spec().TxPowerDBm + gain(src.Spec()) - loss + gain(dst.Spec())
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
			if !e.phyOf(nodes[other.from].Spec()).sameChannel(txPHY) {
				continue
			}
			ol, ok := e.pathLoss(other.from, i)
			if !ok {
				continue
			}
			p := nodes[other.from].Spec().TxPowerDBm + gain(nodes[other.from].Spec()) - ol + gain(dst.Spec())
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

		// One demodulator: whoever this receiver locked onto first has it for
		// the length of their packet, and nothing else gets decoded inside
		// that window however strong it is.
		held := ""
		if !deaf {
			held = e.demodulatorHeldBy(i, t, concurrent, nodes, txPHY)
		}

		// What a collision destroyed on the way through, for a packet that
		// won the demodulator in the first place.
		damaged, repaired, survives := 0.0, 0, true
		if !deaf && held == "" {
			damaged = e.corruptedSymbols(i, t, rxDBm, concurrent, nodes, txPHY)
			repaired, survives = survivesCorruption(damaged, txPHY.codingRate)
		}

		// What the modem would have said, as against what the arithmetic
		// produced. Every SNR that leaves here is the reportable one; the
		// unclamped ratio stays behind to make the decisions.
		reported := dsp.ReportSNRdB(effective)

		rec := capture.Reception{
			PacketID: t.packetID, FromNode: src.Spec().Name, ToNode: dst.Spec().Name,
			RSSIdBm: dsp.ReportRSSIdBm(rxDBm), SNRdB: reported,
			Offered: rxDBm > noiseDBm-10,
		}
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
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec().Name, To: dst.Spec().Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: reported, Frame: t.frame,
				Detail: "its own transmitter was keyed; LoRa is half duplex"})
		case held != "" && effective >= required:
			// Strong enough to have decoded, and it still did not: the one
			// demodulator this receiver has was already following somebody
			// else's preamble. Reported only when the packet would otherwise
			// have arrived, so a weak signal is never relabelled as a
			// collision it was never in the running for.
			rec.Outcome = capture.NotDemodulated
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec().Name, To: dst.Spec().Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: reported, Frame: t.frame, Detail: busyDemodulatorDetail(held)})
		case held == "" && effective >= required && !survives:
			// It locked, it led every interferer on average, and something
			// still landed on its symbols. Waveform mode reaches this verdict
			// through the demodulator; here it is the interleaver's own
			// guarantee applied to the overlap the timing actually produced.
			rec.Outcome = capture.NotDemodulated
			rec.Demod = true
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec().Name, To: dst.Spec().Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: reported, Frame: t.frame,
				Detail: fmt.Sprintf(
					"decoded its header, then %.0f symbol(s) were destroyed by a collision "+
						"it could not capture over; beyond what CR 4/%d can repair",
					damaged, txPHY.codingRate+4)})
		case effective < required:
			rec.Outcome = capture.NotDemodulated
			// How near it came. Interference is included deliberately: a packet
			// lost to a stronger neighbour is not one a better receiver saves,
			// and counting it as nearly-decoded would overstate what sensitivity
			// buys on exactly the crowded mesh where the question is asked.
			e.sens.note(effective-required, false)
			why := fmt.Sprintf("SNR %.1f dB against %.1f dB needed at SF%d", effective, required, e.Config.SF)
			if !math.IsInf(interferenceDBm, -1) && snr >= required {
				why = fmt.Sprintf("would have decoded at %.1f dB, lost to a stronger interferer",
					dsp.ReportSNRdB(snr))
			}
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec().Name, To: dst.Spec().Name,
				PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
				SNRdB: reported, Frame: t.frame, Detail: why})
		default:
			rec.Demod, rec.CRCOK, rec.FirmwareSaw = true, true, true
			rec.Outcome = capture.Accepted
			e.sens.note(effective-required, true)

			// Unique against redundant. A repeater can be busy, legal, and
			// reaching nobody who had not already heard the message — which a
			// duty-cycle figure hides completely.
			e.mu.Lock()
			// Inside the lock with its siblings, and with the waveform path's
			// own increment: Scoreboard reads this counter under e.mu, so a
			// bare increment outside it is a race that loses receptions as
			// well as reporting them wrong.
			dst.Heard++
			if e.seen[dst.Spec().Name] == nil {
				e.seen[dst.Spec().Name] = map[uint64]bool{}
			}
			first := !e.seen[dst.Spec().Name][t.payload]
			e.seen[dst.Spec().Name][t.payload] = true
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
			if repaired > 0 {
				detail = fmt.Sprintf("%d symbol(s) lost to a collision and repaired by FEC; ",
					repaired) + detail
			}
			e.record(Event{AtMs: t.endMs, Kind: "rx", From: src.Spec().Name, To: dst.Spec().Name,
				Frame: t.frame, PacketID: t.packetID, MessageID: t.payload,
				Outcome: rec.Outcome, SNRdB: reported, Detail: detail})

			// Only a node running firmware is handed the frame. An observer
			// hears it and does nothing, which is what an observer is.
			if dst.Firmware != nil {
				if err := dst.Firmware.Bridge.Deliver(t.frame); err != nil {
					// The verdict was reached before the handoff, so this
					// receiver's row is owed to the ledger and the capture
					// whatever the bridge then did. A run that abandons them
					// here loses the one reception that explains the failure.
					e.Ledger.Record(rec)
					e.captureWrite(t, src, dst, txPHY, rec)
					return fmt.Errorf("engine: deliver to %s: %w", dst.Spec().Name, err)
				}
			}
		}
		e.Ledger.Record(rec)

		// The capture takes every receiver's view, including the ones the
		// ledger does not narrate. That is the point of capturing from a
		// simulator: a real capture has one vantage point, and "A heard it, B
		// did not" is the most informative event in a mesh. Once per receiver,
		// after the verdict: a second write of the same row makes a capture
		// count every reception twice, and the count is what a soak judges on.
		e.captureWrite(t, src, dst, txPHY, rec)
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
	spec := e.nodes[from].Spec()
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
	name := e.nodes[from].Spec().Name
	e.mu.Unlock()

	e.record(Event{AtMs: now, Kind: "tx", From: name, PacketID: id, MessageID: t.payload, Frame: frame,
		Detail: fmt.Sprintf("%d bytes, %.0f ms on air", len(frame), airtime)})
}
