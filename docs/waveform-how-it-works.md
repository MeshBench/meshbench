# How the waveform simulator actually works

The as-built record for docs/waveform-source-of-truth.md, written from the
code after the build rather than from the plan before it. This is the source
material for user documentation: every stage names its file, and every
number here was measured, not estimated.

The rule the whole thing serves:

> The RF channel produces signals, never verdicts. The receiver produces
> observations, never packets. The demodulator produces packets, and only a
> valid PHY decode may enter MeshCore.

## The switch

`engine.Config.RFMode` (`internal/engine/waveform.go`) selects the physics:
`calculated` - the zero value, so every pre-existing scenario is untouched -
or `waveform`. It is chosen in Configuration's **RF Simulation** section
(`internal/workbench/configcards.go, panels6c.go`), applied live on a
whole-transmission boundary (`Engine.SetRFMode`), persisted in the prefs
file (`internal/session/prefs.go`), stamped into saved runs
(`RunRecord.RFMode`, `internal/session/runs.go`) and shown in the shell
chrome while a run plays (`internal/gui/shell/layout.go`, the counts line).
The verb is `rf.mode`; everything the panel does goes through it.

## A packet's life in waveform mode

**Transmit.** MeshCore firmware writes a frame to its virtual radio; the
bridge surfaces it (`firmware.Bridge.Transmitted`) and
`Engine.startTransmission` prices its airtime with `dsp.AirtimeMillis` -
RadioLib's own `getTimeOnAir`, which MeshCore's CSMA is built on. When the
airtime elapses, `completeTransmissions` (`internal/engine/transmissions.go`)
collects it for delivery, together with everything that shared the air:
still-in-flight transmissions, others that ended this tick, and - the fix
the MS1 test forced - transmissions that ended earlier but overlapped
something still up (`Engine.recent`). Before that fix a short interferer
became invisible the moment it stopped, in both modes.

**Synthesis.** `frameSamples` (`internal/engine/baseband.go`) renders the
frame exactly as an SX126x would send it: `lora.Encode`
(`internal/lora`) runs the coding chain - explicit header (with its
checksum), payload whitening (LFSR x⁸+x⁶+x⁵+x⁴+1), CRC16, Hamming at the
frame's coding rate, the diagonal interleaver (header block always at SF-2,
data blocks at SF-2 under LDRO), Gray mapping - and
`dsp.FrameLayout.FrameSamples` (`internal/dsp/sync.go`) wraps the data
symbols in MeshCore's own preamble length (32 upchirps at SF≤8, 16 above),
the 0x12 sync word, and the 2.25-symbol downchirp SFD. That 2.25 is
RadioLib's `sfCoeff1 = 4.25` made flesh: the sample stream and the airtime
formula describe the same frame, held equal by test
(`TestSymbolCountMatchesRadioLib`).

Samples are synthesised once per packet per delivery batch (`modCache`) and
shared by every receiver, because unit-amplitude baseband does not depend on
who is listening.

**The channel.** `rxTransmission` (`baseband.go`) prices one transmission
for one receiver: TX power, antenna gains in the true directions, path loss
(free space + ITU-R P.526 terrain diffraction + buildings + the ADR-0015
excess term - `internal/engine/links.go`), the timing offset into the
window, and the oscillator disagreement as a per-sample phase ramp
(`rf.Transmission.PhaseStepRad`). When multipath is switched on, `echoFor`
(`internal/engine/realism.go`) adds one geometric echo per path -
deterministic excess delay and carrier phase, drifted by the fading rate.
`rf.Observe` (`internal/rf/channel.go`) sums everything coherently -
fractional delay carried as phase - and adds Philox counter-based AWGN at
the receiver's own noise floor (plus implementation loss when configured).
The same synthesis feeds the verdicts, the waterfall
(`InFlightTransmissions`), CAD, and the SDR observers; if they rendered from
different signals the pictures would lie about the physics.

**Receive.** `judgeWaveform` (`internal/engine/waveform.go`), in parallel
across candidate receivers with serial bookkeeping so ledgers stay
byte-deterministic: saturate the front end if configured; measure SNR from
the window as telemetry; then `dsp.Detect` runs the real front end - the
preamble found by dechirped bins holding still, the boundary from the SFD
(calibrated relations: bin_up = cfo − sto, bin_down = cfo + sto), a
three-candidate contest to resolve whole-symbol aliasing (judged by the
window *before* each candidate: the true start is preceded by SFD mush, a
late one by a pristine symbol), and a ±2-sample fine stage, because a
one-sample slip shifts every bin. No lock, no packet - "no preamble lock" is
its own ledger entry. Then `CorrectCFO`, per-symbol FFT demodulation, and
`lora.Decode`: Gray, deinterleave, Hamming (4/7 and 4/8 correct one bit per
codeword, 4/5 and 4/6 detect only - the chip's split), dewhiten, header
parse, CRC. The decoder consumes only the frame its header declares, so a
streaming window's tail is the channel's business, not damage.

**The verdict.** `settleWaveform` records the outcome. What reaches MeshCore
is `Bridge.Deliver(r.payload)` - **the decoded bytes, not the transmitted
frame**. With a valid CRC they are the same bytes; on the day a corrupted
frame passes CRC by chance, that is the chip's behaviour too. Misses say
why: no preamble lock, header unreadable, or N codewords beyond repair with
the FEC repair count alongside. SNR and RSSI ride in the ledger as
telemetry; nothing reads them to decide.

**Carrier sense.** In waveform mode `channelBusy` defers to `waveformBusy`
(`waveform.go`): one symbol of summed IQ per listening node per tick,
dechirped, peak-against-mean (`dsp.CADBusy`) - the chip's own detector,
which fires below the decode floor, which is why listen-before-talk works.
The busy verdict feeds the firmware over `kindChannelBusy`
(`internal/firmware/bridge.go`), so MeshCore's CSMA and backoff respond to
actual RF with no engine rule in between.

## Calculated mode, and the hybrid

`deliver` (`transmissions.go`) is the fast model, unchanged in spirit: link
budget, dBm-summed interference, verdict against the demodulator floor.
A node with `scenario.Node.TrueRF` set (the Radio tab's switch, verb
`node.truerf`) is handed to `judgeHybrid` instead - the full waveform
reception inside a calculated run. The divergence harness
(`TestModeDivergence`, run with `-v` for the report) diffs the two modes
over one scenario; its first finding: on a dense six-sender burst,
calculated's no-capture interference model calls roughly half the
collision-affected pairs lost that the demodulator actually captures.

## The SDR observer

`Engine.ObserveSpan` (`internal/engine/observeriq.go`) renders any node's
antenna over a span of simulated time from the same synthesis - never from
packet events. `sdr.ServeRTLTCP` (`internal/sdr/rtltcp.go`) speaks rtl-sdr's
own network protocol, so SDR++'s stock client connects: RTL0 header,
five-byte tuning commands, unsigned 8-bit interleaved IQ (the format's
~48 dB ceiling is owned in shortcomings). Verbs `sdr.serve` / `sdr.stop`;
one client at a time, like the real server. `Engine.SetNodePosition` moves
a node live and forgets its cached losses, so dragging an observer on the
map (`nodes.move`) changes what an attached client hears on the next
window. A paused engine holds the stream at the pause point rather than
inventing future air.

## Buildings

`internal/environ` holds what physically stands there: footprints, heights,
and a MeshBench-owned material taxonomy, every derived value carrying its
source and confidence. Tiles (gzipped JSON lines per zoom-14 slippy tile)
are produced offline by `tools/envgen` from Microsoft Global ML Building
Footprints or OSM GeoJSON, loaded on demand, cached, and *missing tiles are
counted, never mistaken for empty ground*. `buildingLossDB` (`links.go`)
prices them at the run's frequency: each crossed building is a P.526 knife
edge at its rooftop, plus one wall of material loss when the direct ray
passes through. Both modes pay it, because buildings change `GainDB`, never
verdicts. Verb `rf.environment`; loading or dropping the environment
forgets the link cache, because two physics must not share one matrix.

## Determinism

Same seed, same scenario, same ledger, in every mode - pinned by
`TestWaveformModeIsDeterministic` and `TestMultipathIsDeterministicGeometry`.
The pieces that make it true: Philox counter noise keyed by packet and
receiver, crystal offsets and echo geometry derived from names by hash,
parallel DSP with serial bookkeeping, and deaf-map iteration in node order.
The known exception remains emulated firmware, which runs on wall time.

## Measured numbers

Dev machine: Ryzen 5 3600XT (12 threads), RDNA2 GPU, Vulkan.

| what | figure |
|---|---|
| 300-node, 20-sender flood burst (~5 s simulated), calculated | 46 ms |
| same burst, waveform, symbol-level verdicts (W1) | 1.04 s |
| same burst, full coding chain (W2) | 1.89 s |
| same burst, with receiver front end in path (W3) | 2.29 s |
| `rf.Observe` phase-rotation hoist (the W1 snag) | 5.07 s → 1.04 s |
| packet sensitivity vs Semtech floors (full chain) | brackets, by test |
| GPU demod, SF9 × 512 symbols | CPU 4.53 ms, GPU 1.11 ms (4.1×) |

The heaviest burst runs ~2.2× faster than real time on the CPU alone. The
remaining profile is roughly half Gaussian noise synthesis and a fifth FFT.

## Where the build diverged from the plan

- **Tile format**: gzipped JSON lines per tile, not GeoParquet - no new
  dependency; upgrade recorded if a region outgrows it.
- **GPU is an accelerator, not the judge**: f32 can split a decision the
  noise made a near-tie, and the W6 gate itself says GPU on/off must never
  change an outcome. Confidence-gated hybrid demodulation is the recorded
  follow-up.
- **The MS1 test sharpened twice**: with real FEC a grazing 0 dB collision
  is honestly marginal (coding repairs what alignment smears), so the gate
  contrast became superimposed-preambles versus cleared air - which the
  calculated model waves through identically.
- **Two pre-existing engine bugs fixed on the way**: the per-sample
  `cmplx.Exp` in `rf.Observe` (half of all waveform CPU), and ended
  transmissions vanishing from the interference set in both modes.
- **Adjacent-channel rejection deferred**: it needs frequency-domain
  modelling the one-channel baseband cannot express.
- **Per-transmission receive windows**: overlapping packets are each judged
  in their own window, so one physical decode can in principle be
  attributed to two packets; a continuous per-receiver stream is the
  eventual shape (it is what the SDR observer already does).
- **Bit-level conventions await silicon**: whitening phase, header
  checksum, sync-word values are implemented from the reverse-engineering
  literature and self-consistent; `internal/lora`'s golden-vector test arms
  itself the moment `testdata/golden-*.json` files exist.

## What still needs a human or hardware

1. **SDR++ eyes-on**: `sdr.serve` an observer, connect SDR++'s rtl_tcp
   source to the printed address, set the client sample rate to the printed
   rate; watch transmissions, collisions, and the effect of dragging the
   observer.
2. **Golden vectors**: run gr-lora_sdr (GNU Radio) or demodulate a real
   SX1262 capture offline; write `internal/lora/testdata/golden-*.json`.
3. **MS4 with firmware in the loop**: a live-firmware run in waveform mode
   showing CSMA deferral change as RF conditions change.
4. **The SX1262 ladders** (W5): sensitivity, CFO, capture, collision-timing
   sweeps against the real chip; tolerances into shortcomings.
5. **The excess-loss re-fit** (W8): envgen over a real Scotland footprint
   extract, `rf.environment`, then `validate.fetch` / `validate.calibrate`;
   the +20 dB fudge should shrink measurably.
