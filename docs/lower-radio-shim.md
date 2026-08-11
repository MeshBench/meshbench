# Moving the shim below the driver

## The problem, stated precisely

MeshBench replaces MeshCore's radio at the **top** of its driver stack:

    mesh::Radio                 <- abstract; Dispatcher talks to this
      RadioLibWrapper           <- MeshCore's own driver base
        CustomSX1262            <- per-chip driver
          SX1262 (RadioLib)     <- third-party library
            Module              <- SPI, GPIO, DIO interrupts
              hardware

`HostRadio : public mesh::Radio` sits at the first line and everything below it
is absent. That is why the CAD study could not see 1.17's improvement: the
diff between 1.16.0 and 1.17.0 is almost entirely in `src/helpers/radiolib/` —
`CustomSX1262.h` (+81 lines), `RadioLibWrappers.cpp` (+48), and the same for
SX1268, STM32WLx and LR2021. The listen-before-talk rewrite, the preamble-detect
fix and the stuck-IRQ workaround all live in code we substitute wholesale.

The shared core barely changed: `Dispatcher` gained four lines, plumbing a
`setCADEnabled()` that defaults to a no-op. So of the firmware we *do* run,
almost nothing differs between the two versions — which is exactly what twelve
runs of byte-identical timing showed.

**We are not measuring the firmware's radio behaviour. We are measuring ours.**

## What "lower" could mean

Three levels, and they are not equally worth doing.

### A. Shim at the RadioLib class boundary — recommended

Provide our own `SX1262` class with RadioLib's public API, so MeshCore's
`CustomSX1262` compiles and runs unmodified against it.

What starts running for real: `RadioLibWrapper`, `CustomSX1262`, and with them
every line 1.17 changed — how the driver arms interrupts, how it interprets
preamble-detect, its workaround for flags that stick, how `isReceiving()` is
actually decided.

The API surface is small. Measured against 1.17.0's driver sources, the calls
used are:

    startReceive  startTransmit  finishTransmit  standby  sleep
    readData      getPacketLength  getRSSI  getSNR  getTimeOnAir
    setOutputPower  setPreambleLength  setPacketReceivedAction
    random  randomByte
    + the IRQ surface: setDioIrqParams / getIrqFlags / clearIrqFlags / scanChannel

Fifteen or so methods and one flags register. The IRQ surface is the whole
point — everything interesting in the 1.17 diff is about when those bits are
set, read and cleared.

**Effort:** the bulk is not the methods, it is deciding *when each IRQ fires* in
simulated time and wiring that to the engine's channel state. Perhaps 600–900
lines of C++ in the shim, plus a small amount of Go to expose per-node channel
occupancy at finer granularity than the current per-tick boolean.

**What it does not do:** RadioLib itself still is not running. Bugs that live in
RadioLib, or in how MeshCore drives its exact state machine, remain invisible.

### B. Shim at the SPI/Module boundary

Emulate the SX1262 *chip*: its command opcodes, registers, FIFO and DIO lines,
beneath real, unmodified RadioLib.

This is the honest maximum. Everything above the SPI pins is the real software
stack, and a change anywhere in it — MeshCore, RadioLib, or the interaction —
becomes observable.

**Effort:** several times A. The SX1262 command set is large, RadioLib exercises
a lot of it, and every timing-sensitive path (BUSY line, calibration, IRQ
latency) has to behave plausibly or the driver wedges in ways that look like RF
problems. This is a project, not a task.

**When it becomes worth it:** if we ever need to answer questions about the
driver's own correctness rather than the mesh's behaviour, or to bring up a chip
MeshCore has just added support for.

### C. Leave it, and say so

Keep the current shim and state plainly, in the tool, that driver-level changes
are outside what it can measure. Cheap, honest, and leaves the original question
unanswered.

## Recommendation

**Superseded: B was chosen.** The detailed plan is in
[virtual-sx1262.md](virtual-sx1262.md).

The deciding fact came out while sizing it: RadioLib 7.6.0 - the version MeshCore
pins - already has a `RadioLibHal` abstraction with sixteen pure-virtual methods.
There is no Arduino to fake and no surgery on RadioLib; it is written to be
ported. That moves B from "a project, not a task" to roughly a week, and it is
the only option that makes the tool's central claim true of the code that decides
when to transmit.

What follows is kept as the reasoning that led there.

**A, in four steps, with a decisive test at the end of each.**

### Step 1 — a virtual SX1262 that compiles

Write `variants/host/SX1262.h` providing RadioLib's class shape, and build
MeshCore's `CustomSX1262.h` against it instead of `HostRadio`. Do nothing
clever: return sensible constants, no IRQs, no traffic.

*Test:* `simple_repeater` links and boots. The compiler is the discovery
mechanism here — every method MeshCore actually calls shows up as an error, and
the list stops being guesswork.

### Step 2 — transmit and receive through it

Wire `startTransmit`/`readData` to the existing bridge messages, and make
`RX_DONE` fire when the engine delivers a frame. Airtime comes from the engine,
not from a formula in the shim — `getTimeOnAir()` must agree with what the
channel actually charges, or the firmware's CSMA desynchronises from the
simulation silently.

*Test:* a two-node flood behaves exactly as it does today. This step should
change no result at all; if it does, the new path is wrong.

### Step 3 — the IRQ surface, which is the actual product

Model the flags that matter: `PREAMBLE_DETECTED`, `HEADER_VALID`, `RX_DONE`,
`CRC_ERR`, `TX_DONE`, `TIMEOUT`, and CAD's `DONE`/`DETECTED`. Set them from the
engine's view of what is on the air at this node, at the simulated instant it
becomes true — a preamble detected some symbols into a transmission, not at the
end of it.

Then delete `HostRadio::isReceiving()`: the answer stops being ours and starts
being MeshCore's, computed by its own driver from flags we present.

*Test, and it is the whole reason for the exercise:* **1.16.0 and 1.17.0 stop
producing identical ledgers.** Add a per-node counter of deferrals and CAD
timeouts so the mechanism is visible rather than inferred, and re-run the sweep
that produced twelve identical rows.

### Step 4 — the other chips, if wanted

`CustomSX1268`, `CustomSTM32WLx`, `CustomLR2021` share the wrapper. Once the
SX1262 path works the others are mostly parameterisation, and they matter only
if we want to ask whether a change behaves differently per radio — which is a
real question, since 1.17 touched all four.

## What could go wrong

- **RadioLib's classes are concrete, not virtual.** Ours must be
  source-compatible, not merely similar: same names, same signatures, same
  return types. Divergence shows up as a compile error, which is the good case,
  or as a subtly different return value, which is not.
- **Timing is the hard part, not the API.** A preamble IRQ that fires at the
  wrong simulated instant produces behaviour that looks like a firmware
  difference and is ours. Step 2's "this must change nothing" test exists to
  catch that before step 3 muddies it.
- **We would own more of MeshCore's assumptions.** Today the shim is small
  enough to hold in your head. This roughly triples it, and every upstream
  change to the driver layer becomes something that can break our build. That is
  the real cost, and it is recurring.
- **It may still not move the numbers.** If MeshCore's listen-before-talk turns
  out to be dominated by behaviour our channel does not model, the arms could
  stay identical for a different reason. Step 3's test is designed to tell us
  that plainly rather than leave it ambiguous.

## A related principle, while it is in mind

Measurement belongs in the bench, not in the shim.

A companion hears its own message relayed back by a neighbour. That is real, and
it is *useful* to the firmware — a hear-back is how MeshCore knows its flood
propagated, and the companion listens for exactly that. So the shim must not
suppress it, and neither must the firmware: nothing about the simulated radio
should be bent to make a statistic come out tidily.

What was wrong was counting it as a delivery, which put companion delivery at
105.88% — more than the number of other companions there are. That is a
reporting decision and it is fixed where reporting happens: `measure()` skips
the originating node when counting who a message reached.

The general rule this is an instance of: **the simulator reproduces what
happens; the bench decides what counts.** Any time those two get mixed up, the
result is a number that cannot be checked against reality, because reality is no
longer what produced it.

There is one more edge to tighten here, and it belongs in the bench too. An `rx`
event means *demodulated and handed to the firmware* — not *accepted by the
application*. MeshCore may still drop it as a duplicate, which the engine
records as a separate outcome. "Delivery" today therefore means "arrived and
decoded". Counting only what the firmware kept would be a truer measure of
whether a message got through, and is a small change to `measure()` once the
outcome is carried through.

## Why bother

The tool's claim is that it runs real firmware. It does — but at the moment it
runs real firmware with our radio driver bolted underneath, and the last two
studies have both foundered on exactly that seam. Step 3 is the point at which
"we run the firmware" becomes true of the part of the firmware that decides when
to transmit, which is the part every interesting question so far has been about.
