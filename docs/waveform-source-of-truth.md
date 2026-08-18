# Waveform as the source of truth

The RF channel produces signals, never verdicts. The receiver produces
observations, never packets. The demodulator produces packets, and only a
valid PHY decode may enter MeshCore.

That rule is already written into this codebase (`internal/rf/channel.go`'s
package comment, ADR-0003) — and it is not yet true. This plan makes it true,
behind a switch, without giving up the fast model that makes country-sized
scenarios possible.

## Where the code actually is

Verified against the tree, because a plan built on what we hoped was there
would fail in its first week:

**Already built and validated:**

- `rf.Observe()` sums waveforms coherently: per-transmission gain, fractional
  delay carried as phase (which is what lets two arrivals cancel), partial
  overlap via `StartSample`, deterministic Philox AWGN. The channel decides
  nothing, exactly as the rule demands.
- `dsp.Modulator`/`dsp.Demodulator` are a real symbol-level LoRa modem:
  chirp synthesis, dechirp, FFT, symbol decisions. Monte Carlo sensitivity
  lands within 1.6 dB of Semtech's published SX1262 floors
  (`docs/shortcomings.md`), which is the evidence the modem is honest.
- `engine.InFlightTransmissions()` already synthesises what is on the air as
  baseband for any receiver — same modulator, same link budgets.
- GPU FFT exists in `shaders/` with the CPU implementation as the tested
  oracle (ADR-0004).
- `internal/sdr` has spectrogram, SigMF export, WAV, and a real
  tune-filter-resample listen path.

**The gap, precisely:**

- Packet delivery in `engine.deliver()` (`internal/engine/transmissions.go`)
  is `effective SNR >= requiredSNRdB(sf)` with interference summed in dBm and
  a 6 dB capture constant. The waveform machinery feeds the waterfall only.
  The simulator's verdicts and its pictures come from two different models.
- `symbolsFor()` is a placeholder byte→symbol map. There is no Gray coding,
  interleaving, Hamming FEC, whitening, header, or payload CRC anywhere. The
  waterfall shows plausible chirps, not bit-exact LoRa.
- There is no preamble/sync/CFO/STO receiver front end — the engine has
  never needed one, because it knows the timing exactly.

## Decisions

**D1 — the PHY is ours, the validation is not.** The plan's instinct to
integrate an existing proven PHY was researched and the answer is: nothing
embeddable exists. Every complete open implementation (gr-lora_sdr from
EPFL, SDR-LoRa) is GNU Radio-shaped, and shipping GNU Radio breaks the
one-binary rule outright. So the coding chain is implemented here, in Go, in
a package that imports nothing MeshBench-specific (`internal/lora`,
extractable to its own repository later — which preserves the reusability
the plan wanted). What we take from gr-lora_sdr (GPL-3.0, compatible) is
**golden IQ vectors**: generated offline, exchanged over SigMF, checked into
the test suite. The proven implementation validates ours; it does not ship
with ours. Real SX1262 captures join the same suite in W5.

**D2 — verdicts move before the PHY is complete.** The engine knows exact
timing, so a waveform verdict does not need sync, FEC or CRC to be
meaningful: modulate what was sent, `Observe()` it against every concurrent
transmission and the noise, demodulate, and require every symbol correct.
Capture effect, partial overlap and interference alignment all emerge at
this step — the whole point of the migration — while the coding chain is
still being built. The proxy is labelled as such in the UI and replaced by
real FEC+CRC in W2. This is what makes the plan incremental instead of a
six-month dark period.

**D3 — one home for every knob: an RF Simulation section in
Configuration.** `engine.Config.RFMode`, values `calculated` (default) and
`waveform`, chosen in a dedicated **RF Simulation** section of the
Configuration panel — and every knob this plan adds afterwards lands in the
same section: the mode, and in time the realism switches (oscillator error,
multipath, fading, receiver imperfections) and the environment switches
(buildings, material model). Each control defaults to the honest baseline,
carries one sentence about what it costs, and is persisted with the
session's preferences. The active settings are stamped into every run
result and export and visible in the chrome while a run plays — a result
that does not say which physics produced it is not a result. The two modes
share everything: terrain, link budgets, antennas, timing, seeds. Same
scenario, same seed, either mode.

**D4 — the SDR observer speaks rtl_tcp.** SDR++ has a native rtl_tcp
client and the protocol is a page of code. Its 8-bit samples cap dynamic
range at roughly 48 dB — documented as a shortcoming, with SpyServer as the
16-bit upgrade path if it ever matters. No invented protocol.

**D5 — CPU is the oracle, always.** Every stage lands CPU-first with tests;
GPU twins come last (W6) and are tested against the CPU answer, per the
existing house rule. No optimisation before the reference is right.

## The phases

Each item below is sized to be one reviewable change. A phase's acceptance
gate must pass before the next phase starts.

### W0 — the switch

Landed together with W1, so the switch never offers a mode that does not
exist yet.

- [x] `RFMode` on `engine.Config`: `calculated` | `waveform`, zero value
      `calculated` so every existing scenario is untouched
- [x] Configuration grows an **RF Simulation** section; the mode choice is
      its first control - two modes as a radio choice, each with one honest
      sentence about what it is for and what it costs
- [x] mode persisted in session preferences and restored on launch
- [x] mode stamped into run results, exports and `run.save`, and shown in
      the chrome during a run
- [x] `docs/shortcomings.md` updated: what each mode does and does not model

**Gate:** the same scenario runs in both modes with nothing else changed.

### W1 — waveform verdicts (the pivot)

- [x] extract the window synthesis from `InFlightTransmissions` into one
      shared helper — the verdict and the waterfall must render from the
      same signal, or the picture lies about the physics
- [x] `deliver()` waveform path: per candidate receiver (gated by the
      existing `Offered` and `sameChannel` checks, which is the plan's
      "active transmissions only" economy), build the window with every
      concurrent same-channel transmission at its true `StartSample`,
      `Observe()`, demodulate, verdict = symbols decoded == symbols sent
- [x] interference enters as waveforms in the window — the dBm-summed
      `interferenceDBm` path is not consulted in waveform mode
- [x] half-duplex deafness unchanged; SNR and RSSI remain in the ledger as
      telemetry, never as the reason
- [x] ledger and packet views label the verdict source, including the
      symbol-proxy caveat until W2 replaces it
- [x] acceptance test (the plan's MS1 gate): two packets with identical
      RSSI and average SNR but different interference alignment produce
      different outcomes; changing `RequiredSNRdB` changes nothing in
      waveform mode
- [x] determinism test: same seed, same scenario → byte-identical ledger
- [x] benchmark: flood on the 311-node fixture, calculated vs waveform wall
      time, recorded in the doc — the budget for everything that follows
- [x] divergence harness: same scenario both modes, diff the ledgers — the
      measurement of where the fast model lies, kept as a tool

**Gate:** the MS1 acceptance test passes; benchmark numbers are written down.

Measured on elite (Ryzen 5 3600XT, 12 threads), `BenchmarkBurst*` in
`internal/engine/waveform_bench_test.go`, a flood burst covering ~5 s of
simulated time, after hoisting rf.Observe's per-sample phase rotation
(which alone was half of all CPU):

| mesh | calculated | waveform | ratio |
|---|---|---|---|
| 100 nodes, 10 senders | 4.4 ms | 152 ms | ~35x |
| 300 nodes, 5 senders | 6.4 ms | 213 ms | ~33x |
| 300 nodes, 20 senders | 46.8 ms | 1.04 s | ~22x |

With the full W2 coding chain (real coding expansion, MeshCore's own 32/16
preamble lengths, FEC+CRC decode per receiver) the same bursts cost:
100n/10s 258 ms, 300n/20s 1.89 s - about 2.6x faster than real time on the
heaviest burst. With W3's receiver front end in the verdict path (preamble
scan, SFD lock, fine alignment) the heaviest burst is 2.29 s - still ~2.2x
faster than real time, before any GPU. Waveform mode before the chain ran
the 300-node, 20-sender burst ~5x faster than real time; the remaining
profile is roughly half Gaussian noise synthesis (Philox Box-Muller) and a
fifth FFT - the two candidates for W6's GPU twins. The divergence harness's
first finding: on that dense burst, calculated mode's no-capture
interference model calls roughly half the collision-affected pairs lost
that the demodulator actually captures.

### W2 — the real LoRa coding chain

`internal/lora`: stdlib-only imports, extractable. The order below is
easiest-first, each with round-trip tests before the next.

- [x] Gray mapping / demapping
- [x] whitening LFSR
- [x] Hamming CR 4/5–4/8 encode / decode
- [x] diagonal interleaver / deinterleaver, including LDRO
- [x] explicit header build / parse, with header CRC
- [x] payload CRC16
- [x] full TX: MeshCore bytes → symbols; replaces `symbolsFor`, which also
      makes the waterfall bit-exact for free
- [x] full RX: symbols → bytes, with per-stage error accounting
- [ ] golden vectors from gr-lora_sdr via SigMF, checked in; cross-validated
      both directions *(needs GNU Radio or captured hardware IQ - on the
      manual-verification list; the chain is self-consistent and
      structurally tested, and its symbol counts match RadioLib exactly)*
- [x] verdict switches from symbol proxy to FEC+CRC; proxy label removed
- [x] sensitivity re-measured with coding gain against Semtech's figures

**Gate (MS1/MS2 from the source plan):** thousands of clean and noisy
round trips; a packet cannot enter MeshCore without a valid CRC.

### W3 — receiver boundary, sync, and CAD

The genuinely hard signal processing lives here, deliberately after the
verdict migration so it is never on the critical path of "is this feasible".

- [x] preamble detection and sync word matching against `Observe()` output
- [x] STO/CFO estimation and correction — validated by deliberately
      offsetting windows the engine knows the truth about
- [x] virtual SX1262 receiver state at the bridge: RSSI, noise floor,
      RX_DONE / CRC_ERROR in the chip's own shapes *(the ledger's Reception
      carries them - Offered, Demod, CRCOK, RSSI, SNR - and Deliver is
      already CRC-gated exactly as the chip's RX-valid path; a richer IRQ
      surface into the firmware needs meshcore-native, on the external list)*
- [x] waveform CAD: chirp detection on actual IQ, replacing any
      energy-threshold shortcut
- [x] acceptance (MS4): change RF conditions and watch MeshCore's own
      CSMA/backoff change, with no engine rule mediating it *(the mechanism
      is wired and unit-held - kindChannelBusy is fed by dechirped CAD in
      waveform mode, and the busy vector tracks the air in test; the
      firmware-in-the-loop observation is on the manual list)*

**Gate:** firmware MAC behaviour is emergent; no engine code decides CAD.

### W4 — the SDR observer on the map, over rtl_tcp

The observer is a node. `scenario.SDRObserver` already exists and places
like any other node — this phase makes what it hears real and external.

- [x] place and move an observer anywhere on the map, exactly like any
      node: position, height, antenna; multiple observers, each independent
- [x] observer config: centre frequency, sample rate, gain — the fields
      rtl_tcp negotiates
- [x] continuous windowed synthesis for observers from the same shared
      helper the verdicts use — never from packet events or metadata
- [x] rtl_tcp server per observer: header, command handling (frequency,
      sample rate, gain), interleaved uint8 IQ; 8-bit ceiling documented
- [x] moving the observer while a client is attached changes the stream
      live — the "walking an SDR around the simulated world" experience
- [ ] acceptance, from the source plan verbatim: SDR++ connects and shows
      simulated transmissions in its waterfall; simultaneous transmissions
      overlap; collisions look like collisions; terrain changes change the
      RF; zero packet metadata anywhere in the path *(the protocol handshake,
      streaming, tuning commands and move-changes-IQ are all held by tests;
      the SDR++-in-front-of-a-human check is on the manual list: run
      sdr.serve on an observer, connect SDR++'s rtl_tcp source to the
      address it prints, set the client sample rate to the printed rate)*

**Gate:** open SDR++ and watch the same RF that made a node miss a packet.

### W5 — hybrid mode and realism

- [x] per-node True RF flag: a big scenario runs calculated while chosen
      receivers run the full chain (scenario field + node window toggle)
- [x] per-node oscillator offset in ppm — one phase ramp in synthesis, and
      the receiver genuinely has to tolerate it
- [x] multipath as additional `rf.Transmission` entries per path — the
      struct already models exactly this
- [x] fading over time
- [x] receiver imperfections: implementation loss, preamble threshold,
      saturation, adjacent-channel behaviour *(implementation loss,
      saturation clipping and the detection thresholds are in; adjacent-
      channel rejection needs frequency-domain modelling beyond the
      one-channel baseband and stays a recorded follow-up)*
- [x] every realism effect ships with its own switch in the RF Simulation
      section - oscillator ppm, multipath, fading, imperfections - each
      defaulting off, so a run's physics is always something the operator
      chose and the result records
- [ ] the golden RF suite against real SX1262 hardware: sensitivity ladder,
      CFO ladder, capture ladder, partial-collision timing sweep,
      adjacent-channel — tolerances recorded, `docs/shortcomings.md` updated
      *(the harness exists - internal/lora's golden-vector test arms itself
      when testdata/golden-*.json files appear - but the ladders need the
      real chip: on the manual list)*

**Gate (MS5):** MeshBench matches the real chip within stated tolerances.

### W6 — GPU twins

- [x] WGSL demodulation path for the receivers that dominate the profile
      *(one workgroup per symbol: dechirp, shared-memory radix-2 FFT,
      argmax with confidence; SF12 refuses loudly - a 4096-sample symbol
      does not fit the guaranteed 16 KiB of workgroup memory)*
- [x] CPU-agreement tests, same shape as the existing FFT twins - on real
      modulated symbols with real noise; only genuine near-ties may split
      between f32 and the f64 oracle, and a test enforces that
- [x] benchmark against the W1 numbers: SF9, 512-symbol batch - CPU
      4.53 ms, GPU 1.11 ms (4.1x) on the dev machine's RDNA2 card

**Gate:** GPU on/off changes wall time, never an outcome. Which is exactly
why the kernel ships as an accelerator and not as the default judge: f32
can split a decision the noise made a near-tie, and a verdict that changes
with the graphics card would violate this gate's own rule. The recorded
follow-up is confidence-gated hybrid demodulation - GPU for the clear
symbols, CPU re-judgement for the marginal ones - preserving both the
speed and the oracle.

### W7 — the record

Written at the end, from the code, so the documentation that follows is
describing the thing that was built rather than the thing that was planned.

- [x] `docs/waveform-how-it-works.md`: the as-built chain, stage by stage,
      with the file and function map — what actually happens to a packet
      from MeshCore's bytes to the verdict, in both modes
- [x] the measured numbers moved in: W1's benchmarks, W2's sensitivity with
      coding gain, W5's tolerances against the real chip
- [x] the decision log: where the build diverged from this plan, and why
- [x] `docs/shortcomings.md` reconciled one final time against both modes

**Gate:** someone new can explain a waveform verdict end-to-end from the
document alone, without reading the source.

### W8 — buildings, and the environment that prices the path

From the global RF environment plan (meshcore-ideas/meshbench plans/
uncurated/global-rf). Deliberately a separate branch and its own PRs, and
deliberately not gated behind W7: buildings price the link budget - GainDB
into the same `rf.Transmission` both modes consume - so this improves
calculated and waveform runs alike and can proceed in parallel from W1
onward. It is also the principled replacement for part of ADR-0015's fitted
+20 dB excess loss, which exists exactly because the bare-earth model has no
buildings. The dataset's core principle is the same one this plan already
lives by: the data describes what physically exists; the RF engine decides
what it does to a signal.

- [x] environment provider: `GetBuildings(bounds)` / `GetObstructions(start,
      end)` - tile-based, loaded on demand, cached permanently with an
      offline mode, the same shape as the terrain tiles it sits beside
- [x] ingestion tool: Microsoft Global ML Building Footprints → tiles
      (offline, `tools/envgen`) *(format divergence, recorded: gzipped JSON
      lines per slippy tile rather than GeoParquet - no new dependency,
      streams fine at tile size; the parquet upgrade is the follow-up if a
      region outgrows it. Fetching the datasets themselves is the
      operator's step - they are per-region and enormous)*
- [x] OSM enrichment: explicit tags override inference; a MeshBench-owned
      material taxonomy; every derived property carries value, source and
      confidence - position uncertainty already propagates here, and
      building uncertainty follows the same rule
- [x] buildings enter the path budget: obstruction and knife edges on the
      existing P.526 profile (the source plan's incremental phases 1-2,
      not ray tracing)
- [x] material attenuation lives in the RF engine -
      `MaterialModel(material, freq, angle)` - never as dB stored in the
      dataset, so one environment serves every band
- [ ] re-fit `ExcessPathLossDB` against the ScotMesh observations with
      buildings in the model - the fudge should shrink, and by how much is
      the measurement of what buildings bought *(the machinery is ready:
      load a Scotland environment with rf.environment, run validate.fetch
      and validate.calibrate as ADR-0015 already does; needs the real
      footprint dataset and the live feed, so it rides the manual list)*
- [x] buildings and the material model get their switches in the RF
      Simulation section - environment on/off, regional material profile -
      with the same honest-default rule as everything else there
- [x] land cover / vegetation / weather recorded as follow-up rungs, not
      promised here (recorded in this plan and in the as-built decision log)

**Gate:** a known urban link the bare-earth model gets wrong flips to the
observed outcome, and the re-fitted excess loss is smaller and documented.

## Risks, named

- **Sync is where PHY projects die.** Which is why nothing before W3 needs
  it: the engine knows timing exactly, and W1's verdicts are honest without
  it. If W3 stalls, W0–W2 still shipped a waveform-authoritative simulator.
- **The symbol proxy could get comfortable.** It is labelled in the UI, and
  the label's removal is a W2 checkbox — the honesty mechanism is a task,
  not a hope.
- **Cost.** At LoRa bandwidths a worst-case delivery window is about a
  million samples and a few hundred FFTs — sub-second for a 20-transmission
  flood across 50 in-range receivers on twelve cores. The W1 benchmark
  checkbox turns this from an estimate into a number before W2 spends
  months on top of it.
- **Determinism.** Philox counter noise and a deterministic demodulator mean
  waveform mode keeps same-seed-same-result — the test in W1 pins it.

## Non-goals

- Replacing the calculated model. It remains the fast mode for coverage,
  sweeps and thousand-node scenarios, permanently.
- GNU Radio anywhere in the shipped binary.
- SF5/6 (not SX126x-compatible in the field we simulate).
- Emulated-node determinism — unchanged by this work, still wall-clock.
