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

MeshBench has two reception models, chosen in Configuration under **RF Simulation** and stamped
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

### 2.2 The coding chain exists in one RF mode and not the other

**This section used to say there was no CRC, no interleaving, no Gray coding and
no Hamming FEC anywhere.** That was true of the whole simulator once; it is now
true of only one of its two modes, and the difference decides which mode a
result should be trusted from.

**Waveform mode has the chain.** `internal/rf/lora` implements whitening, Gray
mapping, the diagonal interleaver, Hamming FEC at the configured coding rate,
the explicit header and the payload CRC, in both directions. Its bit-level
conventions were not taken from a paper: the sync-word chirps, the chip's
four-XOR parity equations and the CRC's last-two-bytes quirk were solved from
frames captured off a real SX1262 and are held in place by golden vectors in
`internal/rf/lora/testdata` that the test suite checks the encoder against. A
"received" packet in this mode is one that passed a real integrity check.

**Calculated mode does not have it, by design.** That path decides reception
from a link-budget SNR against the demodulator floor. It never forms a symbol,
so there is nothing for FEC to repair and no CRC to fail — a packet is received
if the margin says so.

**Consequence.** In calculated mode, symbol errors translate to packet errors
differently from a real chip: FEC repairs scattered errors and interleaving
spreads bursts, so a real receiver survives some noise realisations that mode
calls lost and fails some it calls received. That is the price of the speed, and
it is why every large scenario should run calculated and every claim about
marginal reception should be re-checked under waveform.

Airtime *is* computed with the coding rate in both modes (§4.1), so timing is
right either way.

### 2.3 The capture threshold is a hardware figure, not one we measured

Collisions are decided by how far a wanted signal leads its interferer. The
calculated path uses **6 dB**, the figure LoRa capture is usually quoted at for
same-spreading-factor collisions.

Our own receive chain does not need anything like that much. Swept across eight
symbol alignments, it recovers the stronger of two fully-overlapping frames from
**0.5 dB** ahead — because it is an ideal coherent demodulator with no
oscillator error, no AGC transient and no quantisation, so two clean tones in an
FFT are separated by whichever is larger.

**We deliberately use the hardware number rather than our own.** A simulator that
captured as easily as its idealised DSP would let packets survive collisions
that kill them on the bench. `TestCaptureThresholdMatchesTheDemodulator` guards
the relationship rather than the value: it fails if the receive chain ever needs
*more* lead than real hardware, which would make the fast path the optimistic
one.

**Direction of error: unknown, and that is the point.** 6 dB is defensible and
widely cited, but it is not measured from ScotMesh or from a bench here. It is
the least-grounded constant in the collision model, and the one to attack first
with real captures.

By contrast, the rules around it *are* grounded. The reported-SNR ceiling of
+15 dB is measured from 1,992 real ScotMesh receptions, which have a median of
+5.0 dB, a 90th percentile of +13.0 dB and nothing above the wall; reported
RSSI is bounded to the register that reports it (0 to −127.5 dBm), which the
same 2,000 packets span exactly. The repair rule — one destroyed symbol is
recoverable at CR 4/7 and 4/8 and fatal below them, two are fatal everywhere —
falls straight out of the diagonal interleaver and the Hamming layer in
`internal/rf/lora`, whose parity equations were solved from a real captured
frame rather than taken from a paper. And the demodulator lock's contest
window is paced by our own detector's commitment — `dsp.PreambleDetectSymbols`,
five stable dechirped windows — with measurement literature on real chips
putting the same commit at about four symbols; inside that window the dominant
signal takes the lock, which is capture effect at the moment it happens, and a
holder that falls silent early in a long MeshCore preamble frees the receiver
to acquire what is left.

---

## 3. Firmware fidelity

### 3.1 The shipped image boots and reaches its radio; three boards then go quiet

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

**That last paragraph used to say SPI traffic to the SX1262 still did not
happen, so no shipped image had been observed driving its radio. It does now.**
Five published nRF52840 images boot through the MBR and the s140 SoftDevice,
reach their own main loop, and put an advert on the air that another node hears
— measured board by board in the compatibility matrix in `README.md`. Two of
them relay somebody else's packet and are still answering after an idle.

**Consequence, as it actually stands.** Downloading prebuilt firmware is the
default experience it was meant to be (ADR-0020) rather than a stretch goal.
What is still not reproduced is anything depending on the BLE stack stealing
time from the radio ISR: the images that run are repeater builds, and nothing
here connects to one over Bluetooth.

Three of those five advert once and then report their channel busy for the rest
of the run. That is tracked separately and is not a SoftDevice problem — a board
on the same emulator with the same boot chain relays correctly.

### 3.2 The SX1262 model is functional, not cycle-accurate

The peripheral answers `GetStatus`, accepts configuration and carries frames.
The BUSY line now exists rather than being absent — RadioLib spins on it and
gets an answer, which is what let the QEMU boards get as far as they have — but
it is never asserted with real timing. Still unmodelled: BUSY assertion timing,
DIO1 interrupt latency, TCXO startup delay, image calibration time and FIFO wrap
semantics.

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

### 3.4 Both board families run; the front-end path is thin

nRF52840 under Renode and ESP32 under our QEMU fork, which now does model the
SX1262 — `hw/ssi/sx1262` forwards SPI to the same `VirtualSX1262` a native node
clocks, so an emulated node and a native one are the same radio. The wall this
section used to describe is gone.

The front-end module — the amplifier deciding how much power actually reaches
the antenna — is where this is still thin. The enable line crosses into the
model as a GPIO (`radio-fem`), and `Generic_E22_sx1262` exercises it under QEMU:
the module is switched in to transmit, and the board's output changes by the
gain its profile declares.

**This section used to say the Heltec T096 could not be run at all.** It runs,
under Renode, and passes every column of the board check except the front-end
one — which is the column that matters for it. The T096 is the board of the
transmit fault MeshCore 1.17.1 fixed: firmware that never raised the enable
line, so the PA's output went through the module's isolation instead of its
gain. Renode's nRF52840 platform does not drive the pin that board's module
hangs off, so the fault class is now *nearly* reproducible rather than out of
reach — one emulator can show it, and the board it actually happened on is on
the other one.

No fixture shipping today contains a node with a front end at all: every node in
the Scotland and Fife fixtures is a RAK4631, which has none. The front-end path
is exercised by the board check and by no scenario a user runs.

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
UART Service (`tools/ble/nus_peripheral.py`, verified against a real adapter on
elite). It is genuinely connectable from a real phone — but it is *our*
implementation of the companion protocol, not MeshCore's BLE code being
exercised. Removing the SoftDevice removed the firmware's Bluetooth stack along
with it, and the published images that now boot here are repeater builds, which
have no companion role to connect to in the first place.

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
- **GPU end to end.** Four kernels now have CPU twins tested against them —
  dechirp, demod, coverage (including its fold) and pairs — so the individual
  answers agree. What has still never happened is a *full scenario* run on the
  GPU path from end to end, which is where a mismatch in how the pieces are
  scheduled together would show up rather than in any one kernel.
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

Built since this list was first written: the desktop application (MSIM-10), the
terrain cut-through view (MSIM-11), coverage rasters and planning (MSIM-34/35),
terrain and boundary download (MSIM-38/37), the firmware catalogue (MSIM-13),
the provider interface with CoreScope, Beacon and MQTT (MSIM-30/31/32), battery
and solar (MSIM-19), external interference (MSIM-20), the MCP server (MSIM-36),
the validation harness (MSIM-28), replay (MSIM-27), board profiles (MSIM-18),
the multi-node console (MSIM-25), the emulated/native cross-check (MSIM-40), a
command line covering all of it (MSIM-23), the separate MIT repository for our
MeshCore builds (MSIM-39, `MeshBench/meshcore-native`, public since 9 August)
and the resource manager over everything downloaded at runtime.

### What the application does not do yet

**This section used to say the application had no map, and that this was the
largest remaining product gap. It has one.** Nodes project onto terrain as
points with a glyph per role, the basemap, hillshading, buildings, links and the
coverage raster are all layers that can be turned on, and a pair of nodes gives
both margins and the terrain cut-through that explains the verdict. The entry is
kept, struck through rather than deleted, because it was wrong for long enough
to be worth admitting.

The largest remaining product gap is now **the boards**. Ten are described in
`internal/world/scenario`, and as measured on 20 August exactly one —
`Generic_E22_sx1262` — passes every capability the board check asks of it. Two
more are green with a caveat. The rest either cannot be run at all, or run and
then go quiet:

- **Three nRF52 boards advert once and never again** (RAK4631, Xiao nRF52,
  Heltec Mesh Solar). All three report the channel busy, which is what a frozen
  clock would look like to CSMA — see the simulated RTC, which does not advance.
- **Two ESP32 boards boot and then assert inside ESP-IDF startup**
  (Xiao S3 WIO, Heltec V3).
- **Two have not been attempted** (Station G2, Heltec V2).
- **No emulated board has a console**, because the firmware talks to `Serial`
  — USB CDC — and neither emulator platform models USB. Anything that can only
  be established by asking the node a question is reported as untested rather
  than as passing.

That is the gap a new user meets first: they own a board, and the odds are it is
not one of the three that work. Everything in the physics above matters less to
them than that.

---

## 7. Unvalidated, which is different from unbuilt

**This section used to say the RF model had never been checked against a real
observation at all.** That is no longer true, and being precise about which half
has been checked matters more than the headline did.

**Checked against real air:** the bit-level conventions. Frames captured off a
real SX1262 over `rtl_tcp` on 18 August were demodulated offline and solved for
the sync-word chirps, the Hamming parity equations and the CRC's last-two-bytes
quirk — all three of which were wrong here before that session and are now held
by golden vectors carrying their own provenance. If our encoder drifts from what
that chip actually emitted, the suite says so.

**Checked against real observations:** the reporting bounds. The +15 dB
reported-SNR ceiling and the 0 to −127.5 dBm RSSI range come from 1,992 real
ScotMesh receptions rather than from a datasheet.

**Not checked against anything real:** the part that matters most to a planner —
*whether a link the model says will work, works*. Every propagation component is
tested against a published reference (ITU-R P.526, Semtech's sensitivity
figures, RadioLib's airtime) and the GPU against its CPU twin, but no predicted
margin has ever been compared with a packet that did or did not arrive on a real
hill.

The harness for exactly that comparison exists — `internal/study/validate`,
MSIM-28. It takes observed receptions, reports bias, spread and percentiles,
counts every exclusion, and refuses to treat a silent receiver as a negative
observation. What it has never had is data. Running it against a real CoreScope
or Beacon export remains **the single highest-value unfinished item in the
project**, and nothing else on this list would change more numbers.

Until it does, the propagation numbers are "correct according to the textbook",
which is not the same as "true on the hill above Aberfeldy".

---

## 8. Legal and licensing

**This section previously said no licence had been chosen, that there was no
`LICENSE` file, that attribution did not exist, and that the MeshCore build repo
had not been created. All four were true when written and none of them is true
now.** It is recorded here because a document about honesty that has quietly
gone stale is the failure mode it exists to prevent.

- **MeshBench is GPL-3.0-or-later**, decided 14 August 2026. The reasoning is in
  `docs/licence.md`: what is linked into this binary is permissive throughout,
  MeshCore is a separate process fetched at runtime rather than linked, and the
  emulator forks are aggregated beside the binary rather than combined with it.
  The one dependency that needed a decision — `eclipse/paho.mqtt.golang`, which
  is EPL-2.0 *or* EDL-1.0 — is taken under its EDL branch, and `tools/licgen`
  fails the build if a future dependency arrives under EPL alone.
- **Attribution is generated and enforced, not maintained by hand.**
  `tools/licgen` walks the build graph of `./cmd/meshcoresim`, reads every linked
  module's licence out of the module cache, and **fails the run** on a module
  whose licence it cannot name — the enforcement is the build, not a review. The
  curated half (the forks, the bundled native pieces, what is downloaded at
  runtime, the data sources) lives in `docs/licences.json` with its texts checked
  in beside it, so generation needs no network. Every bundle carries a `LICENCES`
  directory of the texts, the workbench embeds the same inventory as a licence
  window, and the release pipeline runs `licgen -require-project-licence` before
  it will publish.
- **Map data attribution is drawn where the data is shown.** Each basemap layer
  carries its own attribution string and the map renders it; that is an ODbL and
  CARTO requirement rather than a courtesy, and it is why the field is not
  optional on a layer.
- **The source offer is met by the pipeline.** GPL-3.0 §6 obliges a recipient of
  a binary to be able to get the corresponding source. The repository is private,
  so every release attaches `meshbench-<tag>-source.tar.gz`. When the repository
  is made public that archive can be replaced with a link.
- **`MeshBench/meshcore-native` exists and is public**, under MeshCore's own MIT
  terms. That is where MeshCore is compiled; nothing of it is linked here.

**What is genuinely still open:**

- **Contributions.** Alex holds the copyright alone, which is what keeps
  relicensing possible. A substantial outside contribution freezes that unless it
  arrives under a CLA, and there is no CLA and no `CONTRIBUTING.md` saying so.
  Decide before merging one, not after.
- **Nominatim's usage policy.** Boundary lookup calls the public OpenStreetMap
  Nominatim endpoint. The *data* licence is settled — ODbL, attributed — but the
  endpoint's own rate limits and identification requirements are a separate
  obligation that has not been checked against what the application actually
  sends.
- **The Ofcom worry recorded here was hypothetical and remains so.** No Ofcom
  register data is ingested anywhere; the only mention is a comment naming it as
  a place a person might look up a real site's ERP. There is nothing to resolve
  until something actually reads it.

---

## 9. Conditions that produce no error at all

Everything above is a limit of the model. This is a shorter and more dangerous
list: configurations that change a result while reporting nothing wrong.

**A region inferred and never applied.** Every node transmits, no node relays,
and nothing reports an error. It reads as a mesh with no propagation.

**A scope written without its `#`.** The key on the wire is `sha256("#sco")`.
`sha256("sco")` matches no repeater in existence, so every repeater receives the
packet, derives a different key, and declines to forward it. No error anywhere.

**Saved node state overriding a compiled default.** Both arms of a comparison
return identical numbers and the change looks inert.

**A bare version tag.** MeshCore tags one role at a time, so `v1.17.0` resolves
nothing while `repeater-v1.17.0` resolves.

**More than about eight emulated nodes on a twelve-core machine.** Boots
stretch, simulated time falls behind the wall clock, and the symptom is a mesh
gone quiet — which is what a genuine RF problem looks like.

**A permissive fixture.** More generous than the real network. This one does say
so, on screen and in the first line of the test runner's output, every time,
because a quiet version of it produces exactly the flattering-but-wrong answer
this document exists to prevent.

## 10. Measurement floors depend on the metric

The ±20% figure sometimes quoted is the spread of *reach under contention from
around eight simultaneous senders*, measured by running one configuration
repeatedly. It is a property of that contention, not of the simulator, and it
does not transfer to every measurement.

A one-originator flood on a 58-node network has produced an identical
transmission count across eight seeds while receptions on those same runs varied
by ±17%.

Measure the control's own spread on the metric in question, and quote that
figure rather than a general one.

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
