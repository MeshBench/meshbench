// The waveform channel as the firmware's CSMA hears it: carrier sense
// from the same summed IQ the verdicts use, and the hybrid judgement.
// Split from waveform.go at the file limit.
package engine

import (
	"math"
	"runtime"
	"sync"

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// waveformBusy is channelBusy's waveform-mode twin: MeshCore's own
// listen-before-talk fed by the chip's actual question - does one symbol of
// dechirped IQ concentrate the way a chirp's does - instead of by an SNR
// comparison. This is what makes the firmware's CSMA and backoff emergent:
// change the RF and the MAC changes, with no engine rule in between.
func (e *Engine) waveformBusy(now uint32, nodes []*Node, air []transmission) []bool {
	busy := make([]bool, len(nodes))
	cache := e.cadCache(air)
	// Fill the cache serially before the listeners fan out: the workers
	// only read it, and a lazily-filled map under them would be a race.
	for _, t := range air {
		e.modulated(cache, t, e.phyOf(nodes[t.from].Spec()))
	}
	// Every listener in parallel. Three hundred nodes each dechirping a
	// symbol every tick is real work, it lands on the tick that paces the
	// engine's clock, and each answer is an independent write to its own
	// index - the cheapest kind of parallelism there is.
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, dst := range nodes {
		// By kind, not by attached process: what can listen is a property of
		// the node, and the physics must not change because a test runs the
		// channel without processes on it.
		if !dst.Spec().Kind.RunsFirmware() {
			continue
		}
		i, dst := i, dst
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			rxPHY := e.phyOf(dst.Spec())
			var txs []channel.Transmission
			for _, t := range air {
				if t.from == i {
					continue
				}
				if !e.phyOf(nodes[t.from].Spec()).sameChannel(rxPHY) {
					continue
				}
				if tx, ok := e.rxTransmission(t, i, float64(now), nodes, cache); ok {
					txs = append(txs, tx)
				}
			}
			if len(txs) == 0 {
				return
			}
			n := dsp.SamplesPerSymbol(rxPHY.sf)
			noiseDBm := dsp.NoiseFloorDBm(rxPHY.bandwidthHz, e.noiseFigOf(dst.Spec()))
			window := channel.Observe(txs, channel.Receiver{
				NoisePowerLinear: math.Pow(10, noiseDBm/10),
				Seed:             e.Config.Seed,
				Offset:           uint64(now)*0xD1B54A32D192ED03 + uint64(i)<<40,
			}, n)
			busy[i] = dsp.CADBusy(window, rxPHY.sf)
		}()
	}
	wg.Wait()
	return busy
}

// cadCache keeps in-flight transmissions' baseband across ticks: CAD asks
// every tick, and re-synthesising whole frames each time would cost more
// than the FFTs it feeds. Entries leave when their transmission does.
func (e *Engine) cadCache(air []transmission) modCache {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.wfCAD == nil {
		e.wfCAD = modCache{}
	}
	alive := map[uint64]bool{}
	for _, t := range air {
		alive[t.packetID] = true
	}
	for id := range e.wfCAD {
		if !alive[id] {
			delete(e.wfCAD, id)
		}
	}
	return e.wfCAD
}

// ChannelBusyForTest exposes the carrier-sense vector, so a test can hold
// the CAD path to the physics without a firmware process in the loop.
func (e *Engine) ChannelBusyForTest(now uint32) []bool { return e.channelBusy(now) }

// judgeHybrid runs the full waveform reception for one receiver inside a
// calculated-mode delivery - the per-node True RF flag. Reports whether it
// handled the receiver; the cheap gates that would have skipped it entirely
// (terrain, deafness) fall through to the calculated path's own handling.
func (e *Engine) judgeHybrid(t transmission, rxIdx int, concurrent []transmission,
	nodes []*Node, txPHY phy, cache modCache) bool {
	loss, ok := e.pathLoss(t.from, rxIdx)
	if !ok {
		return false
	}
	for _, other := range concurrent {
		if other.from == rxIdx && other.startMs < t.endMs && other.endMs > t.startMs {
			return false // deaf: the calculated path's report is already right
		}
	}
	src := nodes[t.from]
	rxDBm := src.Spec().TxPowerDBm + gain(src.Spec()) - loss + gain(nodes[rxIdx].Spec())
	noiseDBm := dsp.NoiseFloorDBm(txPHY.bandwidthHz, e.noiseFigOf(nodes[rxIdx].Spec()))
	if extra := e.emitterNoiseAt(rxIdx); !math.IsInf(extra, -1) {
		noiseDBm = addDBm(noiseDBm, extra)
	}
	if rxDBm <= noiseDBm-30 {
		return false // nothing recoverable; let the calculated path narrate
	}
	e.mu.Lock()
	seed := e.Config.Seed
	e.mu.Unlock()
	c := wfCandidate{i: rxIdx, rxDBm: rxDBm, noiseDBm: noiseDBm,
		heldBy: e.demodulatorHeldBy(rxIdx, t, concurrent, nodes, txPHY)}
	if c.heldBy != "" {
		// One demodulator here too: a True RF receiver does not get a second
		// one just because the rest of the run is calculated.
		e.settleWaveform(t, src, nodes[rxIdx], c, wfResult{}, txPHY)
		return true
	}
	txSamples := e.modulated(cache, t, txPHY)
	r := e.judgeWaveform(t, c, concurrent, nodes, txPHY, txSamples, cache, seed)
	e.settleWaveform(t, src, nodes[rxIdx], c, r, txPHY)
	return true
}
