# Why the bench saw nothing, and how to make it see

The A/B of MeshCore 1.17.0 against 1.17.1 — 161 real nodes, four arms, two
seeds, eight cells, none failed — returned no difference that can be quoted.
That was reported honestly and it is still true. What follows is the reason,
which turns out not to be one reason, and what to do about each part.

Three of the five causes are confirmed from source rather than suspected. Two
of those mean the sweep **could not** have shown anything, whatever the
firmware did: the mechanism was never switched on, and the code path was never
present. Aggregate delivery counts being insensitive to 2 dB — the explanation
I reached for first — is the least established of the five and comes last here
for that reason.

## What we were trying to see

Two faults, each of a different kind.

**Receive gain reverting.** MeshCore reimplements RadioLib's AGC reset in
`src/helpers/radiolib/SX126xReset.h` and re-applies the compile-time boosted
gain macro, discarding whatever the operator set at runtime. The chip drops to
power-saving gain while `_prefs.rx_boosted_gain` and the CLI `get` keep
reporting the operator's value. Worth roughly 2 dB of receive sensitivity, with
nothing in any log to say it happened.

**A transmit-enable line that never went high.** On the Heltec T096 the front
end stayed switched out, so the PA's output went through the module's isolation
instead of its gain — a swing of tens of dB at the antenna.

## Cause 1 — the fault was never armed

**Confirmed.** `src/helpers/CommonCLI.h:66`:

```cpp
uint8_t rx_boosted_gain = 0; // power settings
```

Boosted gain is **off by default**. The bug destroys a *runtime* setting; on a
node that never made one there is nothing to destroy.

We knew AGC resets were off by default and provisioned `set agc.reset.interval 4`
to switch them on. We never sent `set radio.rxgain on`. So every node in every
arm ran the whole sweep in power-saving gain, the reset fired every four seconds
against a register already at its reset value, and `effectiveRF` correctly
returned the same noise figure for all 161 nodes in all four arms.

The arms were genuinely identical. The bench reported that accurately.

There is a second half, and it is the sharper one. `SX126xReset.h:28-30`:

```cpp
#ifdef SX126X_RX_BOOSTED_GAIN
  radio->setRxBoostedGainMode(SX126X_RX_BOOSTED_GAIN);
#endif
```

The host variant does not define that macro, so on a native build the
re-application is not merely wrong — **it is not compiled at all**. That is the
worse of the fault's two modes, the one where boosted gain is gone until
reboot. The native nodes were the right vehicle for it. They just had nothing
loaded.

### What to do

Send `set radio.rxgain on` alongside `set agc.reset.interval 4`, and then
**assert the mechanism before measuring its effect**. W1 already carries the
gain register across the bridge and the Radio tab already draws it; a
precondition check reads it back off a sample of nodes after provisioning and
fails the cell loudly if the register is not `0x96` where the arm says it
should be.

This generalises past this bug. It is wb1's `investigate` check — "read the
setting back off a node" — promoted from a post-hoc explanation of a null
result to a precondition of running at all. An arm that cannot demonstrate its
own manipulation should not produce a row.

**We had already built the instrument that would have shown this in one
glance** — 161 nodes reporting `0x94` in both arms — and never pointed it at
the running sweep. That is the cheapest lesson in this document.

## Cause 2 — no node in the sweep had a front end

**Confirmed.** Every node in `fixture-scotland-strict.json` is a `RAK4631`.
Of the ten board profiles in `internal/scenario/boards.go` exactly two carry a
`FEM`: `Generic_E22_sx1262` and `Heltec_t096`. `n.Spec.FEM` was `nil` for all
161 nodes, so the entire W3 branch of `effectiveRF` was dead code for the
duration.

Worse, the fault's own board is out of reach. The T096 is nRF52840, so it needs
Renode; the emulated path is QEMU, and of the boards shipping `.uf2` images
only T-Beam Supreme S3 is ESP32-S3. **The T096 transmit fault is not currently
reachable by any bench configuration.**

### What to do

Two steps, and the first is cheap. `Generic_E22_sx1262` is ESP32, has a FEM
with a modelled 25 dB switch isolation, and runs under QEMU today — a mixed
fixture with a handful of E22 nodes exercises W3 for the first time. The
observable is not mesh delivery but the node's own reported transmit power:
a node whose FEM never switched in should read 25 dB down, per direction, and
the Radio tab shows it.

The second is the Renode path for nRF52, which is a real piece of work and
should be scoped separately. Until it exists, say so — `docs/shortcomings.md`
should record that the T096 class of fault is out of reach, rather than leaving
a reader to assume the board matrix covers it.

## Cause 3 — the design was observational, and it need not be

Comparing two releases asks a noisy aggregate to resolve one mechanism while
six other fixes move underneath it. Even a perfect measurement would not have
attributed the result.

The simulator makes a much stronger design available and we did not use it.
**Determinism is a feature here**: same seed, same scenario, same result. So
run the *same channel realisation* and change only the receiver's noise figure.
Every waveform, every arrival time, every collision is identical by
construction; any reception that flips is caused by the 2 dB and nothing else.

That is a paired, within-subject comparison, and it has enormously more
statistical power than two independent runs differenced against each other. It
also needs no repeats to establish significance, because there is no
between-run variance left to overcome.

### What to do

A `paired` experiment mode: one run, two receiver configurations, per-link
outcomes compared. The metric is the count of receptions that flipped and which
links they were on — not a percentage change in a total.

## Cause 4 — measured, and it is not what I assumed

The hypothesis here was that a country-sized flood delivers through so many
redundant paths that no link sits near enough to threshold for 2 dB to matter,
and that the zero seed spread was the same property showing itself.

**That has now been measured, and it is wrong.** `Engine.LinkMargins` computes,
for every ordered pair the link cache has a path for, how far above its
demodulator's floor the signal arrives — the same arithmetic `deliver` performs
per transmission, over the cache instead of over a packet in flight.

| | directions with a path | decoding | median margin | within 2 dB | within 1 dB |
|---|---|---|---|---|---|
| Scotland (161 nodes) | 25 600 | 1 661 | 18.2 dB | **101 (6.08 %)** | 40 (2.41 %) |
| Fife (58 nodes) | 3 249 | 565 | 19.1 dB | **38 (6.73 %)** | 21 (3.72 %) |

The distribution is not the bimodal wall of certainties I described. Scotland's
tenth percentile is 4.1 dB and its lowest decoding direction is 0.1 dB above the
floor: there is a real population living at the edge, and **about one decoding
direction in sixteen could be flipped by 2 dB**.

So the fixture can express this effect. Causes 1 and 2 already explain the null
result on their own, and this one does not need to be invoked.

**Fife settles it.** Fife has proportionally *more* near-threshold links than
Scotland, and it was the fixture where all four arms returned byte-identical
figures across two seeds. If marginal links were what the seed acts on, Fife
should have been the noisiest of the two. The claim that zero seed spread and
insensitivity to receiver gain are "the same property" is therefore refuted —
and so is my use of it to close the open question in `docs/bench-parity.md`.
That open is still open.

What does survive is narrower and more useful: the marginal links exist, but
**aggregate delivery counts destroy them**. Flipping 101 of 1 661 directions
need not change which nodes ultimately receive a flood, because the flood
reaches them by other edges. The signal is present at link level and is thrown
away by summing.

### What to do

That points somewhere different from where I first pointed it. The fixture does
not need replacing and the sweep does not need a precondition that refuses to
run on it — **the metric needs replacing.** Count per-link outcomes and the
directions that flipped, not totals over a mesh. Cause 3's paired design is the
way to do it, and it is now the highest-value item here rather than the third.

Report the margin distribution in the Bench anyway, because it says what a given
scenario can be asked and none of us knew the number until now. But report it as
context, not as a gate.

## Cause 5 — the figure we would be measuring is a placeholder

`RxBoostedGainImprovementDB = 2.0` is not traceable to any source, and
RadioLib's maintainer left the feature unimplemented for exactly that reason.
Any result quoted from a sweep that turns on it is partly a result about our
constant.

### What to do

Sweep it. Run the mechanism across a plausible range — 0.5, 1, 2, 3 dB — and
report the effect as a function of the assumption. A result that says "below
1.5 dB this is invisible on this mesh, above 2.5 dB it costs 4 % of receptions"
is more useful and more honest than a single number resting on a guess, and it
needs no hardware. A bench measurement against a real SX1262 remains the way to
close it properly.

## What to do, in order

Reordered once cause 4 was measured rather than assumed. Steps 1 to 3 are built;
what each one turned up is noted against it.

1. **Arm the mechanism and assert it landed.** *Done.* Where an arm pins a
   setting the chip reports, a sample of radios is read back and the cell fails
   if they do not hold it. Wiring it in failed every cell in the suite at first,
   which was itself informative: nodes attach and report nothing because the
   installed release binaries predate the extended radio payload. So it only
   speaks when the arm pinned something observable, and that false failure is
   now its own regression test.
2. **Count what a decibel was worth.** *Done, and not as planned.* The design
   called for two runs of one channel realisation with the receiver varied. That
   is unnecessary: every reception already computes its margin against the
   demodulator's floor, so a single run can count exactly how many decodes would
   have been lost had the receiver been worse, and how many misses would have
   decoded had it been better. Evaluated analytically, it cannot diverge — there
   is only one run — and it needs no assumption about what boosted gain is worth,
   because it reports by margin band rather than at one figure.
3. **Report the margin distribution** as context on what a scenario can be
   asked. `Engine.LinkMargins` computes it; the Bench display is left.
4. **A mixed fixture with E22 nodes**, so W3 is exercised at all. The T096 gap
   is now recorded in `docs/shortcomings.md` §3.4 rather than implied away: that
   board is nRF52 and needs Renode, so the fault it is named for cannot be
   studied here at all, and no shipping fixture contains a node with a front end.
5. **Quote the effect as a function of `RxBoostedGainImprovementDB`.** *Done, by
   measurement rather than by sweeping it.* Step 2's bands report the share of a
   run's decodes at risk at 1, 2, 3, 6 and 10 dB, so the constant does not have
   to be believed to read the answer. On Scotland, measured:

   | if the receiver loses | decodes at risk |
   |---|---|
   | 1 dB | 5.9 % |
   | 2 dB | 12.4 % |
   | 3 dB | 19.3 % |
   | 6 dB | 36 % |
   | 10 dB | 57 % |

   About six per cent of decodes per decibel through the range that matters. A
   sweep of the constant would have cost four times the runs to say the same
   thing less precisely.
6. **Port wb1's `investigate` checks**, of which step 1 is the most valuable
   made mandatory.

Steps 1 and 2 together would have changed the last sweep's outcome from "no
difference" to a measured count of deliveries that hung on a decibel — an answer
either way, and one no amount of repeating the old design could have produced.

## The thing worth keeping

The bench was not wrong. It ran eight cells, reported no difference, and
refused to call it a result — which was correct, because there was no
difference to find in what it was actually running. Every fault above is in
what we asked it to measure, not in its measuring.

The gap the whole exercise exposes is that **an experiment can be executed
perfectly and still be void**, and nothing in the apparatus checked the
manipulation was real before spending an hour on the outcome. That check is
step 1, and it is worth more than either of the two bugs that prompted it.
