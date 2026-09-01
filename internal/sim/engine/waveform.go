// The waveform verdict: reception decided by demodulating what arrived.
//
// In waveform mode the channel produces a signal and the demodulator produces
// the verdict. There is no code here that compares an SNR to a threshold -
// capture effect, partial overlap and interference alignment all emerge from
// summed IQ and an FFT, or they do not happen at all. SNR is still measured
// and recorded, but as telemetry: it is never the reason.
//
// The verdict is the full receive chain: demodulated symbols through Gray,
// the diagonal deinterleaver, Hamming FEC, dewhitening, the explicit header
// and the payload CRC (internal/lora). What a receiver hands MeshCore is the
// decoded bytes - not the transmitted frame - so a repair is a repair and a
// CRC failure is a loss, exactly as on the chip.
package engine

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/lora"
	"github.com/MeshBench/meshbench/internal/sim/capture"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// RFMode selects which physics decides reception.
type RFMode string

const (
	// RFCalculated is the fast model: link budget, noise and interference in
	// dBm, verdict by demodulator floor. The zero value, so every existing
	// scenario is untouched.
	RFCalculated RFMode = "calculated"
	// RFWaveform is the authoritative model: IQ through channel.Observe, verdict
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

// SetTrueRF flags one node for waveform verdicts inside a calculated run,
// and reports whether it found the node.
//
// Here rather than in the caller because the flag lives on the engine's own
// copy of the spec, and that copy is swapped whole rather than written
// through: a caller reaching in to set a field would be editing a value other
// goroutines are already reading.
func (e *Engine) SetTrueRF(name string, on bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.nodes {
		if n.specRef().Name != name {
			continue
		}
		n.changeSpec(func(s *scenario.Node) { s.TrueRF = on })
		return true
	}
	return false
}

// modCache is one delivery batch's modulated baseband, keyed by packet.
//
// A transmission's unit-amplitude samples do not depend on who is listening,
// so they are synthesised once per batch and shared by every receiver - the
// difference between one modulation per packet and one per packet-receiver
// pair, which on a flood is the difference between fast and quadratic.
type modCache map[uint64][]complex128

func (e *Engine) modulated(c modCache, t transmission, p phy) []complex128 {
	if s, ok := c[t.packetID]; ok {
		return s
	}
	s := frameSamples(t.frame, p)
	c[t.packetID] = s
	return s
}

// wfCandidate is one receiver that cleared the cheap gates and is worth DSP.
type wfCandidate struct {
	i        int
	rxDBm    float64
	noiseDBm float64
	// heldBy names whoever already had this receiver's one demodulator. Set
	// here rather than discovered by the judge because a receiver that is not
	// listening is not worth synthesising a window for - and because the judge
	// runs one goroutine per pair, which is precisely the shape that cannot
	// see another pair's outcome.
	heldBy string
}

// wfResult is what the receive chain said for one candidate.
type wfResult struct {
	decoded bool
	synced  bool   // the front end found and locked the preamble
	payload []byte // what MeshCore actually gets - the decode, not the send
	stats   lora.DecodeStats
	snrdB   float64
}

// deliverWaveform is deliver's waveform-mode twin: same gates, same ledger,
// different judge. The DSP runs in parallel across receivers; the bookkeeping
// stays serial, in node order, so the ledger and event log are deterministic.
// wfDelivery is one finished transmission prepared for judgement: its
// synthesis, its candidates, and - after the batch pool runs - its results.
type wfDelivery struct {
	t         transmission
	src       *Node
	txPHY     phy
	txSamples []complex128
	cands     []wfCandidate
	deaf      map[int]capture.Reception
	results   map[int]wfResult
}

// prepareWaveform is the serial half: synthesis into the shared cache (a
// lazily-filled map under parallel judges would be a data race) and the
// cheap candidate gates.
func (e *Engine) prepareWaveform(t transmission, concurrent []transmission,
	nodes []*Node, cache modCache) *wfDelivery {
	src := nodes[t.from]
	txPHY := e.phyOf(src.specRef())
	d := &wfDelivery{
		t: t, src: src, txPHY: txPHY,
		txSamples: e.modulated(cache, t, txPHY),
	}
	for _, other := range concurrent {
		if other.packetID != t.packetID {
			e.modulated(cache, other, e.phyOf(nodes[other.from].specRef()))
		}
	}
	d.cands, d.deaf = e.waveformCandidates(t, concurrent, nodes, txPHY)
	d.results = make(map[int]wfResult, len(d.cands))
	return d
}

// deliverWaveformBatch judges every finished transmission's every candidate
// in one pool, then settles in transmission order. One pool rather than one
// per transmission because several finishing on the same tick is exactly
// the busy case: judged one at a time, a batch of small candidate sets
// leaves most of the machine idle while the clock stalls.
func (e *Engine) deliverWaveformBatch(done []transmission,
	concurrent []transmission, cache modCache) error {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	seed := e.Config.Seed
	e.mu.Unlock()

	deliveries := make([]*wfDelivery, len(done))
	for i, t := range done {
		deliveries[i] = e.prepareWaveform(t, concurrent, nodes, cache)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for _, d := range deliveries {
		for _, c := range d.cands {
			d, c := d, c
			if c.heldBy != "" {
				// No window is built for a receiver that was never listening.
				// Settlement still reports it: a packet lost to a busy
				// demodulator is a cause, not an absence.
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				r := e.judgeWaveform(d.t, c, concurrent, nodes, d.txPHY,
					d.txSamples, cache, seed)
				mu.Lock()
				d.results[c.i] = r
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	// Settlement is the ledger, and the ledger's order is part of the
	// result: transmission order, then candidate order, then node order for
	// the deaf - exactly the sequence the one-at-a-time loop produced.
	for _, d := range deliveries {
		for _, c := range d.cands {
			e.settleWaveform(d.t, d.src, nodes[c.i], c, d.results[c.i], d.txPHY)
		}
		for i := range nodes {
			if rec, ok := d.deaf[i]; ok {
				e.recordDeaf(d.t, d.src, nodes[i], rec, d.txPHY)
			}
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
		if !dst.specRef().Kind.RunsFirmware() && dst.specRef().Kind != scenario.SDRObserver {
			continue
		}
		rxPHY := e.phyOf(dst.specRef())
		if dst.specRef().Kind != scenario.SDRObserver && !txPHY.sameChannel(rxPHY) {
			continue
		}
		loss, ok := e.pathLoss(t.from, i)
		if !ok {
			e.record(Event{AtMs: t.endMs, Kind: "miss", From: nodes[t.from].specRef().Name,
				To: dst.specRef().Name, PacketID: t.packetID, Outcome: capture.OutOfRange,
				Class:  noTerrainDataClass,
				Detail: "no terrain data covers this path"})
			continue
		}
		rxDBm := e.rxPowerDBm(nodes, t.from, i, loss)
		noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(dst.specRef()))
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
				PacketID: t.packetID, FromNode: nodes[t.from].specRef().Name,
				ToNode: dst.specRef().Name, RSSIdBm: dsp.ReportRSSIdBm(rxDBm),
				SNRdB:   dsp.ReportSNRdB(rxDBm - noiseDBm),
				Offered: true, Outcome: capture.NotDemodulated,
			}
			continue
		}
		cands = append(cands, wfCandidate{i: i, rxDBm: rxDBm, noiseDBm: noiseDBm,
			heldBy: e.demodulatorHeldBy(i, t, concurrent, nodes, txPHY)})
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
	nodes []*Node, txPHY phy, txSamples []complex128,
	cache modCache, seed uint64) wfResult {

	window := len(txSamples)
	wanted := channel.Transmission{
		Node: nodes[t.from].specRef().Name, Samples: txSamples, GainDB: c.rxDBm,
		PhaseStepRad: e.phaseStepFor(nodes[t.from], nodes[c.i], txPHY),
	}
	txs := []channel.Transmission{wanted}
	if echo, has := e.echoFor(wanted, nodes[t.from].specRef().Name,
		nodes[c.i].specRef().Name, txPHY, t.startMs); has {
		txs = append(txs, echo)
	}
	for _, other := range concurrent {
		if other.packetID == t.packetID || other.from == c.i {
			continue
		}
		if other.endMs <= t.startMs || other.startMs >= t.endMs {
			continue
		}
		if !e.phyOf(nodes[other.from].specRef()).sameChannel(txPHY) {
			continue
		}
		txs = append(txs, e.rxTransmissions(other, c.i, float64(t.startMs), nodes, cache)...)
	}

	noiseLinear := math.Pow(10, e.applyImplementationLoss(c.noiseDBm)/10)
	observed := channel.Observe(txs, channel.Receiver{
		NoisePowerLinear: noiseLinear,
		Seed:             seed,
		// A distinct, deterministic noise stream per packet-receiver pair:
		// same seed, same scenario, same samples, every run.
		Offset: t.packetID*0x9E3779B97F4A7C15 + uint64(c.i)<<32,
	}, window)

	e.saturate(observed)
	// Measured from the IQ, then clamped to what the modem could report. The
	// front end saturates before the estimator does, so a window this strong
	// reads as the ceiling on the chip too.
	res := wfResult{snrdB: dsp.ReportSNRdB(channel.SNRdB(observed, noiseLinear))}
	// The receiver front end is real: no lock, no packet. Detection, sync
	// word timing and the SFD's STO/CFO split all run against the observed
	// IQ - a preamble buried in interference fails here exactly as it does
	// on the chip, and that failure is its own ledger entry.
	sync, locked := dsp.Detect(observed, frameLayout(txPHY))
	if !locked {
		return res
	}
	res.synced = true
	if sync.CFOBins != 0 {
		dsp.CorrectCFO(observed, txPHY.sf, sync.CFOBins)
	}
	d := dsp.Demodulator{SF: txPHY.sf}
	n := dsp.SamplesPerSymbol(txPHY.sf)
	scratch := make([]complex128, n)
	var shifts []int
	for at := sync.DataStart; at+n <= len(observed); at += n {
		got, _ := d.DemodulateSymbolInto(scratch, observed[at:at+n])
		shifts = append(shifts, got)
	}
	res.payload, res.decoded, res.stats = lora.Decode(loraParams(txPHY), shifts)
	return res
}

// settleWaveform is the serial bookkeeping for one judged receiver - the same
// ledger, events, seen-tracking and firmware handoff the calculated path
// keeps, with the demodulator's answer in the driving seat.
func (e *Engine) settleWaveform(t transmission, src, dst *Node, c wfCandidate,
	r wfResult, txPHY phy) {
	rec := capture.Reception{
		PacketID: t.packetID, FromNode: src.specRef().Name, ToNode: dst.specRef().Name,
		RSSIdBm: dsp.ReportRSSIdBm(c.rxDBm), SNRdB: r.snrdB, Offered: true,
		Demod: r.stats.HeaderOK, CRCOK: r.decoded && r.stats.CRCOK,
	}
	if c.heldBy != "" {
		// Never demodulated, so there is no measured SNR to report: the
		// estimate the gates produced is the honest figure, and it is what
		// the receiver would have seen had it been free to look.
		rec.SNRdB = dsp.ReportSNRdB(c.rxDBm - c.noiseDBm)
		rec.Outcome = capture.NotDemodulated
		e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.specRef().Name, To: dst.specRef().Name,
			PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
			SNRdB: rec.SNRdB, Frame: t.frame, Class: ClassReceiverBusy,
			Detail: busyDemodulatorDetail(c.heldBy)})
		e.Ledger.Record(rec)
		e.captureWrite(t, src, dst, txPHY, rec)
		return
	}
	if !r.decoded {
		rec.Outcome = capture.NotDemodulated
		why := fmt.Sprintf(
			"waveform: header unreadable at %.1f dB measured SNR", r.snrdB)
		if !r.synced {
			why = fmt.Sprintf(
				"waveform: no preamble lock at %.1f dB measured SNR", r.snrdB)
		}
		if r.stats.HeaderOK {
			why = fmt.Sprintf(
				"waveform: %d codeword(s) beyond repair, %d repaired, CRC %v, at %.1f dB",
				r.stats.Failed, r.stats.Corrected, r.stats.CRCOK, r.snrdB)
		}
		e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.specRef().Name, To: dst.specRef().Name,
			PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
			SNRdB: r.snrdB, Frame: t.frame, Class: waveformMissClass(c, txPHY.sf),
			Detail: why})
		e.Ledger.Record(rec)
		e.captureWrite(t, src, dst, txPHY, rec)
		return
	}
	rec.Outcome, rec.FirmwareSaw = capture.Accepted, true
	e.mu.Lock()
	dst.Heard++
	if e.seen[dst.specRef().Name] == nil {
		e.seen[dst.specRef().Name] = map[uint64]bool{}
	}
	first := !e.seen[dst.specRef().Name][t.payload]
	e.seen[dst.specRef().Name][t.payload] = true
	if first {
		src.UniqueDelivery++
	} else {
		src.RedundantRelay++
	}
	e.mu.Unlock()
	detail := "waveform: decoded, CRC valid; first time this node heard the message"
	if r.stats.Corrected > 0 {
		detail = fmt.Sprintf(
			"waveform: decoded with %d codeword(s) repaired by FEC, CRC valid", r.stats.Corrected)
	}
	if !first {
		detail += "; already had this message"
	}
	e.record(Event{AtMs: t.endMs, Kind: "rx", From: src.specRef().Name, To: dst.specRef().Name,
		Frame: t.frame, PacketID: t.packetID, MessageID: t.payload,
		Outcome: rec.Outcome, SNRdB: r.snrdB, Detail: detail})
	if dst.Firmware != nil {
		// The decode, not the transmitted frame: what arrives at MeshCore is
		// whatever the receive chain produced. With a valid CRC they are the
		// same bytes - and on the day they are not, that is the chip's
		// behaviour too.
		_ = dst.Firmware.Bridge.Deliver(r.payload)
	}
	e.Ledger.Record(rec)
	e.captureWrite(t, src, dst, txPHY, rec)
}

// recordDeaf is the half-duplex outcome, identical in both modes: something
// measurable arrived and this node's own transmitter was keyed.
func (e *Engine) recordDeaf(t transmission, src, dst *Node,
	rec capture.Reception, txPHY phy) {
	e.record(Event{AtMs: t.endMs, Kind: "miss", From: src.specRef().Name, To: dst.specRef().Name,
		PacketID: t.packetID, MessageID: t.payload, Outcome: rec.Outcome,
		SNRdB: rec.SNRdB, Frame: t.frame, Class: ClassHalfDuplex,
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
		c.write(t.endMs, src.specRef().Name, dst.specRef().Name, txPHY,
			rec.RSSIdBm, rec.SNRdB, rec.Outcome, rec.CRCOK, t.frame)
	}
}
