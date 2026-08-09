# Shortcomings

What MeshcoreSim does **not** model, does not model well, or has not yet been
checked against reality. Everything here is known and deliberate; none of it is
a bug report.

The reason this document exists is in `CLAUDE.md`: *"The simulator is kinder
than the air. Say so in the UI — never let a user assume otherwise."* A
simulator that does not publish its own error budget is a confident liar.

Ordered by how likely each one is to change a decision someone makes on the
strength of a simulation.

---

## 1. Physics that is absent

### 1.1 No multipath — the biggest single gap

Every link is one coherent path. Real 868 MHz propagation over UK terrain
arrives as many paths with different delays, and their vector sum fades in and
out as anything moves.

**Consequence.** The simulator cannot produce the failure that dominates real
marginal links: a path with a perfectly good median signal that drops out for
hundreds of milliseconds at a time. A link MeshcoreSim calls reliable may be
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

### 3.4 Only one board family actually runs

nRF52840 under Renode. The ESP32 path boots under Espressif's QEMU fork but hits
the same wall — `radio_init()` drives an SX1262 that QEMU does not model, and no
ESP32-side peripheral exists yet. Board profiles (MSIM-18) are specified, not
implemented.

### 3.5 BLE is ours, not the firmware's

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
| The application itself | MSIM-10 — everything so far is libraries, tests and render tools. There is no desktop shell yet. |
| Coverage rasters, planning tools | MSIM-34, MSIM-35 |
| Terrain and boundary auto-download | MSIM-38, MSIM-37 |
| Prebuilt firmware catalogue | MSIM-13, ADR-0020 |
| Observer import and real-traffic replay | MSIM-27 |
| Provider interface, CoreScope/Beacon/MQTT | MSIM-30, 31, 32 |
| MCP server | MSIM-36 — the Claude skill exists; the server does not |
| Battery and solar | MSIM-19 |
| External interference, antenna filters | MSIM-20, MSIM-21 |

---

## 7. Unvalidated, which is different from unbuilt

**The RF model has never been checked against a real observation.** Every
component is tested against a published reference — Semtech's sensitivity
figures, ITU-R P.526, RadioLib's airtime — and the GPU against its CPU twin. No
part of it has been compared with a packet that actually crossed real air.

MSIM-28 is that comparison and it is the single highest-value unfinished item in
the project. Until it runs, every number here is "correct according to the
textbook", which is not the same as "true on the hill above Aberfeldy".

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
**treat a MeshcoreSim result as a best case.** If it says a link will not work,
believe it. If it says a link will work marginally, go and measure.
