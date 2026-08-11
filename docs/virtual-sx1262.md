# Option B: a virtual SX1262 beneath real RadioLib

The aim is that everything above the SPI pins is the real software stack —
MeshCore's `Dispatcher`, its `CustomSX1262` driver, and RadioLib itself — so a
change anywhere in it becomes observable. Today we replace all three with about
a hundred lines of `HostRadio`, which is why two firmware versions differing
only in their radio driver produced byte-identical results.

## The seam, and why this is smaller than it sounds

MeshCore pins **RadioLib 7.6.0**, and RadioLib ≥6 has a hardware abstraction
class already:

    class RadioLibHal {
      virtual void pinMode(pin, mode) = 0;
      virtual void digitalWrite(pin, value) = 0;
      virtual uint32_t digitalRead(pin) = 0;
      virtual void attachInterrupt(num, cb, mode) = 0;
      virtual void detachInterrupt(num) = 0;
      virtual void delay(ms) = 0;
      virtual void delayMicroseconds(us) = 0;
      virtual RadioLibTime_t millis() = 0;
      virtual RadioLibTime_t micros() = 0;
      virtual long pulseIn(pin, state, timeout) = 0;
      virtual void spiBegin() = 0;
      virtual void spiBeginTransaction() = 0;
      virtual void spiTransfer(out, len, in) = 0;
      virtual void spiEndTransaction() = 0;
      virtual void spiEnd() = 0;
    };

Sixteen pure-virtual methods, all of them trivial except `spiTransfer`. There is
no Arduino to fake and no build surgery on RadioLib: it is written to be ported.

That leaves the real work in one place — **what answers the SPI transactions** —
which is the SX1262 itself.

## Shape

    MeshCore Dispatcher                  unchanged, real
      CustomSX1262 / RadioLibWrapper     unchanged, real  <- the 1.17 diff lives here
        RadioLib SX1262                  unchanged, real, vendored
          Module -> SimHal               ours: pins, SPI, time, interrupts
            VirtualSX1262                ours: the chip
              bridge -> RF engine        existing lockstep link

Two new components, and one deletion.

**`SimHal : public RadioLibHal`** — pins as an array, SPI transfers handed to the
virtual chip, time from the node's simulated clock, and interrupt callbacks
recorded rather than registered with an OS.

**`VirtualSX1262`** — a command interpreter and a state machine. It holds the
chip's registers, its data buffer, its IRQ flags and its BUSY line, and it is
where the engine's view of the air becomes something a driver can read.

**Deleted: `HostRadio`.** Once this works, `mesh::Radio` is implemented by
MeshCore's own `CustomSX1262`, and the only thing we provide is a chip.

## What the chip has to do

RadioLib's SX126x driver defines 44 command opcodes. It issues far fewer at
runtime for a LoRa link, and they cluster into four groups.

**Configuration**, answered by storing the value:
`SET_STANDBY`, `SET_PACKET_TYPE`, `SET_RF_FREQUENCY`, `SET_MODULATION_PARAMS`,
`SET_PACKET_PARAMS`, `SET_BUFFER_BASE_ADDRESS`, `SET_PA_CONFIG`, `SET_TX_PARAMS`,
`SET_REGULATOR_MODE`, `SET_DIO2_AS_RF_SWITCH_CTRL`, `SET_DIO3_AS_TCXO_CTRL`,
`CALIBRATE`, `CALIBRATE_IMAGE`, `SET_LORA_SYMB_NUM_TIMEOUT`,
`SET_RX_TX_FALLBACK_MODE`, `WRITE_REGISTER`, `READ_REGISTER`.

**Data**: `WRITE_BUFFER`, `READ_BUFFER`, `GET_RX_BUFFER_STATUS`.

**Operation**: `SET_TX`, `SET_RX`, `SET_CAD`, `SET_CAD_PARAMS`, `SET_SLEEP`.

**Status and interrupts** — the part that matters:
`SET_DIO_IRQ_PARAMS`, `GET_IRQ_STATUS`, `CLEAR_IRQ_STATUS`, `GET_STATUS`,
`GET_PACKET_STATUS`, `GET_DEVICE_ERRORS`, `CLEAR_DEVICE_ERRORS`, `GET_STATS`.

Configuration can be a map of opcode to stored bytes. Operation and interrupts
are the design work.

### The IRQ flags are the product

    RX_DONE, TX_DONE, PREAMBLE_DETECTED, HEADER_VALID, CRC_ERR,
    CAD_DONE, CAD_DETECTED, TIMEOUT

Every one must become true at the right *simulated instant*:

| Flag | When |
|---|---|
| `PREAMBLE_DETECTED` | a few symbol times into a transmission this node can hear |
| `HEADER_VALID` | after the LoRa header would have been demodulated |
| `RX_DONE` | when the engine delivers the frame, if it decoded |
| `CRC_ERR` | when it arrived but did not decode |
| `TX_DONE` | when the engine says our waveform ended — never computed here |
| `CAD_DETECTED` | if anything is audible during the CAD window |
| `TIMEOUT` | when the driver's own timeout expires with nothing heard |

`PREAMBLE_DETECTED` is the one the whole exercise is for: 1.17's rewrite is about
detecting a preamble properly and coping with flags that stick, and it cannot be
exercised by a shim that never sets the bit.

### BUSY, and the trap in it

Every SX1262 command drives BUSY high until it completes, and RadioLib spins on
`digitalRead(BUSY)` waiting for it to fall. **In lockstep, a spin that does not
advance simulated time never ends.**

The rule: the node's clock belongs to `SimHal`, and `delay`, `delayMicroseconds`
and the BUSY read all advance it. A command takes a plausible number of
microseconds and BUSY falls when the local clock passes that instant. The
firmware therefore experiences realistic command latency, and the simulation
cannot deadlock inside a driver.

This is the single most likely place to hang the whole workbench, so it gets a
watchdog: if a node's local clock advances beyond a tick's worth without the
firmware returning, the run fails loudly rather than freezing.

## Time

Two clocks, and they must not be confused.

- **Simulated network time** — the engine's, advanced in ticks, shared by every
  node. Transmissions, deliveries and airtime live here.
- **Node local time** — what `millis()` and `micros()` return to RadioLib and
  MeshCore. Starts at the tick's instant and advances with command latency and
  `delay()` inside the tick.

At the end of a tick the node's local clock is reconciled to the network clock.
If it has run past the tick, the excess carries into the next one, which is what
keeps a node that did a lot of SPI work from silently getting free time.

`getTimeOnAir()` must return what the engine actually charges. CLAUDE.md already
requires this and it becomes load-bearing here: the firmware's CSMA arithmetic is
built on it, and a disagreement desynchronises the two silently.

## The non-regression harness — build this first

A two-node exchange, captured before anything changes, is what makes the whole
migration checkable. It is the first thing to write, not the last.

**`internal/engine/radiostack_test.go`**

    scenario: two repeaters, fixed positions, one seed, clean node storage
    action:   node A originates one message
    assert:   node B receives and accepts it

Recorded as golden artefacts:

1. **The frames.** Every transmitted frame, byte for byte, hex encoded.
2. **The ledger.** Every event: instant, kind, from, to, outcome, detail.
3. **The consoles.** What each firmware printed, which catches a driver that is
   working but complaining.
4. **The summary.** Transmissions, receptions, airtime per node.

Run it against today's `HostRadio` and commit the result as the baseline. Then,
at every step of the migration, run it again.

### What must match, and what may not

**Must match exactly**, at every step: the frame bytes. The message MeshCore
builds does not depend on which radio carries it, and if those change, something
is wrong in a way no amount of timing tolerance should hide.

**Must match exactly by the end**: which node received what, and whether it
accepted it. A two-node link with margin has no legitimate reason to deliver
differently.

**May legitimately differ**: absolute timing, by the command latency the virtual
chip introduces — the real stack does SPI work that `HostRadio` never did. The
test asserts *ordering* and a tolerance, not equality, and the tolerance is
stated in the test with the reason.

**Expected to differ, and the point of it all**: once IRQs are real, two firmware
versions should stop agreeing. The harness gains a second scenario — six senders
into one channel, the contended case — whose purpose is to *fail* to be identical
across 1.16 and 1.17. Until it does, this work is not finished.

## Build order

Each step ends with the harness green, and each is independently useful.

### 1. The harness, against today's shim

Write it, run it, commit the golden files. Nothing else changes. If the baseline
is not reproducible now, nothing later can be trusted — and this is the cheapest
possible moment to discover that.

### 2. Vendor RadioLib and compile it for the host

Add RadioLib 7.6.0 to `meshcore-native`, with a `SimHal` whose methods are all
stubs, and a `VirtualSX1262` that answers `GET_STATUS` and nothing else. Build
`CustomSX1262` against it. Do not wire it up.

*Test:* it links, and the harness still passes on the old path. **Confirm
RadioLib's licence** and record it beside the vendored copy, as with the
Wireshark dissector — this is a dependency decision, not a detail.

### 3. Configuration and BUSY

Implement the configuration opcodes, the BUSY line, and the local clock. Bring a
node up on the new stack far enough that `begin()` succeeds and the driver
reaches its idle state.

*Test:* a node boots on the virtual chip and prints its banner. The harness still
runs on the old path; the new one need only initialise.

### 4. Transmit and receive

`SET_TX` hands the buffer to the bridge; `TX_DONE` fires when the engine reports
the waveform ended. Delivery from the engine fills the buffer and raises
`RX_DONE`. `GET_PACKET_STATUS` returns the engine's real RSSI and SNR.

*Test — the important one:* **switch the harness to the new stack and expect the
frames to be byte-identical and delivery unchanged.** Timing may move; nothing
else may. Any other difference is a bug in the new path, and finding it here,
with two nodes and one message, is worth a great deal more than finding it in a
154-node sweep.

### 5. Interrupts, CAD and preamble detection

The flags table above, driven by the engine's per-node view of the air. Delete
`HostRadio` and its `isReceiving()`: the answer becomes MeshCore's, computed by
its own driver.

*Test:* the harness is unchanged, **and** the contended scenario stops producing
identical ledgers across 1.16 and 1.17. Add per-node counters for deferrals, CAD
busy and CAD timeouts so the mechanism can be seen rather than inferred.

### 6. The rest of the family

`SX1268`, `STM32WLx` and `LR2021` share the wrapper and most of the command set;
1.17 touched all four. Worth doing only if we want to ask whether a change
behaves differently per radio — which is a real question, but a later one.

## What could go wrong

- **A hang inside the driver.** The most likely failure and the worst, because
  it presents as a frozen workbench. Mitigated by the local clock advancing on
  every blocking call and by a watchdog that fails the tick instead of spinning.
  This has already bitten twice in this codebase; assume it will again.
- **Timing that looks like a result.** Command latency shifts every transmission
  slightly. Step 4's "frames identical, timing may move" test exists to separate
  the two before step 5 makes them hard to tell apart.
- **Silent airtime disagreement.** If `getTimeOnAir()` and the engine differ, the
  firmware's CSMA drifts out of step with the channel and nothing says so. Assert
  the two against each other directly, not only through behaviour.
- **RadioLib doing something we have not modelled** — image calibration, TCXO
  startup delay, register reads we return zero for. These usually surface as a
  driver refusing to leave `begin()`, which step 3 is sized to shake out.
- **Upstream churn.** We would track two projects' assumptions instead of one,
  and MeshCore bumping RadioLib becomes our problem. That is the recurring cost,
  and it is the honest argument against doing this at all.
- **It may still not move the numbers.** If 1.17's improvement depends on radio
  behaviour our channel does not model — a real chip's preamble detection under
  interference, say — the arms could remain identical for a new reason. Step 5's
  test is written to say that plainly rather than leave it ambiguous.

## "Is it the actual firmware then?"

Worth stating exactly, because the answer is nearly yes and the remainder
matters.

**MeshCore's own source is already compiled unmodified, today.** The build points
at a checkout and compiles `src/**` and `examples/<role>/*.cpp` as they are.
Nothing is patched. The only build-time influence is three defines and one role
flag, and the one flag that changes behaviour - `MAX_GROUP_CHANNELS=40` - exists
because the companion reports its capacity in a protocol frame and a host has no
flash limit to derive it from.

What is substituted is the **platform beneath** the firmware, in
`variants/host/`: the Arduino core, the board, the flash filesystem, the RTC,
the sensors, the RNG - and, today, the radio.

Option B moves exactly one of those from ours to theirs. After it:

| | today | after |
|---|---|---|
| MeshCore application and mesh logic | real | real |
| MeshCore radio driver | ours | **real** |
| RadioLib | absent | **real** |
| SX1262 chip | absent | ours, virtual |
| Arduino core, board, filesystem, RTC, sensors, RNG | ours | ours |
| The air: modulation, noise, capture effect | the engine | the engine |

The bottom row must stay ours - that is the simulator, and the reason any of
this is worth doing. The row above it is where the hardware ends.

### Two caveats

**We build as an nRF52.** `-DNRF52_PLATFORM` is set, so where MeshCore takes a
platform-specific path we take that one. It is therefore the nRF52 flavour of
the firmware, not a platform-neutral one, and a bug that only appears on ESP32
will not appear here. Worth revisiting once the radio stack is real, because the
platform define currently also reaches the radio.

**A virtual chip is a model of a chip.** After this, everything above the SPI
pins is genuinely the software that runs on hardware. What answers those pins is
still our best understanding of an SX1262, and a behaviour we have not modelled -
a real preamble detector under interference, a real chip's errata - is a
behaviour the firmware cannot show us. The stack becomes real; the silicon does
not.

## Effort and what it buys

Several times option A: the HAL is an afternoon, the configuration opcodes a day,
and the IRQ and timing model is the rest — call it the bulk of a week of focused
work, with the harness paying for itself the first time it catches a regression.

What it buys is that the claim on the front of the tool becomes true of the part
that matters. "It runs real MeshCore firmware" is already true of the mesh logic;
after this it is also true of the code that decides *when to transmit*, which is
what every question asked of this simulator so far has actually been about.
