# Waveform performance plan

The observed failure: in waveform mode with several transmitters up, the
simulation falls behind the wall until it near-stops; the SDR observer,
honestly bound to simulated time, stalls with it. The brief: quicker than
quick, benchmark-driven, and nothing that trades away determinism or the
CPU/GPU twin rule.

## Measured baseline (2026-08-18, Ryzen 5 3600XT)

`BenchmarkConcurrentWaveform*`: 40 nodes flat-earth, 5 s simulated.

| scenario | wall |
|---|---|
| 8 talkers, waveform | 199 ms |
| 3 talkers, waveform | 113 ms |
| 8 talkers, calculated | 1.2 ms |

CPU profile of the 8-talker run, cumulative:

| where | share |
|---|---|
| `rf.Observe` (window synthesis) | 65% |
| — of which `Philox.normalPair` (Box-Muller noise: log+sin+cos+sqrt) | 46% |
| `dsp.FFT` (no twiddle/bit-rev caching) | 24% |
| `dsp.Detect` front end | 15% |

The engine alone runs 25× realtime on flat earth even at 8 talkers - so
the field stall is the engine cost *times* what the bench leaves out: real
terrain, hundreds of nodes, CAD per listening node per tick, the observer
pump, and the store's per-tick readouts. The engine's DSP is still the
right first target: every one of those multipliers multiplies it.

## Workstreams, in order

**P0a - noise without transcendentals.** Replace Box-Muller with the
AS241 inverse-CDF polynomial: same Philox counters, same determinism
contract, no log/sin/cos per sample, correct tails (the FEC threshold
lives in the tails; a bounded sum-of-uniforms would flatter it). One-time
consequence, stated loudly: noise realisations change, so seeded results
change once, and outcome-pinned tests are re-tuned in the same commit.

**P0b - FFT twiddle and bit-reversal caches.** Sizes are a handful of
powers of two; cache the tables per size instead of recomputing the
rotation chain every call.

**P0c - benchmark gates.** The concurrent bench above plus
`BenchmarkObserveSpanDuringTransmission` run before and after; the plan's
acceptance is a measured multiple on the 8-talker case, not a feeling.

**P1 - the store's tick.** The engine clock is paced by the store tick,
so anything O(traffic) there stalls simulated time itself: the 2000-event
tail is rebuilt and re-classed every tick, stats are recomputed over it,
trails rescan a 4 s window. Make the readouts incremental (event log is
append-only; a cursor beats a rescan). Also audit `EventsSince`/
`EventsTail` for full scans.

**P2 - GPU synthesis and noise.** The biggest headroom and the biggest
lift: `rf.Observe` (sum + AWGN) and the dechirp/FFT of `dsp.Detect` as
WGSL kernels behind the existing GPU switch, CPU twin tested, f32
near-tie rule as with demod. Do after P0/P1 land, sized by what remains.

**P3 - foreign kernels (cgo/SIMD) only if still short.** The project
prefers no new dependencies and already owns a GPU path; hand-rolled
assembly or cgo SIMD is the last resort, not the second step.

## Landed (2026-08-18, same day)

- **P0a** - Box-Muller replaced by Acklam's inverse CDF: 18.75 ns per
  pair, tails verified by test (3-sigma and 4-sigma fractions). Seeded
  realisations moved once; every outcome-pinned test passed unchanged.
- **P0b** - FFT plans: cached twiddles and bit-reversal per size.
- **Structural, and probably the field stall itself**: a geometry-keyed
  profile cache. A radio report - which MeshCore firmware effectively
  issues around every transmission, via the FEM/AGC fields - invalidated
  the pair's loss, and recomputing the loss re-walked up to 257 DEM
  lookups per pair, synchronously, inside the judge path. On a real
  terrain network with several transmitters that is exactly "stutters
  until it pretty much stops". The profile now outlives every radio
  report (test: a report costs arithmetic, zero DEM lookups) and dies
  only when a node actually moves.

- **P1** - the store's readouts run at ten hertz, not per step: the tick
  paces the engine's clock, and the 2000-event tail conversion, per-node
  bridge stats and trail scan were re-describing tables nobody could
  re-read yet. A paused or hand-stepped run still refreshes every tick,
  because a person stepping once is looking straight at it.
- **Batch judgement**: every transmission finishing on a tick is judged
  in one parallel pool over all (transmission, receiver) pairs, then
  settled in the ledger's fixed order - several finishing together is
  exactly the busy case, and one-at-a-time left the machine idle on each
  one's small candidate set.

Bench movement (engine-only, flat earth, so the profile cache and the
readout throttle do not show here): 8 talkers 199 -> 134 ms, 3 talkers
113 -> 50 ms per 5 s simulated. Remaining profile: Observe accumulation,
FFT, noise - the P2 GPU synthesis targets, with the caveat that
verdict-path synthesis must stay bit-identical CPU (GPU on/off must not
change an outcome), so P2's honest scope is the presentation surfaces
plus accelerator-with-fallback patterns like the demod's.

## Non-goals

Approximations that change verdicts (the demodulator stays exact), mode
divergence, or anything that makes GPU-on/GPU-off change an outcome.
