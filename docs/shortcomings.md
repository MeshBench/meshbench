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

### 3.5 The board's other radio is stubbed, and firmware that uses it will not work

Every ESP32 part carries Wi-Fi and Bluetooth. Neither is simulated: this
simulator has one radio and it is the LoRa transceiver on the board's SPI bus.

That would be unremarkable if an absent peripheral were simply absent. It is
not. `esp_wifi_init` runs Espressif's PHY blob, which reaches the analog blocks
through a register bridge at `0x6000E000` and waits for each transfer to
complete. With nothing answering, it waits for ever: measured on a LilyGo
T-Deck running wadamesh, **15,523,974 reads of one register**, and the
application never reaching its interface — a handheld frozen on its boot logo
with no clue as to why.

So the bridge is stubbed. It answers "the transfer finished", the calibration
returns, and firmware that brings Wi-Fi up at boot gets past it and goes on to
build its interface. What it does *not* get is a radio. Nothing is on the air,
no network can be joined, and a firmware that waits for an association will
wait for ever with the emulator saying nothing about why.

The Hardware tab says so, on every board whose MCU is an ESP32: *Wi-Fi and
Bluetooth — stubbed, never on the air*. It is derived from the MCU rather than
declared per board, so it cannot go stale as boards are added.

**Treat a node's Wi-Fi and Bluetooth as absent.** Anything a firmware reports
about them here is the stub talking, not a measurement.

The stub gets a firmware *past* the Wi-Fi PHY, but Bluetooth has a harder wall
behind it, and a firmware that brings BLE up at boot can stop there. A LilyGo
T-Deck running the Rust `mesh-rs` build is the measured case: it boots through
ESP-IDF, emits one companion advert, issues the display's `SWRESET` — and then,
about a second in, the whole executor freezes. Measured over ninety seconds:
the console emits its single advert and never another byte, the panel's chip
select is asserted exactly once (for that `SWRESET`) and never again, and there
is no reboot and no panic. It is not spinning either — a thousand register
touches, not fifteen million. It is *parked*: the last thing it did was sweep
the BT controller and radio-PHY calibration registers (`0x60011xxx`,
`0x60035084`+, the `r_rw_rf_init` path), and then it idled in `waiti` with
nothing left to wake it — the way a NimBLE host waits on an HCI command-complete
event from a controller that here has no silicon to raise it. The dev's own build reports
BLE back-off loops for exactly this, and the emulator's `TIMG0` alarms halving
in the background are those back-offs. So the panel never gets its `SLPOUT` and
the LoRa radio is never touched: both are downstream of a boot that never
returns from Bluetooth. The controller's exchange RAM is faked (at `0x3FC00000`,
so the older `LoadStoreError` panic in `r_rw_rf_init` no longer reboots the
board), which is why `mesh-rs` stays *alive* rather than crashing — but staying
alive is as far as it gets. A companion build that talks over USB instead of
BLE, MeshCore's own among them, never enters this path and boots to its
interface.

There is no BLE-controller emulation to borrow for this. Espressif's own
`esp-emulator` does carry a virtual BLE controller that runs unmodified NimBLE
firmware (Apache-2.0, so its licence would allow it), but it emulates only the
RISC-V parts — C3, C6, H2, P4 — not the Xtensa S3, and it intercepts HCI by
firmware symbol, which a stripped closed binary like `mesh-rs` does not carry.
So closing this would mean writing an S3 BLE controller from nothing, which is a
radio on the air by another name and out of scope here. It is recorded, not
attempted.

### 3.5a Which wire an application's console is on, and what happens when it is the wrong one

An ESP32-S3 board built with `ARDUINO_USB_CDC_ON_BOOT` puts Arduino's `Serial`
on the USB Serial/JTAG peripheral rather than on UART0. Six of the boards here
are built that way — the T-Deck, the RAK3112, the Heltec Wireless Tracker, the
E213, the E290 and the Wireless Paper — and until that peripheral carried bytes
they printed their ROM banner to one wire and everything afterwards into a
register stub. They read as boards that started and then stopped talking, and
the companion protocol, which rides the same port, had no far end.

It carries bytes now, and which port a board uses is recorded with the board
(`QEMUWiring.ConsoleOnUSB`). UART0 is still logged separately on those boards —
the *boot* source in a node's Output tab — because the ROM prints there before
any of this is configured, and that output is what says whether a board started
at all.

**Where this can still be wrong:** the flag is a property of the *build*, not
of the silicon. A board's profile records what MeshCore's own variant does. An
imported firmware compiled the other way round will have its console on the
wire the profile does not name, and will read as silent. The Output tab's boot
source is the check: if the ROM banner is there and nothing follows it, the
console is on the other wire.

### 3.5b What a firmware may still walk into, and what happened when one did

The emulated ESP32-S3 answers a great deal that it does not model, and the
answers are per register rather than blanket, because the two conventions are
mixed — some waits end when a busy bit clears and some when a done bit sets.
That table grows as firmware finds the gaps in it, and it grows by measurement.

Running `mesh-rs`, an ESP-IDF v5.5 firmware for the T-Deck, found four things
in one sitting, each hiding the next: a PSRAM model that indexed backwards on
an address with its top bit set and took the emulator down with it; a command
register on the analog bus that a newer PHY blob uses and the table did not
answer; an RSA accelerator that handed a zero modulus to libgcrypt, which
aborts the process; and a timer group that divided by a zero prescaler, which
the watchdog beside it had guarded against for years. All four are fixed.

Three more followed, and the first of them was the black screen itself. The RF
front end's registers were declared four bytes at a time, and a narrower access
is not a slower access: QEMU refuses it and the guest takes a fault. `mesh-rs`
reads one of those registers a byte at a time, masks a bit and writes it back —
an ordinary read-modify-write — and took that fault on its first attempt, after
which its handler faulted on its own stack and the board disappeared into a
storm of double exceptions with nothing printed. The SYSTEM block also forgot
everything written to it, so every `REG_SET_BIT` on a peripheral clock-enable
was a no-op. And the second core ran from the moment the machine came up, so it
sat in the ROM's "wait for somewhere to jump" loop and took the first word
written to the message register — which for this firmware is a message, written
long before it starts a core.

**Where it reaches now:** it boots, brings up its PHY and its PSRAM, draws one
whole 320×240 frame to the panel, and speaks its own framed protocol on the USB
port. Then it raises a software interrupt with `wsr.intset`, restores `PS` to
let it in, and its own interrupt entry faults — an unending storm of double
exceptions with a stack pointer descending 256 bytes a time. Measured with no
peripherals attached, so it is not the panel, the card slot, the keyboard or
the touch panel. It never reads the touch panel at all, where a working build
reads it 56 times in the same window.

**It stops on one instruction.** Printing `EXCCAUSE` at every exception ends
the search: the software interrupt is delivered with a valid stack
(`exccause = 4`), and the very next event is a double exception with
`exccause = 32` — CoprocessorDisabled — at `0x40378b04`, which disassembles to
`rur.fcr`: reading the floating-point control register, inside the firmware's
own context-save, two instructions after it saves the MAC16 accumulators.
`CPENABLE` is zero, so it traps; a trap inside an exception vector with
`PS.EXCM` set is a double exception, which re-enters the same vector and
reaches the same instruction. Forty-three thousand of them in eighteen seconds,
and not one ever handled — nothing enables the FPU.

Everything else this firmware appears to do is downstream of that loop: the
stack walking out of RAM, the twelve million rejected writes, the single frame
drawn and then silence.

**The emulator is right about it**, checked rather than assumed: Espressif's own
configuration for this part gives coprocessor 0 as the FPU and `CPENABLE`
resets to zero, as the ISA specifies. The same instruction with the same
`CPENABLE` traps identically on silicon.

That ordering question was tested rather than left open. QEMU recognises a
pending interrupt on the instruction after `wsr.ps`, while the ISA says a `WSR`
to `PS` is not guaranteed in effect until an `RSYNC` — and this firmware has no
`rsync` in between. Moving the check off `wsr.ps` and onto `rsync`, where the
ISA puts it, changes nothing: the storm is identical. Reverted, because an
unproven change to shared Xtensa translation would reach every other guest.
Also checked and correct: exception entry never clears `CPENABLE`, here or on
silicon.

What remains is a statement about firmware state rather than about this
emulator. The firmware does manage `CPENABLE` — its image carries four
`wsr.cpenable` and sixty-four `rsr.cpenable` — so on a real board the
interrupted task has the FPU enabled and this handler survives. Which task is
running when a self-raised software interrupt fires is not something that can
be established from outside a closed binary.

**There is a way to look past it**, and it is deliberately not the default. It
brings the emulated coprocessors up enabled, which the part does not do. With
it on, this firmware stops looping — no refused stores at all, against eighteen
million — and reaches its own panic handler, which prints *panic details
unavailable after restart*. That is a fault worth being able to see, and it was
completely hidden before.

It is a property of the firmware being looked at rather than of the hardware —
the same T-Deck runs a stock MeshCore image that wants nothing to do with it —
so it is stored per build, beside the image, and follows the build when it is
renamed or moved. Set it in the build's own window in the Firmware Library, or
with `firmware.update {version, coproc_at_reset: true}`.
`MESHBENCH_QEMU_COPROC_AT_RESET=1` still forces it on for every board at
once, which is the form a script reaching for it once wants.

**Where that firmware stops now.** Past the trap it brings up PSRAM, resets the
display controller, and initialises a card completely — CMD0, CMD8, CMD58,
CMD16, CMD9, CMD10, ACMD51, ACMD13, with the CSD and CID read back as data
blocks, where before it died at CMD8. It then survives its own interrupt
dispatch, which it did not: the machine's interrupt matrix laid every address
in its window out as a map register, so the four status registers at 0x18C —
the ones a firmware reads to learn *which* peripheral interrupted it — returned
map bytes. That firmware recognised nothing in them and called a null handler,
thirteen jumps to address zero per run, ending in a panic. With the registers
answering, both are zero.

It still does not draw. It sends the display's SWRESET and never follows with
SLPOUT, so the panel stays dark and the Hardware tab shows a black screen. What
is known about that: its executor is alive and being woken — the timer group
fires and is dispatched, and the alarms it sets halve repeatedly, which is the
shape of something retrying with a backoff. Its SD card work finishes and then
no further byte crosses the SPI bus. The delay a display needs after a software
reset is 120 ms and guest time reaches several seconds, so it is not waiting on
that clock.

One thing worth knowing before chasing it: **guest time here runs about fifty
times slower than the wall clock** — measured at 2.66 seconds of emulated time
in 150 seconds of real time on this machine. A firmware waiting on a long
timeout will look hung long before it is.

The stock MeshCore image for the same board is the control: it runs the whole
ST7789 sequence — SWRESET, SLPOUT, COLMOD, MADCTL, CASET, RASET, INVON, NORON,
DISPON — and draws, on the same machine, in the same run configuration. So the
panel model, the SPI wiring and the board profile are all right; what remains
is inside the closed firmware, which stops in a panic handler of its own before
it reaches SLPOUT. See §3.5d.

Treat anything measured under that switch as measured on a machine that is
lying about a register: a firmware which genuinely mismanages its floating
point enable is flattered by it rather than caught. It exists to make the next
fault visible, not to make a board work.

**Seven explanations were tested and disproved** getting here, all by
measurement: the interrupt's core configuration, the MMU flag bits, the MMU
page numbering, PSRAM pages past the end of the fitted part, the fabricated
Bluetooth memory block, the rejected writes themselves, and interrupt delivery
timing.

### 3.5c Third-party firmware and the flash a filesystem is kept in

Two firmware families run under emulation besides MeshCore's own build, and for
a while neither could keep a file — nor could MeshCore. Every filesystem read
on every ESP32-S3 board came back two bytes late: an erased sector read as
`00 00 ff ff ...`, a freshly written superblock read back as garbage, and
LittleFS and SPIFFS both refused to mount or format.

The cause was in the emulator, not any of the firmware. The flash controller
and the flash chip model disagreed about the read's dummy phase: a quad I/O
read clocks its dummy cycles on four lines, and the controller counted them as
one bit each, so it sent one dummy byte where the chip expected three and every
data byte after arrived two places late. The application itself was untouched,
because it executes through the cache — which reads the image directly and never
goes down the controller's data path — so every board booted and nobody noticed
that nothing could be *kept*. Fixed in the fork by counting the dummy phase at
the width of the read.

What runs now, each measured on the board's own profile:

| firmware | board | result |
|---|---|---|
| MeshCore 1.17.1 companion | LilyGo T-Deck | boots, formats SPIFFS, reaches its home screen (node ID, message count, "Connected"), answers a USB companion query, and keeps its identity across a reboot |
| MeshCore 1.17.1 repeater | Heltec V3 | formats SPIFFS, radio active, same Repeater ID on the second boot as the first |
| Meshtastic 2.7.26 | LilyGo T-Deck | boots, PSRAM, I²C, draws; LittleFS formats and mounts clean on the second boot |
| Meshtastic 2.7.26 | Heltec V3 | the same, on the GigaDevice flash model |

A first boot on wiped flash now spends real time formatting, because the format
actually happens — at this machine's emulation speed, minutes for a multi-megabyte
partition. It is done once; the flash persists, so the second boot mounts
immediately.

**The T-Deck companion's screen is a status display, not an on-screen menu.**
The `companion_radio_usb` build reads only its GPS switch and one button; it is
driven over USB (or BLE on the BLE build), and shows node ID, unread count and
link state on the panel. On-screen navigation belongs to the GUI-first builds,
which are a different image.

### 3.5d mesh-rs on the emulated T-Deck: boots, does not draw

mesh-rs is closed-source Rust firmware. Past the emulator faults documented
above it boots, brings up PSRAM, resets the display controller, initialises a
card completely, and — unlike MeshCore and Meshtastic — drives its display over
SPI **DMA** rather than polled SPI. Two more emulator gaps were fixed to follow
it that far: the general-purpose SPI controller had no DMA data path at all (a
DMA transfer clocked nothing, because the model only read the CPU data
registers), and the GDMA's channel lookup returned the wrong channel (its
"peripheral matches AND is started" test was coded as OR). With both fixed, a
DMA transfer moves its data and raises the end-of-list interrupt the driver
waits on — verified: the transfer completes and the correct completion status
is raised on the right channel.

It still does not draw. After that first DMA completes it sleeps in a backoff
of its own, not polling or waiting on any register this emulator can be shown
to mishandle — the completion interrupt it asked for is delivered. The stall is
inside the closed firmware. The stock MeshCore and Meshtastic images drive the
same panel on the same machine and draw, so it is recorded as a firmware-side
blocker rather than an emulator one.

### 3.5e wadamesh, and the state of the third-party bring-ups

wadamesh (an older Arduino/ESP-IDF companion build, distinct from the Rust
mesh-rs) **boots and runs** on the emulated T-Deck: NVS, PSRAM, touch
detection and the main loop all come up, and it polls the GT911 touch panel
actively. It is not stuck — what looked like a hang is its main loop reading
the battery ADC every iteration.

Two rough edges, both fidelity rather than correctness, and neither introduced
by the fixes above (no input, touch, keyboard or GPIO code was changed):

- **The ADC calibration eFuse is modelled, and the battery meter is the
  inverse of the firmware's own curve.** A real ESP32-S3 has ADC calibration
  burnt in eFuse BLK2 at the factory (`BLK_VERSION_MAJOR` at bit 128 = 1 for
  "ADC calib V1"); a blank block made `esp_efuse_rtc_calib_get_ver()` return 0,
  so `esp_adc_cal_characterize` logged *"No calibration efuse burnt"* / *"cal-
  ibration efuse version does not match"* — on every reading, for a firmware
  that reads the ADC in its loop. The emulated S3 eFuse now seeds
  `BLK_VERSION_MAJOR=1` (`esp32s3_efuse.c`), so the firmware takes the
  calibration path and the log is quiet. The ADC diffs are left zero, which the
  firmware reads as the baseline V1 curve (ADC1 init codes 1850/1940/1940/2010,
  cal points 3200/2400/1700/900 at 850 mV).

  That curve is **not** the linear default the uncalibrated fallback used, so
  burning the version alone would have shifted every S3 board's reported voltage
  — by about +15 %, measured. So it is a coordinated change: `batteryMeter`
  encodes the raw as the *inverse* of that exact curve (transcribed from
  esp-idf's `curve_fitting_coefficients.c` and `esp_efuse_rtc_calib.c`),
  evaluated at the voltage the board's halving divider puts on the pin. The
  firmware reads the true cell voltage back — more consistent than before, since
  every S3 firmware now shares one calibration curve rather than its own
  uncalibrated fallback. The battery meter is on ADC1 at 12 dB (atten 3), whose
  3300 mV pin full scale is what each board's `FullScaleMV` divider is stated
  against. `TestBatteryMeterReportsTrueVoltage` and the curve round-trip tests
  hold it to the firmware's arithmetic.
- **The first-boot filesystem format is slow**, because it happens in real
  time at emulation speed — the LittleFS/SPIFFS format a firmware does on a
  blank partition or SD card can take tens of seconds of guest time, which is
  minutes of wall clock. It is a one-time cost: the flash and card persist, so
  the next boot mounts at once. A workbench boot-offset check can time out
  during that first format; the firmware keeps running behind it.

The three third-party images now stand as: **MeshCore** works fully (companion
flow, screen, identity); **Meshtastic** boots, mounts LittleFS on the second
boot, and draws its full UI; **wadamesh** boots and runs with the rough edges
above; **mesh-rs** boots and initialises everything but stops in a panic of its
own before drawing (§3.5d). The remaining gaps are firmware-side or
emulation-fidelity limits rather than emulator faults on any path that could be
verified.

### 3.6 Forwarding policy is ours, not the repeater application's

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

### 3.7 BLE is ours, not the firmware's

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
  `tools/licgen` walks the build graph of `./cmd/meshbench`, reads every linked
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
