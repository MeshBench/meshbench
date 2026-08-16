# Post-release firmware QA: the board sweep

MeshCore 1.17.1 fixed seven bugs. The question this started from was whether
MeshBench would have caught them. The useful version of that question turned out
to be different, and it is not about catching MeshCore's bugs before they ship.

**We consume releases, we do not gate them.** By the time a release exists the
bugs are already in it, and the thing worth building answers a different
question: *when a release lands, what is actually safe to run, and what changed?*

That is a product MeshBench is uniquely placed to build and nobody else can. It
subsumes most of what this document originally proposed and deletes one whole
workstream.

---

## What upstream actually publishes

Measured, not assumed:

- **Three role tags per version** — `repeater-v1.17.1`, `companion-v1.17.1`,
  `room-server-v1.17.1`. `board_catalogue.go` already knows this; `List` is per
  tag "because tags are per role upstream".
- **97 boards** in both 1.17.0 and 1.17.1.
- `EmulationVerified` contains **2** of them.

### The unit of analysis is (board, role), not board

This cost two wrong answers before it came out right, so it is recorded here.

Bug 2 was an R1 Neo and Minewsemi build failure. Diffing the **board** sets
between 1.17.0 and 1.17.1 shows 97 against 97, nothing added, nothing removed —
the bug is invisible. Diffing **(board, role)** pairs:

| | 1.17.0 | 1.17.1 |
|---|---|---|
| R1Neo / Minewsemi, `repeater` | **0 assets** | **4 assets** |
| the same boards, `companion` | 8 | 8 |
| the same boards, `room-server` | 4 | 4 |

The repeater role failed to build; the other two were fine, so the boards never
vanished from the catalogue. Aggregate by board and the signal is gone.

**Consequence: a build failure is detectable from the published catalogue alone,
for all 97 boards, with no compilation and no emulation.** The original plan
costed a PlatformIO board matrix at 2–3 weeks to catch this. It is hours of work
against `BoardCatalogue.List`, which already exists and already parses these
names. That workstream is deleted.

---

## The sweep

Pull every board image for a version, run each through a fixed battery, report
what changed since the version before. Four tiers, and the first is available
now.

### Tier 0 — Catalogue conformance

**All 97 boards. No emulation. Hours to build.**

For a version, enumerate (board, role) pairs across the three tags. Diff against
the previous version. Report:

- pairs that **vanished** — a role that stopped building, which is bug 2
- pairs that **appeared** — new hardware, or a fix
- assets that fail `ParseAssetName` — upstream naming drift

That last one is not hypothetical. Asset names are inconsistent about case:
`ThinkNode_M2_Repeater` against `Heltec_v3_repeater`. Our `assetPattern` carries
`(?i)` and copes, but a check that reports unparseable names will catch the day
that stops being enough.

This tier is worth building first regardless of everything below it. It is the
cheapest useful thing in the document.

### Tier 1 — Boot sweep

**Gated on board coverage. Currently 2 boards.**

For each (board, role) we can emulate: download, boot, and assert the node gets
as far as driving its radio — chip version read, LoRa mode set, modulation
configured, an advert on the channel. That is the sequence `boards.go` already
records for the RAK4631 and the E22, turned into a test rather than a note.

Failure here is a board that ships firmware which does not come up. That is bugs
1, 5, 6 and 7 — pin tables, TCXO voltages and out-of-range constants all end in a
node that will not initialise its radio.

### Tier 2 — Behavioural battery

**Needs C1 for the sensitivity row. The rest can start earlier.**

One reference scenario, pinned seed, everything held constant except the board
under test. The user's phrase for this was "one at a time", and that is the right
design: the board is the variable, the network is the control.

| Check | Why it belongs here |
|---|---|
| **Airtime conformance** | `CLAUDE.md`: airtime must match the firmware's own `getEstAirtimeFor()`, because CSMA is built on it and "if our channel disagrees, the two desynchronise silently". That is a per-release, per-board claim and nothing currently tests it. |
| **Receive margin** | The C1 work. Catches bug 3 — gain silently reverting after an AGC reset. |
| **Flood and ack** | A packet through a three-node chain, both directions. Reachability is asymmetric, so both. |
| **CSMA timing** | Falls out of the airtime work. |
| **Duty cycle** | Energy behaviour over a simulated period. |

Airtime conformance is the one to build first. It is a `CLAUDE.md` domain rule
with no test behind it, it needs no new chip modelling, and it is exactly the kind
of silent divergence that a release can introduce without anyone noticing.

### Tier 3 — The release diff report

**The actual product.**

Run tiers 1 and 2 against version N and N−1 and report per-board deltas. Three
questions it answers that nothing else does:

1. *Is this release safe for my network?* — before flashing forty repeaters.
2. *What changed since the version I run?*
3. *Why did my network get worse after I upgraded?*

The third is the one with no other answer available anywhere.

**This only works because determinism is a feature.** `CLAUDE.md`: same seed,
same scenario, same result, counter-based RNG. A cross-version diff is
attributable precisely because nothing else can move. Any behavioural difference
between two runs *is* the firmware. Lose determinism and the entire report
becomes noise.

---

## The workstreams, as components of the sweep

Reframed from the original document. They are no longer four parallel ideas; they
are what the sweep needs.

### C1 — receive gain couples to noise figure

**The highest-value item, and post-release framing raises it further.**

Behavioural diffing is the product, and C1 is what makes radio configuration
visible at all. `docs/virtual-sx1262.md` opens on the failure this fixes: "two
firmware versions differing only in their radio driver produced byte-identical
results". Tier 3 is worthless for radio-config changes until this exists.

It also fixes a live honesty defect. `boards.go` asserts `SensitivityDBm: -137`
as a datasheet constant; bug 3 is a case where the firmware does not deliver it.
Board figures become the **default** the firmware can override, not the truth.

**Cross-repo.** `VirtualSX1262`, the bridge and `radioserver` live in
`MeshBench/meshcore-native`. Register semantics land there in C++, the noise
figure coupling lands here in Go, and they meet across the bridge protocol. Scope
it as two repos from the start.

### D — variant header parser

**The scaling mechanism, not just a correctness check.**

Board wiring is hand-read today; every `QEMUWiring` comment says so. A parser
generates `QEMUWiring`/`RenodeWiring` mechanically for all 97 boards instead of
by hand, which is what makes Tier 1 more than a two-board sweep.

It also cross-checks the nine entries already in the table, which have not been
re-read since they were entered and may already be stale. A stale NSS pin
produces "chip not found", which `boards.go` says three separate times "reads as
a broken emulator rather than a wrong number".

Normalise the numbering: the `RenodeWiring` comment documents the flat-versus-port
trap already.

### B — pin constant validation

**Shares D's parser. Nearly free once D exists.**

Assert every pin constant is within `[0, N_PINS)` for its MCU, reporting `-1`
sentinels separately from out-of-range values. Bug 5 is a `-1` used where an
index was expected.

Second half, worth more than the first: **make the emulator fault on invalid pin
access** rather than absorbing it. `RadioServerSX1262.cs:127` already argues this
position for bytes — "a silent zero here is the hardest kind of fault to find".
Same principle, applied to pins.

### A — board matrix build

**Deleted.** Superseded by Tier 0 at a fraction of the cost. We consume published
binaries; whether they compile is upstream's problem, and their absence is
already visible in the catalogue.

### Declined — display verification

Bug 4, the T-Beam Supreme S3 display. Reachable via a modelled SSD1306 and a
golden framebuffer, declined because nothing about MeshBench's purpose improves
when it can see a display. If a display model arrives for another reason this
falls out nearly free; it should not be the reason.

---

## Sequencing

| Phase | Work | Estimate |
|---|---|---|
| 0 | **Tier 0 catalogue conformance** | Hours to days |
| 1 | D's parser → B validation → generated wiring | 3–4 weeks |
| 2 | Tier 1 boot sweep + verification harness | 4–6 weeks |
| 3 | Tier 2 airtime conformance | 2–3 weeks |
| 3′ | C1, cross-repo, parallel | 6–8 weeks |
| 4 | Tier 3 release diff report | 2–3 weeks after 2 and 3 |
| 5 | Board coverage big rocks | quarters |

Phase 0 stands alone and should be built this week.

### Phase 5, itemised

| Blocker | Unlocks | Estimate | Risk |
|---|---|---|---|
| ESP32-S3 general-purpose SPI in QEMU | Heltec V3, Mesh Solar, Xiao S3, Station G2, T-Beam Supreme S3 | 1–3 months | High |
| SX1268 model | Band variants of supported boards | Days — close kin to the SX1262 | Low |
| SX1276/SX1278 model | Heltec V2 and the older fleet | 3–6 weeks — register-based, different interface | Medium |
| LR1110/LR1121 | Newest boards | Months | High |
| ESP32-C3/C6 | RISC-V boards | Separate from S3 | High |

**nRF52 is not a technical blocker.** Published images "need a Nordic
SoftDevice", which is a redistribution problem, not an engineering one — the
RAK4631 already boots with one. It needs a supply-your-own-blob flow, days rather
than months.

**Estimate to useful coverage: one quarter, plus the S3 month.** To all 97
boards: two to three quarters, with a tail that may never justify the wiring. The
right outcome for that tail is `EmulatableBoards()` giving a specific reason,
which it is already designed to do.

---

## What this still would not catch

- **Anything analog.** The TCXO check tests a configuration value against a spec,
  not an oscillator. `docs/shortcomings.md` §1.2 already says we model no
  oscillator error at all.
- **Anything needing real hardware.** Six of the seven bugs are ultimately about a
  physical board. The honest ceiling is that we check firmware against a
  *description* of hardware, and a wrong description passes. This belongs in
  `docs/shortcomings.md` once any of this lands.
- **ESP32-S3, until QEMU grows a general-purpose SPI controller.** Only bug 4
  (T-Beam Supreme S3) is behind that wall. Measured from the published assets:
  T096, T-Echo, T-Echo Lite, T-Echo Card, Minewsemi and R1Neo all ship `.uf2`,
  so they are nRF52 and run on the Renode path that already works.

`tools/esp32/README.md` is stale and will mislead anyone estimating from it: it
calls a modelled SX1262 "the single remaining blocker for the emulated backend on
both architectures", but that was cleared for plain ESP32 when
`Generic_E22_sx1262` was verified. Fix it before it costs someone a day.

## The ADR question

`CLAUDE.md` forbids reintroducing a dependency on MeshCore without a new ADR.
Deleting workstream A removes the need to build MeshCore in CI, which was the
sharpest form of that question. D still needs variant headers as **reference
data** — small files, parsed, not linked and not built.

That is a weaker claim than the original and probably still deserves the ADR's
one paragraph, recording that reading a header is neither linkage nor a build
dependency, and that ADR-0001 stands untouched.

## Work items

Not yet in Plane; assign MSIM numbers when raised.

| Item | Tier / stream | Size |
|---|---|---|
| Catalogue conformance diff, (board, role) | Tier 0 | Hours–days |
| ADR: variant headers as reference data | gate | Hours |
| Variant header parser | D | Weeks |
| Pin constant range validation | B | Days, after the parser |
| Emulator faults on invalid pin access | B | Days |
| Generated wiring for the full board set | D | Weeks |
| Boot sweep harness | Tier 1 | Weeks |
| Airtime conformance battery | Tier 2 | 2–3 weeks |
| Rx gain register couples to noise figure | C1 | 6–8 weeks, cross-repo |
| TCXO voltage as a startup condition | C2 | Days, after C1 |
| Release diff report | Tier 3 | 2–3 weeks |
