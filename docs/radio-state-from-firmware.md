> **Working note, last true on 14 August 2026.** Kept for the thinking in it, not maintained as a description of the code. **2 of the 2 package paths it names no longer exist**, the seven-layer restructure of 19 August having moved them. Where this disagrees with the tree, the tree is right; the authority is the code in `internal/mesh/shim/`.

# Radio state comes from the firmware, not the board profile

Two bugs from MeshCore 1.17.1, chosen because they are the two MeshBench should
be able to see and currently cannot:

- **Receive gain** (#3158) — boosted gain reverted to the compiled default after
  an AGC reset. A node that asked for boosted gain silently lost roughly 2 dB of
  sensitivity. Nothing crashed, nothing logged.
- **Transmit enable** (#3188) — `PIN_SPI1_MISO` was `-1` on the Heltec T096; the
  out-of-bounds access clobbered the front-end module's transmit-enable line, so
  the power amplifier never turned on.

The wider survey is in `docs/firmware-bug-detection-plan.md`. This document plans
only these two, because they turn out to be the same bug.

## They are the same bug

`internal/scenario/boards.go` carries a node's RF parameters as static constants
traced to datasheets:

```go
MaxTxDBm:       22,
SensitivityDBm: -137,
NoiseFigureDB:  6,
```

Bug #3 is firmware failing to deliver the sensitivity. Bug #5 is firmware failing
to deliver the transmit power. **Both are static figures the running firmware can
silently invalidate, and in both cases we keep simulating the datasheet.**

`Station_G2` shows how far that reaches: `MaxTxDBm: 30` with a note that it has an
external PA. That 30 dBm is asserted unconditionally. If its firmware failed to
enable the PA — exactly the T096 bug on a different board — we would still hand
someone a coverage map built on 30 dBm.

**The one idea: board profile figures become the *default*, and the firmware can
override them.** Applied twice.

This is the same principle `CLAUDE.md` already states for the channel — *"the
channel does not decide anything… capture effect must emerge"*. We do not encode
either bug. We model the chip and the board correctly, and the firmware's failure
to drive them becomes visible on its own.

## The seam that is missing

Today the firmware tells us both of these things and we discard both.

```
firmware → SPI  → VirtualSX1262 → bridge → RF engine
firmware → GPIO → (nothing)                    ↑
                                     static figures from boards.go
```

`docs/virtual-sx1262.md` files `WRITE_REGISTER`, `SET_TX_PARAMS` and
`SET_DIO3_AS_TCXO_CTRL` under *"Configuration, answered by storing the value"*.
Stored, never read. The receive gain register arrives over SPI on every run.

What is needed:

```
firmware → SPI  → VirtualSX1262 ─┐
firmware → GPIO → FEM enable ────┴→ radio state → bridge → RF engine
                                                             ↑
                                              boards.go figures as defaults
```

One state object, owned by `VirtualSX1262`, reported across the existing bridge.

**Not three implementations.** The FEM enable is a GPIO, and the temptation is to
watch it separately in Renode, QEMU and the native build. `RadioServerSX1262.cs`
already records why that is wrong — a second model of one chip was deleted because
two implementations "have to agree for ever. They do not agree for ever."
`RadioServerSX1262` is already an `IGPIOReceiver` for chip select; it takes the
FEM pin the same way and forwards it. One owner, three ways in, as now.

---

## W1 — Radio state across the bridge

**Shared foundation. Both bugs need it; neither is possible without it.**

Define the state `VirtualSX1262` maintains and reports:

| Field | Source | Consumes |
|---|---|---|
| `RxBoostedGain` | register `0x08AC` | noise figure |
| `TxPowerDBm` | `SET_TX_PARAMS` | transmit power |
| `FemEnabled` | GPIO from the board's FEM pin | transmit power |

Then change the engine so effective parameters are computed as *board default
overridden by reported state*, rather than read straight off the profile.

- Register and pin semantics in `MeshBench/meshcore-native` (C++).
- Effective-parameter computation here, in the engine and `internal/dsp`.
- A new bridge protocol tag alongside the existing five (`CsAssert`, `CsRelease`,
  `Xfer`, `ReadBusy`, `ReadIrq`).

**Check first:** whether the engine currently takes transmit power from the
scenario in Go or from the chip. If `SET_TX_PARAMS` is already plumbed through,
W1 is smaller than it looks.

## W2 — Receive gain becomes noise figure (bug #3)

Model register `0x08AC`, **and model what clears it.**

The clearing is the whole job. An AGC reset clearing the gain register is what
real silicon does — datasheet behaviour, not a defect. The firmware's defect is
failing to re-apply afterwards. Model the chip faithfully and the bug appears
with no knowledge of it anywhere in our code. Model the register but not its
reset, and both firmware versions look identical and we learn nothing.

Then map gain mode to a noise-figure delta, so `NoiseFigureDB` from the board is
the power-saving default and boosted gain improves on it.

## W3 — FEM gates transmit power (bug #5)

Add a front-end module to the board profile: which GPIO enables transmit, and what
it contributes when asserted.

Route that pin into `RadioServerSX1262` as a second `IGPIOReceiver` line, exactly
as chip select already arrives. When the line is not asserted, the node transmits
at bare chip power rather than the profile's `MaxTxDBm` — or not at all, depending
on the part.

`Station_G2` gets the same treatment, because its 30 dBm has the same problem.

**Prerequisite: the T096 does not exist in `boards.go`.** It ships `.uf2`, so it is
nRF52 and runs the Renode path that already works for the RAK4631 — no ESP32-S3
blocker. But it needs a profile, Renode wiring (SPI base, NSS, IRQ), its FEM pin,
and a first boot. That is the RAK4631 exercise repeated, and it is the long pole
on this bug.

## W4 — Variant consistency check (bug #5, fast path)

**Independent of W1–W3. Cheapest reliable catch, and it needs no emulation.**

`PIN_SPI1_MISO = -1` is visible in the T096's variant header without running
anything. But a naive range check is the wrong tool: `-1` is idiomatic Arduino for
"not connected", so flagging every one of them buries the signal.

The narrower invariant holds:

> if a peripheral is declared **and initialised**, every pin it needs must be valid

SPI1 being brought up with `MISO = -1` is contradictory on its own terms whatever
`-1` means elsewhere. That runs statically over all 97 published boards, including
every one we cannot boot.

W4 and W3 are complementary, not alternatives. W4 catches *the pin is
contradictory*; W3 catches *the pin is fine and the firmware never drove it*.

---

## Tests

Two levels each, because the cheap level catches the bug even when the expensive
one is fiddly to tune.

### Level 1 — assertions on chip and pin state

Fast, deterministic, no RF margin to tune.

- **#3:** after an AGC reset, the gain register holds what the firmware last
  asked for.
- **#5:** the FEM enable line is asserted before a transmission begins.

The #5 assertion is the strongest single test in this plan. It is
board-independent, needs no margin tuning, and it would have caught the T096 bug
directly.

### Level 2 — link at the margin

Proves the coupling reaches the air, which Level 1 does not.

- **#3:** two nodes, pinned seed, path loss set so received SNR sits between the
  boosted and power-saving thresholds. 1.17.1 decodes across an AGC reset;
  1.17.0 goes deaf.
- **#5:** a link that closes only with the FEM's gain. A T096 on 1.17.0 fails it.

Both are attributable only because determinism is a feature — same seed, same
scenario, same result. Any difference between the two firmware versions *is* the
firmware.

---

## Sequencing

**#3 can start now. #5 cannot.** Bug #3 needs a board that already boots, and the
RAK4631 is verified. Bug #5 needs the T096 brought up first.

| Step | Work | Estimate |
|---|---|---|
| 0 | Resolve the unknowns below | Days |
| 1 | W4 variant consistency check | ~1 week, independent |
| 2 | W1 radio state across the bridge | 2–3 weeks, cross-repo |
| 3 | W2 gain → noise figure, Levels 1 and 2 | ~2 weeks |
| 4 | T096 profile, wiring and first boot | 1–2 weeks |
| 5 | W3 FEM → transmit power, Levels 1 and 2 | ~2 weeks |

Roughly **7–9 weeks**, with the first catch landing at step 3 and a cheap partial
catch at step 1.

Run W4 first regardless. It is a week, it is independent of the cross-repo work,
and it covers all 97 boards rather than the two we can boot.

## Unknowns to resolve before step 2

These change the estimate, and three of them are datasheet questions that must not
be answered from memory:

1. **Register `0x08AC`, its boosted and power-saving values, and the sensitivity
   delta.** Roughly 2 dB is the figure quoted, but read it off the SX1262
   datasheet. If the reset condition is modelled wrong the test passes on both
   versions and teaches nothing.
2. **What provokes the AGC reset**, and whether it can be triggered
   deterministically from a scenario.
3. **Is boosted gain runtime-configurable in MeshCore, and what is the compiled
   default?** The bug only bites where the runtime setting differs from the
   compiled one — if they match on our test board, nothing happens.
4. **Does the RAK4631's firmware use boosted gain at all?** If not, step 3 needs a
   different board and inherits step 4's bring-up cost.
5. **Does the engine already take transmit power from `SET_TX_PARAMS`,** or from
   the Go-side scenario config?
6. **The T096's FEM pin**, from its variant header — which W4's parser will be
   reading anyway.

## Afterwards

Once board figures are firmware-overridable, `docs/shortcomings.md` needs an
entry: MeshBench previously simulated datasheet transmit power and sensitivity
regardless of what the firmware programmed, and any result produced before this
landed assumed the firmware configured its radio correctly.

That is a case of the simulator being kinder than the air, and `CLAUDE.md` says we
say so.
