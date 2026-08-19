# Shortcomings

What MeshBench does **not** model, does not model well, or has not yet been
checked against reality. Everything here is known and deliberate; none of it is
a bug report.

The reason this document exists is in `CLAUDE.md`: *"The simulator is kinder
than the air. Say so in the UI — never let a user assume otherwise."* A
simulator that does not publish its own error budget is a confident liar.

Ordered by how likely each one is to change a decision someone makes on the
strength of a simulation.

---

## 0. Two RF modes, two error budgets

Since the waveform work (docs/waveform-source-of-truth.md) MeshBench has two
reception models, chosen in Configuration under **RF Simulation** and stamped
into every result:

**Calculated** (the default): reception is a link-budget SNR against the
demodulator floor, with concurrent transmissions summed into the noise in
dBm. Fast, and the mode every large scenario should use. Two known biases:
any overlap counts as full-length interference however briefly it clipped
the packet (pessimistic under partial collisions), and there is no capture -
a strong signal is destroyed by a weak overlap the real chip would ignore
(measured against waveform mode: on a dense 60-node, 6-simultaneous-sender
burst, roughly half the collision-affected pairs decode under the waveform
that calculated calls lost).

**Waveform**: the channel synthesises IQ, every concurrent transmission
lands in the receiver's window at its true offset, and the verdict is the
full receive chain - demodulation, Gray, the diagonal deinterleaver, Hamming
FEC, dewhitening, the explicit header and the payload CRC (internal/lora).
What reaches MeshCore is the decoded bytes. Capture, partial overlap and
interference alignment are emergent; a grazing collision that FEC can repair
is repaired, and the ledger says how many codewords it cost. One remaining
caveat: the receiver front end models ideal sync (plan W3 grants exact
timing inside the engine). The bit-level conventions - sync word, Hamming
parity matrix, whitening, header checksum, payload CRC - have been verified
against a real SX1262 over the air (tools/goldencap, 2026-08-18): the
sync-word chirps, the chip's four-XOR parity equations and the CRC's
last-two-bytes quirk were all solved from captured frames and corrected in
internal/lora, and two captured frames are checked in as golden vectors that
the test suite holds the encoder to. Packet sensitivity brackets Semtech's
published floors in test.

---

## 1. Physics that is absent

### 1.1 No multipath — the biggest single gap

Every link is one coherent path. Real 868 MHz propagation over UK terrain
arrives as many paths with different delays, and their vector sum fades in and
out as anything moves.

**Consequence.** The simulator cannot produce the failure that dominates real
marginal links: a path with a perfectly good median signal that drops out for
hundreds of milliseconds at a time. A link MeshBench calls reliable may be
unusable in practice, and there is no warning sign in the output — the SNR just
looks fine.

**Direction of error: optimistic.** This is the one to say out loud to anyone
planning from a result.

Fixing it properly means a tapped-delay-line channel with a fading process per
tap, which the sample-accurate architecture supports but nothing currently
builds.

### 1.2 No frequency error, no oscillator drift

We demodulate against a perfect reference chirp. A real SX1262 runs on a crystal
with a few ppm of error, which at 869 MHz is several kHz — a non-trivial
fraction of a 125 kHz channel — plus drift with temperature over a day.

**Consequence.** Carrier frequency offset degrades real sensitivity, and it is
part of why two overlapping transmissions behave the way they do. Our capture
effect emerges from amplitude alone, so near-far cases are cleaner than reality.

**Direction of error: optimistic.**

### 1.3 Bare-earth terrain only

Terrain comes from a DEM. No buildings, no trees, no clutter class, no body
loss from the person holding the node.

**Consequence.** Urban and wooded paths are substantially better in simulation
than in the field — for woodland at 868 MHz the difference is tens of dB, not a
correction factor. A rural hilltop-to-hilltop path is the case this models well;
a handheld in a town is the case it models worst.

**Direction of error: strongly optimistic wherever anything is in the way that
is not a hill.**

Loading a building environment (Configuration's Buildings card - a runtime
pull from OpenStreetMap or Microsoft's ML footprints, or tiles prepared with
`tools/envgen`) closes part of this: each crossed building becomes a rooftop
knife edge plus one wall of material loss. It is still not clutter, trees or
body loss, and a pulled database inherits that database's gaps - ML
footprints carry no materials, so those buildings fall back to a regional
default with the low confidence that implies. The merged pull narrows that
where OSM has surveyed the building - explicit type and material override
the inference - but only there; the unsurveyed majority keeps the default.

### 1.4 Diffraction is knife-edge, and single-mechanism

Bullington construction with the ITU-R P.526 correction — the method P.1812 and
P.452 use for this problem. There is no rounded-obstacle correction and **no
ground reflection**.

This replaced a Deygout decomposition, which was wrong here in a way worth
recording. Deygout recurses into the sub-paths either side of the principal
edge, which is sound on a profile with distinct peaks and unsound on a smooth
one: a spherical earth flattens into a parabola, and recursing on a parabola
charges the same curvature once per level. Measured on flat ground at 869 MHz,
loss stepped from 0 dB at 68 km to 34.7 dB at 69 km — a hard edge across a
coverage raster that no terrain put there. There is now a test asserting
continuity in distance, because a discontinuity here is invisible in any single
path calculation and only shows up as an artefact on a map.

**Consequence.** A real hill is not a knife edge; knife-edge loss is the
optimistic bound for a rounded one. Bullington reduces a whole profile to one
equivalent edge and the correction term restores what that under-reads, but it
is a correction, not a multi-edge calculation. Separately, a two-ray
ground-reflection model matters over water, estuaries and flat farmland, and its
absence can be wrong in *either* direction — constructive interference can make
a real path better than we predict.

### 1.5 Noise is thermal AWGN and nothing else

`N = −174 + 10·log₁₀(BW) + NF`. No impulsive noise, no elevated man-made noise
floor, no adjacent-channel interference, no intermodulation.

**Consequence.** A node beside a switch-mode supply, a solar inverter or an EV
charger sees a noise floor well above thermal. MSIM-20 adds deliberate external
emitters; ambient noise is not modelled at all.

**Direction of error: optimistic.**

### 1.6 The SDR observer sees one channel, not a band

An observer captures the modelled channel and nothing else, so its
`SampleRateHz` is the simulated bandwidth. A real SDR at 2 Msps sees several
hundred kHz of neighbours; ours sees empty space, because the engine never
generated anything there. Adjacent channels are **absent, not quiet** — the
waterfall will never show a neighbouring network, an ISM-band doorbell or a
harmonic, because none exists to show.

The receive filter in `Listen` is also a brick wall in the frequency domain. No
real receiver has one, so adjacent-signal rejection is better than any hardware
achieves.

The stream is also resampled to whatever rate the client asks for - no SDR
client offers the observer's native rate in its menu - by windowed-sinc
interpolation, which keeps a transmission exactly as wide as its bandwidth
(images sit ~60 dB down, below the 8-bit format's own floor). The floor
across the rest of the span is the server's own addition at the receiver's
noise density - a real dongle's front end fills its span, and a silent
shoulder looked synthetic - but only the in-band portion is what the
verdicts hear; signals from adjacent channels still do not exist. A level
control anchored to that floor keeps strong bursts from clipping into
broadband splatter; a burst more than ~33 dB over the floor briefly
presses the floor down instead, which is the trade a real front end makes
with its gain.

### 1.7 Antenna patterns are analytic, not measured

Isotropic, dipole, collinear and Yagi from closed-form expressions. No measured
patterns (`.msi`, NEC), no mast shadowing, no mutual coupling, no radome or
feeder loss beyond a scalar, no ground-plane interaction.

**Consequence.** A real collinear on a real mast has nulls and a distorted
pattern that our idealised one does not. Nothing models the mast blocking the
back of its own antenna.

---

## 2. Measured error we already know about

### 2.1 The demodulator is up to 1.6 dB better than a real SX1262

Measured against Semtech's published sensitivity, at 1% packet error over
40-symbol frames:

| SF | measured | Semtech | delta |
|---|---|---|---|
| SF7 | −7.45 | −7.50 | +0.05 |
| SF8 | −9.76 | −10.00 | +0.24 |
| SF9 | −13.07 | −12.50 | **−0.57** |
| SF10 | −15.35 | −15.00 | **−0.35** |
| SF11 | −18.62 | −17.50 | **−1.12** |
| SF12 | −21.56 | −20.00 | **−1.56** |

A negative delta means **we decode packets a real chip would not**. Our
demodulator is an ideal coherent dechirp-and-FFT; a real chip has
implementation loss that grows with spreading factor.

**Consequence.** Long-range SF11/SF12 links — exactly the ones people care
about — are up to 1.6 dB more optimistic than the rest. That is real distance on
a marginal path.

This is not calibrated out, deliberately: adding a fudge factor would hide the
fact that the chain is idealised. It should be corrected with a measured
implementation-loss curve (MSIM-28/29), not a constant.

### 2.2 No CRC, no interleaving, no Gray coding, no Hamming FEC

The modem chain is chirp modulation, channel, dechirp, FFT, peak pick. Real LoRa
adds whitening, Gray mapping, Hamming FEC at the configured coding rate,
interleaving and a CRC.

**Consequence.** Two things. First, our symbol errors translate to packet errors
differently from a real chip — FEC repairs scattered errors and interleaving
spreads bursts, so a real receiver survives some noise realisations we fail and
fails some we survive. Second, we have no CRC, so a "received" packet is one
whose symbols we got right, not one that passed a real integrity check.

Airtime *is* computed with the coding rate (§4.1), so timing is right even
though the error behaviour is not.

---

## 3. Firmware fidelity

### 3.1 The shipped image boots, but does not yet reach its radio

**This entry previously said the published `.uf2` could not be run at all,
because the SoftDevice needed Nordic behaviour Renode does not model. That was
wrong, and measuring it disproved it.**

The real `RAK_4631_repeater-v1.17.0` image boots through the MBR and the s140
SoftDevice and reaches its own application main loop under Renode. Three things
were in the way, none of them the SoftDevice:

1. **UICR is absent from Renode's nRF52840 platform.** The MBR reads
   `UICR.NRFFW[0]` to find a bootloader; Renode returns 0 for an unmapped
   region, and 0 is a valid flash address, so the MBR spends eternity handing
   over to a bootloader that is not there.
2. **Renode's flash is zero-filled; real erased flash is `0xFF`.** The MBR
   checks for exactly that pattern to decide whether an MBR parameter page
   exists. Loading the SoftDevice and the application as separate images leaves
   zeros between them, and the boot chain quietly takes the wrong branch. The
   fix is to build one pre-filled image — `tools/renode/merge-nrf52.py`.
3. **MWU is unimplemented.** The SoftDevice uses the Memory Watch Unit for its
   critical sections and hits it constantly. Renode answers from the SVD, which
   is enough not to fault but is not the peripheral behaving.

What still does not happen is SPI traffic to the SX1262, so the shipped image
has not been observed driving its radio. That is now a peripheral-coverage
question — the right SPI instance for the board, and MWU — rather than a
proprietary-blob question.

**Consequence, as it actually stands.** Our own SoftDevice-free build is what
runs end to end today, so anything depending on the real scheduler, or on the
BLE stack stealing time from the radio ISR, is still not reproduced. But the
path to running stock images is open and short, which matters: downloading
prebuilt firmware is meant to be the *default* experience (ADR-0020), not a
stretch goal.

### 3.2 The SX1262 model is functional, not cycle-accurate

The Renode peripheral answers `GetStatus`, accepts configuration and carries
frames. It does **not** model BUSY assertion timing, DIO1 interrupt latency,
TCXO startup delay, image calibration time, or FIFO wrap semantics.

**Consequence.** Firmware bugs in radio state-machine timing — the class that
bites hardest on real boards — will not reproduce. This also means the
emulated/native cross-check (MSIM-40) currently compares two things that agree
by construction on timing, which weakens it.

### 3.3 Native nodes cannot catch anything architecture-dependent

A native node is x86-64 or arm64, 64-bit, with a different `int` width in
places, different alignment and a different FPU.

**Concretely**: the bug that blocked the ARM port for hours was the Cortex-M4F
powering up with its FPU disabled. A native node cannot express that bug. This
is the whole reason both backends exist, and it is why native being ~3300×
faster is not a reason to delete the emulated one.

### 3.4 Two board families run, and the boards with front ends are not among them

nRF52840 under Renode and ESP32 under our QEMU fork, which now does model the
SX1262 — `hw/ssi/sx1262` forwards SPI to the same `VirtualSX1262` a native node
clocks, so an emulated node and a native one are the same radio. The wall this
section used to describe is gone.

What is still missing is the boards where a front-end module decides how much
power reaches the antenna. The enable line crosses into the model as a GPIO
(`radio-fem`), and `Generic_E22_sx1262` can exercise it under QEMU today, but
**the Heltec T096 cannot be run at all**: it is nRF52840, so it needs Renode
rather than QEMU, and of the boards shipping `.uf2` images only T-Beam Supreme S3
is ESP32-S3.

That matters more than a missing board usually would, because the T096 is the
board of the transmit fault MeshCore 1.17.1 fixed — firmware that never raised
the enable line, so the PA's output went through the module's isolation instead
of its gain. **A study of that class of fault cannot presently be run here.** No
fixture shipping today contains a node with a front end at all: every node in
the Scotland and Fife fixtures is a RAK4631, which has none, so the front-end
path is unexercised rather than merely untested.

### 3.5 Forwarding policy is ours, not the repeater application's

The native node links MeshCore's *library* — `Mesh`, `Dispatcher`, `Packet`,
`Identity` — which is where routing, retransmit timing, duty-cycle accounting
and CSMA live. It does not link the repeater *application*.

That matters for exactly one method. `MyMesh::allowPacketForward` in
`examples/simple_repeater` enforces region transport codes, a configurable
loop-detect table and several hop caps, and it needs Arduino preferences, an
RTC and a filesystem to do it. Our node implements that method's essential
half: flood packets forward until the hop cap.

**Consequence.** Relay *timing* is MeshCore's, and so is every decision about
when the channel is clear. Relay *eligibility* is ours, and a network whose
behaviour depends on region scoping or on the stricter loop-detect settings
will behave differently here. Without any override the base class refuses to
forward at all and a flood stops dead at the origin's neighbours — which looks
exactly like a network with no repeaters configured, so this is not something
that can simply be left out.

### 3.6 BLE is ours, not the firmware's

The Bluetooth companion is a host-side BlueZ GATT server presenting the Nordic
UART Service. It is genuinely connectable from a real phone — but it is *our*
implementation of the companion protocol, not MeshCore's BLE code being
exercised. Removing the SoftDevice removed the firmware's Bluetooth stack along
with it.

---

## 4. Things that are right but narrowly

### 4.1 Airtime is an estimate the firmware makes, not a simulator shortcut

Worth being explicit, because it looks like a shortcut in a simulator that
models actual waveforms. How long a transmission occupies the channel is decided
by the samples the engine generates — nothing consults a formula for that, and
the node is *told* when its waveform ended rather than timing itself.

`getEstAirtimeFor()` is separate: a pure virtual MeshCore calls **before**
transmitting, to size its CSMA backoff and spend its duty-cycle budget. Real
hardware answers it with RadioLib's `getTimeOnAir()`, so we answer it with the
same calculation, checked against the Go one (MSIM-42). An estimate the firmware
makes about itself, faithfully reproduced.

The rest of the timing chain is weaker. CAD timing,
the AGC reset interval and the noise-floor calibration interval are still the
firmware's own numbers running against a channel that does not model what those
mechanisms are for.

### 4.2 `isInRecvMode()` is what the node can observe, not what the channel knows

The native node reports "listening whenever not transmitting". A real node is
also deaf during calibration, during mode transitions, and briefly after an
interrupt. Reachability is therefore slightly overstated.

### 4.3 Position uncertainty is modelled at import, not propagated everywhere

CoreScope positions carry an uncertainty. It is stored and displayed, but it
does not yet widen every downstream result the way HAM-34 does in hamreach.

---

## 5. Scale, and what has not been measured

| Workload | Measured | Where |
|---|---|---|
| Demodulation, one core | ~160 real-time receivers, flat across SF | MSIM-3, elite (12 cores) |
| 100-node mesh, CPU only | ~19× real time | MSIM-3 |
| Raster, one core | 50 M profile-steps/s | MSIM-3 |
| Native node, lockstep | 10 s simulated in 3.0 ms (**3323×**) | MSIM-41, elite |
| Emulated node | roughly 1× real time | MSIM-13 |

**What has not been measured, and matters:**

- **Lockstep at scale.** One native node at 1 ms ticks is 1000 socket round
  trips per simulated second. A 100-node scenario is 100,000 per second across
  100 processes, and the 3323× figure will not survive that. The tick
  granularity may have to be coarsened, or nodes batched — neither is designed.
- **GPU end to end.** The dechirp kernel is validated against its CPU twin, but
  no full scenario has run on the GPU path.
- **Memory at 100+ nodes.** Never run.

The honest headline: **a 20-node native scenario is comfortable today; a
100-node one is a plan, not a measurement.**

---

## 6. Not yet built

Specified and ticketed, with nothing running behind them:

| | |
|---|---|
| Antenna filters against interference | MSIM-21 — the emitter model exists (MSIM-20); per-emitter emission masks and receive filters do not |
| Instrumented firmware builds | MSIM-26 |
| Calibrating excess loss from residuals | MSIM-29 — needs MSIM-28 to have data first |
| Shadow mode: live model-versus-reality | MSIM-33 |
| Our own MeshCore builds in a separate MIT repo | MSIM-39 — the repo has not been created |

Built since this list was first written: the desktop application (MSIM-10), the
terrain cut-through view (MSIM-11), coverage rasters and planning (MSIM-34/35),
terrain and boundary download (MSIM-38/37), the firmware catalogue (MSIM-13),
the provider interface with CoreScope, Beacon and MQTT (MSIM-30/31/32), battery
and solar (MSIM-19), external interference (MSIM-20), the MCP server (MSIM-36),
the validation harness (MSIM-28), replay (MSIM-27), board profiles (MSIM-18),
the multi-node console (MSIM-25), the emulated/native cross-check (MSIM-40) and
a command line covering all of it (MSIM-23).

### What the application does not do yet

There is a window, and it answers the question the whole project is about — pick
two nodes, get both margins and a terrain cut-through explaining the verdict.
What it does not have is a **map**. Nodes are a list, not points on terrain, and
the coverage rasters that `meshcoresim coverage` writes as a PNG are not drawn in
the application. That is the largest remaining gap in the product rather than in
the physics.

---

## 7. Unvalidated, which is different from unbuilt

**The RF model has never been checked against a real observation.** Every
component is tested against a published reference — Semtech's sensitivity
figures, ITU-R P.526, RadioLib's airtime — and the GPU against its CPU twin. No
part of it has been compared with a packet that actually crossed real air.

The *harness* for that comparison now exists (`internal/validate`, MSIM-28): it
takes observed receptions and reports bias, spread and percentiles, counts every
exclusion, and refuses to treat a silent receiver as a negative observation. What
it has never had is data. Running it against a real CoreScope or Beacon export is
the single highest-value unfinished item in the project.

Until it does, every number here is "correct according to the textbook", which is
not the same as "true on the hill above Aberfeldy".

---

## 8. Legal and licensing

- **No licence chosen** (MSIM-15). No `LICENSE` file, deliberately.
- **No `THIRD_PARTY_NOTICES.md`.** MeshCore (MIT), rweather/arduinolibs Crypto
  (MIT) and MeshCore's vendored ed25519 all require attribution on
  distribution. This must exist before anything ships.
- **Our MeshCore builds go in a separate repo** under MeshCore's own MIT licence
  (MSIM-39, ADR-0020), because they link MeshCore. Not yet created.
- **Ofcom mast data terms unresolved** (MSIM-20), as are the terms for whichever
  boundary source MSIM-37 uses.

---

## What this is good at

For balance, because the list above is long and the tool is not weak:

- Rural, line-of-sight-ish, hilltop-to-hilltop VHF/UHF paths over real terrain,
  with honest diffraction — the case it is built for.
- **Why** a link failed, not just that it did: the terrain cut-through, the
  Fresnel intrusion and the link budget are all inspectable.
- Comparative work — *this mast versus that one*, *five metres higher*, *SF10
  versus SF12* — where systematic optimism largely cancels between the two
  answers being compared. This is most of what a planner actually does.
- Real firmware behaviour: routing, flood suppression, duty-cycle policing and
  CSMA are MeshCore's own code, not a reimplementation of it.

The systematic biases are almost all in one direction, which makes them usable:
**treat a MeshBench result as a best case.** If it says a link will not work,
believe it. If it says a link will work marginally, go and measure.
