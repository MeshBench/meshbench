// The waveform verdict: reception decided by demodulating what arrived.
//
// In waveform mode the channel produces a signal and the demodulator produces
// the verdict. There is no code here that compares an SNR to a threshold -
// capture effect, partial overlap and interference alignment all emerge from
// summed IQ and an FFT, or they do not happen at all. SNR is still measured
// and recorded, but as telemetry: it is never the reason.
//
// Until the coding chain lands (plan W2), the verdict is a symbol proxy: the
// receiver decodes and every payload symbol must match what was sent. That is
// exactly as honest as the modulation is - and harsher than real FEC under a
// clipped tail, which the ledger says out loud rather than hiding.
package engine

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/MeshBench/meshbench/internal/capture"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/rf"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// RFMode selects which physics decides reception.
type RFMode string

const (
	// RFCalculated is the fast model: link budget, noise and interference in
	// dBm, verdict by demodulator floor. The zero value, so every existing
	// scenario is untouched.
	RFCalculated RFMode = "calculated"
	// RFWaveform is the authoritative model: IQ through rf.Observe, verdict
	// by the demodulator.
	RFWaveform RFMode = "waveform"
)

// rfMode normalises the config field's zero value.
func (c Config) rfMode() RFMode {
	if c.RFMode == RFWaveform {
		return RFWaveform
	}
	return RFCalculated
}

// SetRFMode switches the physics live. Safe mid-run: the mode is read once
// per delivery batch, so a switch lands on a whole-transmission boundary.
func (e *Engine) SetRFMode(m RFMode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if m == RFWaveform {
		e.Config.RFMode = RFWaveform
	} else {
		e.Config.RFMode = RFCalculated
	}
}

// modCache is one delivery batch's modulated baseband, keyed by packet.
//
// A transmission's unit-amplitude samples do not depend on who is listening,
// so they are synthesised once per batch and shared by every receiver - the
// difference between one modulation per packet and one per packet-receiver
// pair, which on a flood is the difference between fast and quadratic.
type modCache map[uint64][]complex128

func (e *Engine) modulated(c modCache, t transmission, sf int) []complex128 {
	if s, ok := c[t.packetID]; ok {
		return s
	}
	s := dsp.Modulator{SF: sf}.Modulate(symbolsFor(t.frame, sf))
	c[t.packetID] = s
	return s
}

// wfCandidate is one receiver that cleared the cheap gates and is worth DSP.
type wfCandidate struct {
	i        int
	rxDBm    float64
	noiseDBm float64
}

// wfResult is what the demodulator said for one candidate.
type wfResult struct {
	decoded  bool
	bad      int // payload symbols wrong
	total    int // payload symbols sent
	snrdB    float64
	preamble bool // preamble region survived, for the ledger's Demod flag
}

// waveformPreambleSyms matches symbolsFor's preamble length.
const waveformPreambleSyms = 8

// deliverWaveform is deliver's waveform-mode twin: same gates, same ledger,
// different judge. The DSP runs in parallel across receivers; the bookkeeping
// stays serial, in node order, so the ledger and event log are deterministic.
func (e *Engine) deliverWaveform(t transmission, concurrent []transmission, cache modCache) error {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	seed := e.Config.Seed
	e.mu.Unlock()

	src := nodes[t.from]
	txPHY := e.phyOf(src.Spec)
	txSamples := e.modulated(cache, t, txPHY.sf)
	sentSyms := symbolsFor(t.frame, txPHY.sf)
	// Every concurrent transmission's baseband, synthesised now, serially:
	// the judges run in parallel and a lazily-filled map under them would be
	// a data race.
	for _, other := range concurrent {
		if other.packetID != t.packetID {
			e.modulated(cache, other, e.phyOf(nodes[other.from].Spec).sf)
		}
	}

	cands, deaf := e.waveformCandidates(t, concurrent, nodes, txPHY)

	results := make(map[int]wfResult, len(cands))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for _, c := range cands {
		c := c
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r := e.judgeWaveform(t, c, concurrent, nodes, txPHY, txSamples, sentSyms, cache, seed)
			mu.Lock()
			results[c.i] = r
			mu.Unlock()
		}()
	}
	wg.Wait()

	for _, c := range cands {
		e.settleWaveform(t, src, nodes[c.i], c, results[c.i], txPHY)
	}
	// In node order, not map order: the event log is part of the result, and
	// a result must not depend on map iteration.
	for i := range nodes {
		if rec, ok := deaf[i]; ok {
			e.recordDeaf(t, src, nodes[i], rec, txPHY)
		}
	}
	return nil
}

// waveformCandidates applies the cheap gates: channel, terrain, a floor far
// enough under the deepest LoRa decode that nothing decodable is ever culled,
// and half-duplex deafness. Deaf receivers are returned separately - they had
// measurable signal and a different problem, and the ledger says which.
func (e *Engine) waveformCandidates(t transmission, concurrent []transmission,
	nodes []*Node, txPHY phy) ([]wfCandidate, map[int]capture.Reception) {
	var cands []wfCandidate
	deaf := map[int]capture.Reception{}
	for i, dst := range nodes {
		if i == t.from {
			continue
		}
		if !dst.Spec.Kind.RunsFirmware() && dst.Spec.Kind != scenario.SDRObserver {
			continue
		}
		rxPHY := e.phyOf(dst.Spec)
		if dst.Spec.Kind != scenario.SDRObserver && !txPHY.sameChannel(rxPHY) {
			continue
		}
		loss, ok := e.pathLoss(t.from, i)
		if !ok {
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: nodes[t.from].Spec.Name,
				To: dst.Spec.Name, PacketID: t.packetID, Outcome: capture.OutOfRange,
				Detail: "no terrain data covers this path"})
			continue
		}
		rxDBm := nodes[t.from].Spec.TxPowerDBm + gain(nodes[t.from].Spec) - loss + gain(dst.Spec)
		noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.Spec))
		if extra := e.emitterNoiseAt(i); !math.IsInf(extra, -1) {
			noiseDBm = addDBm(noiseDBm, extra)
		}
		// 30 dB under the floor: SF12 decodes at -20 dB SNR, so the cull sits
		// well below anything the demodulator could conceivably recover. The
		// calculated path's -10 dB gate would silently discard decodable
		// SF10+ signals here, which is exactly the class of decision this
		// mode exists to stop making.
		if rxDBm <= noiseDBm-30 {
			continue
		}
		isDeaf := false
		for _, other := range concurrent {
			if other.from == i && other.startMs < t.endMs && other.endMs > t.startMs {
				isDeaf = true
				break
			}
		}
		if isDeaf {
			deaf[i] = capture.Reception{
				PacketID: t.packetID, FromNode: nodes[t.from].Spec.Name,
				ToNode: dst.Spec.Name, RSSIdBm: rxDBm, SNRdB: rxDBm - noiseDBm,
				Offered: true, Outcome: capture.NotDemodulated,
			}
			continue
		}
		cands = append(cands, wfCandidate{i: i, rxDBm: rxDBm, noiseDBm: noiseDBm})
	}
	return cands, deaf
}

// judgeWaveform builds one receiver's window, observes it, and demodulates.
//
// The window is the wanted transmission's own airtime; every concurrent
// same-channel transmission lands in it at its true offset, as a waveform -
// never as a dBm sum. Alignment therefore matters, which is the acceptance
// test for this whole mode.
func (e *Engine) judgeWaveform(t transmission, c wfCandidate, concurrent []transmission,
	nodes []*Node, txPHY phy, txSamples []complex128, sentSyms []int,
	cache modCache, seed uint64) wfResult {

	window := len(txSamples)
	txs := []rf.Transmission{{
		Node: nodes[t.from].Spec.Name, Samples: txSamples, GainDB: c.rxDBm,
	}}
	for _, other := range concurrent {
		if other.packetID == t.packetID || other.from == c.i {
			continue
		}
		if other.endMs <= t.startMs || other.startMs >= t.endMs {
			continue
		}
		if !e.phyOf(nodes[other.from].Spec).sameChannel(txPHY) {
			continue
		}
		if tx, ok := e.rxTransmission(other, c.i, t.startMs, nodes, cache); ok {
			txs = append(txs, tx)
		}
	}

	noiseLinear := math.Pow(10, c.noiseDBm/10)
	observed := rf.Observe(txs, rf.Receiver{
		NoisePowerLinear: noiseLinear,
		Seed:             seed,
		// A distinct, deterministic noise stream per packet-receiver pair:
		// same seed, same scenario, same samples, every run.
		Offset: t.packetID*0x9E3779B97F4A7C15 + uint64(c.i)<<32,
	}, window)

	d := dsp.Demodulator{SF: txPHY.sf}
	n := dsp.SamplesPerSymbol(txPHY.sf)
	scratch := make([]complex128, n)
	res := wfResult{snrdB: rf.SNRdB(observed, noiseLinear), preamble: true}
	for s := 0; s < len(sentSyms); s++ {
		lo := s * n
		if lo+n > len(observed) {
			break
		}
		got, _ := d.DemodulateSymbolInto(scratch, observed[lo:lo+n])
		if s < waveformPreambleSyms {
			// Sync is granted, not modelled, until W3 - the engine knows the
			// timing exactly. A corrupted preamble is still worth noticing
			// for the ledger's Demod flag.
			if got != sentSyms[s] {
				res.preamble = false
			}
			continue
		}
		res.total++
		if got != sentSyms[s] {
			res.bad++
		}
	}
	res.decoded = res.total > 0 && res.bad == 0
	return res
}

// settleWaveform is the serial bookkeeping for one judged receiver - the same
// ledger, events, seen-tracking and firmware handoff the calculated path
// keeps, with the demodulator's answer in the driving seat.
func (e *Engine) settleWaveform(t transmission, src, dst *Node, c wfCandidate,
	r wfResult, txPHY phy) {
	rec := capture.Reception{
		PacketID: t.packetID, FromNode: src.Spec.Name, ToNode: dst.Spec.Name,
		RSSIdBm: c.rxDBm, SNRdB: r.snrdB, Offered: true,
		Demod: r.preamble && r.bad < r.total, CRCOK: r.decoded,
	}
	if !r.decoded {
		rec.Outcome = capture.NotDemodulated
		e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
			PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
			SNRdB: r.snrdB, Frame: t.frame,
			Detail: fmt.Sprintf("waveform: %d of %d symbols wrong at %.1f dB measured SNR",
				r.bad, r.total, r.snrdB)})
		e.Ledger.Record(rec)
		e.captureWrite(t, src, dst, txPHY, rec)
		return
	}
	rec.Outcome, rec.FirmwareSaw = capture.Accepted, true
	e.mu.Lock()
	dst.Heard++
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
	detail := "waveform: every symbol decoded; first time this node heard the message"
	if !first {
		detail = "waveform: every symbol decoded; already had this message"
	}
	e.record(Event{AtMs: t.endMs, Kind: "rx", From: src.Spec.Name, To: dst.Spec.Name,
		Frame: t.frame, PacketID: t.packetID, MessageID: t.payload,
		Outcome: rec.Outcome, SNRdB: r.snrdB, Detail: detail})
	if dst.Firmware != nil {
		_ = dst.Firmware.Bridge.Deliver(t.frame)
	}
	e.Ledger.Record(rec)
	e.captureWrite(t, src, dst, txPHY, rec)
}

// recordDeaf is the half-duplex outcome, identical in both modes: something
// measurable arrived and this node's own transmitter was keyed.
func (e *Engine) recordDeaf(t transmission, src, dst *Node,
	rec capture.Reception, txPHY phy) {
	e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.Spec.Name, To: dst.Spec.Name,
		PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
		SNRdB: rec.SNRdB, Frame: t.frame,
		Detail: "its own transmitter was keyed; LoRa is half duplex"})
	e.Ledger.Record(rec)
	e.captureWrite(t, src, dst, txPHY, rec)
}

// captureWrite mirrors the calculated path's pcap tap for one reception.
func (e *Engine) captureWrite(t transmission, src, dst *Node, txPHY phy, rec capture.Reception) {
	e.mu.Lock()
	c := e.capture
	e.mu.Unlock()
	if c != nil && rec.Offered {
		c.write(t.endMs, src.Spec.Name, dst.Spec.Name, txPHY,
			rec.RSSIdBm, rec.SNRdB, rec.Outcome, rec.CRCOK, t.frame)
	}
}
