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

**D3 — the switch lives in the engine's config and the Configuration
panel.** `engine.Config.RFMode`, values `calculated` (default) and
`waveform`. Chosen in the Configuration panel, persisted with the session's
preferences, stamped into every run result and export, and visible in the
chrome while a run is playing — a result that does not say which physics
produced it is not a result. The two modes share everything: terrain, link
budgets, antennas, timing, seeds. Same scenario, same seed, either mode.

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

- [ ] `RFMode` on `engine.Config`: `calculated` | `waveform`, zero value
      `calculated` so every existing scenario is untouched
- [ ] Configuration panel card: the two modes as a radio choice, each with
      one honest sentence about what it is for and what it costs
- [ ] mode persisted in session preferences and restored on launch
- [ ] mode stamped into run results, exports and `run.save`, and shown in
      the chrome during a run
- [ ] `docs/shortcomings.md` updated: what each mode does and does not model

**Gate:** the same scenario runs in both modes with nothing else changed.

### W1 — waveform verdicts (the pivot)

- [ ] extract the window synthesis from `InFlightTransmissions` into one
      shared helper — the verdict and the waterfall must render from the
      same signal, or the picture lies about the physics
- [ ] `deliver()` waveform path: per candidate receiver (gated by the
      existing `Offered` and `sameChannel` checks, which is the plan's
      "active transmissions only" economy), build the window with every
      concurrent same-channel transmission at its true `StartSample`,
      `Observe()`, demodulate, verdict = symbols decoded == symbols sent
- [ ] interference enters as waveforms in the window — the dBm-summed
      `interferenceDBm` path is not consulted in waveform mode
- [ ] half-duplex deafness unchanged; SNR and RSSI remain in the ledger as
      telemetry, never as the reason
- [ ] ledger and packet views label the verdict source, including the
      symbol-proxy caveat until W2 replaces it
- [ ] acceptance test (the plan's MS1 gate): two packets with identical
      RSSI and average SNR but different interference alignment produce
      different outcomes; changing `RequiredSNRdB` changes nothing in
      waveform mode
- [ ] determinism test: same seed, same scenario → byte-identical ledger
- [ ] benchmark: flood on the 311-node fixture, calculated vs waveform wall
      time, recorded in the doc — the budget for everything that follows
- [ ] divergence harness: same scenario both modes, diff the ledgers — the
      measurement of where the fast model lies, kept as a tool

**Gate:** the MS1 acceptance test passes; benchmark numbers are written down.

### W2 — the real LoRa coding chain

`internal/lora`: stdlib-only imports, extractable. The order below is
easiest-first, each with round-trip tests before the next.

- [ ] Gray mapping / demapping
- [ ] whitening LFSR
- [ ] Hamming CR 4/5–4/8 encode / decode
- [ ] diagonal interleaver / deinterleaver, including LDRO
- [ ] explicit header build / parse, with header CRC
- [ ] payload CRC16
- [ ] full TX: MeshCore bytes → symbols; replaces `symbolsFor`, which also
      makes the waterfall bit-exact for free
- [ ] full RX: symbols → bytes, with per-stage error accounting
- [ ] golden vectors from gr-lora_sdr via SigMF, checked in; cross-validated
      both directions
- [ ] verdict switches from symbol proxy to FEC+CRC; proxy label removed
- [ ] sensitivity re-measured with coding gain against Semtech's figures

**Gate (MS1/MS2 from the source plan):** thousands of clean and noisy
round trips; a packet cannot enter MeshCore without a valid CRC.

### W3 — receiver boundary, sync, and CAD

The genuinely hard signal processing lives here, deliberately after the
verdict migration so it is never on the critical path of "is this feasible".

- [ ] preamble detection and sync word matching against `Observe()` output
- [ ] STO/CFO estimation and correction — validated by deliberately
      offsetting windows the engine knows the truth about
- [ ] virtual SX1262 receiver state at the bridge: RSSI, noise floor,
      RX_DONE / CRC_ERROR in the chip's own shapes
- [ ] waveform CAD: chirp detection on actual IQ, replacing any
      energy-threshold shortcut
- [ ] acceptance (MS4): change RF conditions and watch MeshCore's own
      CSMA/backoff change, with no engine rule mediating it

**Gate:** firmware MAC behaviour is emergent; no engine code decides CAD.

### W4 — the SDR observer on the map, over rtl_tcp

The observer is a node. `scenario.SDRObserver` already exists and places
like any other node — this phase makes what it hears real and external.

- [ ] place and move an observer anywhere on the map, exactly like any
      node: position, height, antenna; multiple observers, each independent
- [ ] observer config: centre frequency, sample rate, gain — the fields
      rtl_tcp negotiates
- [ ] continuous windowed synthesis for observers from the same shared
      helper the verdicts use — never from packet events or metadata
- [ ] rtl_tcp server per observer: header, command handling (frequency,
      sample rate, gain), interleaved uint8 IQ; 8-bit ceiling documented
- [ ] moving the observer while a client is attached changes the stream
      live — the "walking an SDR around the simulated world" experience
- [ ] acceptance, from the source plan verbatim: SDR++ connects and shows
      simulated transmissions in its waterfall; simultaneous transmissions
      overlap; collisions look like collisions; terrain changes change the
      RF; zero packet metadata anywhere in the path

**Gate:** open SDR++ and watch the same RF that made a node miss a packet.

### W5 — hybrid mode and realism

- [ ] per-node True RF flag: a big scenario runs calculated while chosen
      receivers run the full chain (scenario field + node window toggle)
- [ ] per-node oscillator offset in ppm — one phase ramp in synthesis, and
      the receiver genuinely has to tolerate it
- [ ] multipath as additional `rf.Transmission` entries per path — the
      struct already models exactly this
- [ ] fading over time
- [ ] receiver imperfections: implementation loss, preamble threshold,
      saturation, adjacent-channel behaviour
- [ ] the golden RF suite against real SX1262 hardware: sensitivity ladder,
      CFO ladder, capture ladder, partial-collision timing sweep,
      adjacent-channel — tolerances recorded, `docs/shortcomings.md` updated

**Gate (MS5):** MeshBench matches the real chip within stated tolerances.

### W6 — GPU twins

- [ ] WGSL demodulation path for the receivers that dominate the profile
- [ ] CPU-agreement tests, same shape as the existing FFT twins
- [ ] benchmark against the W1 numbers

**Gate:** GPU on/off changes wall time, never an outcome.

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
