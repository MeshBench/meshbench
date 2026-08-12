# Eight protocol ideas, pre-registered

Read from MeshCore at `727fc05` (the commit `repeater-v1.17.0`,
`companion-v1.17.0` and `room-server-v1.17.0` all point at). Every idea below
comes from the code rather than from imagination, and each names the line that
prompted it.

Written **before** running anything. Metrics, seeds and arms are fixed here so
the result cannot be selected afterwards.

## Why pre-register at all

The previous study on this simulator found two things that would have been
embarrassing to discover late: `Mesh::allowPacketForward()` only ever returns
`false`, so nothing about loop detection can *increase* forwarding; and the loop
thresholds are indexed by path-hash size, so at a three-byte hash all three
settings are `1` and are **identical by construction**. Arms that vary nothing
are worth knowing about before spending a night on them.

It also established the measurement floor: with around eight simultaneous
senders, outcomes move by **±20%** between runs of the same configuration. Any
delta smaller than that is noise, and every number in the report is quoted
against it.

## The ideas

### 1. Cancel a queued relay when someone else relays it first

**Where it comes from.** `Dispatcher::calcRxDelay()` gives a better-scoring node
a shorter delay, so the best-placed relay transmits first. Nothing acts on that.
`_tables->markSeen()` stops a *second* copy being queued, but there is **no
cancellation path anywhere** in `Mesh.cpp`, `Dispatcher.cpp` or the repeater —
`removeOutboundByIdx()` exists on the packet manager and is never called for
this. A node that has already queued its retransmit still transmits it after
hearing the packet relayed by a neighbour.

**Mechanism.** On receiving a flood packet whose hash matches one already queued
outbound, drop the queued copy.

**Hypothesis.** Total transmissions and duplicate receptions fall substantially,
with delivery unchanged. This is the single largest airtime saving available.

**Metric.** Transmissions per message; duplicate receptions per node; delivery
ratio as the control.

**Cost.** One hash comparison per received flood packet against a queue of at
most a few entries.

### 2. A retransmit delay that cannot be zero

**Where.** `MyMesh::getRetransmitDelay()` returns
`getRNG()->nextInt(0, 5*t + 1)` — the lower bound is **zero**, so two relays can
both pick "immediately" and collide.

**Mechanism.** `nextInt(1, 5*t + 1)`.

**Hypothesis.** A small reduction in collisions. Included partly as a **sanity
arm**: it is a one-token change with a mechanically obvious effect, so if it
shows a large delta the harness is suspect.

**Metric.** Collisions and duplicate receptions.

**Cost.** None.

### 3. Loop detection that sees cycles, not just itself

**Where.** `MyMesh::isLooped()` counts how many times **this node's own hash**
appears in the path. A cycle that does not include the node checking is
invisible to it.

**Mechanism.** Also reject when any hash appears more than once in the path.

**Hypothesis.** Little or no effect, because the earlier study found **0 of 8270
observed paths contained a repeat**. Included deliberately as a **negative
control**: if it moves the numbers, the geometry or the harness is generating
loops that the real network does not.

**Metric.** Relays suppressed; delivery ratio.

**Cost.** O(n²) over at most 63 entries, on a path that is usually under five.

### 4. Density-aware relay backoff

**Where.** `calcRxDelay()` is `pow(rx_delay_base, 0.85 - score) - 1` times
airtime. It knows the packet's score and nothing about how many neighbours are
about to make the same decision.

**Mechanism.** Widen the delay window in proportion to the number of distinct
neighbours heard recently.

**Hypothesis.** Fewer collisions in dense clusters at the cost of latency in
sparse ones. This is the idea most likely to help in one place and hurt in
another, which is why the small and medium fixtures are both run.

**Metric.** Collisions; end-to-end latency; delivery.

**Cost.** A small neighbour table with ageing.

### 5. Region-aware duty budget

**Where.** `Dispatcher::getAirtimeBudgetFactor()` returns `1.0`, giving
`1/(1+1)` — a flat **50% duty cycle**, hardcoded, everywhere. EU 868 sub-bands
are commonly 1% or 10%.

**Mechanism.** Derive the factor from the configured region.

**Hypothesis.** Delivery falls and airtime falls sharply. The interesting output
is not whether it helps but **what the current default costs in legality terms**,
which is a fact worth publishing regardless of the delta.

**Metric.** Airtime per node per hour against the duty limit; delivery ratio.

**Cost.** None at runtime.

### 6. Congestion-aware hop limits

**Where.** `allowPacketForward()` enforces `flood_max`, `flood_max_unscoped` and
`flood_max_advert` — three static ceilings, none aware of how busy the node is.

**Mechanism.** Reduce the effective ceiling by one when the node's own transmit
budget is below a threshold.

**Hypothesis.** Reach shrinks slightly; congestion collapse under load is
avoided. Best measured on the large fixture.

**Metric.** Delivery ratio against offered load; hop-count distribution.

**Cost.** One comparison.

### 7. ACK redundancy on poor paths

**Where.** `Mesh::getExtraAckTransmitCount()` returns `0`, and the repeater does
not override it. An acknowledgement crossing a marginal link has exactly one
chance.

**Mechanism.** Send the ACK twice when the received SNR of the request was below
a threshold.

**Hypothesis.** Round-trip completion improves on marginal paths for a small,
bounded airtime cost. This is the one most likely to be a straightforward win.

**Metric.** Request/response completion ratio; airtime.

**Cost.** One extra transmission per marginal exchange.

### 8. Do not force a transmit through a busy channel

**Where.** `Dispatcher::checkSend()` treats `getCADFailMaxDuration()` (4 s) as a
deadline and then, per its own comment, *"force the pending transmit below"* —
it transmits into a channel it knows is busy.

**Mechanism.** Force only high-priority traffic; defer or drop the rest.

**Hypothesis.** Fewer collisions under sustained load, at the cost of some
dropped low-priority traffic. Directly relevant to the listen-before-talk work
already published.

**Metric.** Collisions; delivery by priority class.

**Cost.** None.

## How they are run

- **Native firmware only.** Emulated nodes run on wall time, so two runs of one
  seed do not match, and the whole study depends on repeatability.
- **One local branch per idea** off `dev` in a local MeshCore clone, named
  `study/01-cancel-queued-relay` and so on. **Nothing is pushed.**
- **Baseline** is `727fc05` unmodified.
- **A control arm** that is the baseline built twice under different branch
  names. If the two disagree by more than the floor, the harness has a
  reproducibility problem, the study stops, and the report says so instead of
  reporting deltas.
- **Medium fixture (Scotland)** for the headline numbers; **small (Fife)** for
  the density-sensitive arms; **large** only for claims about scale.
- Seeds fixed in advance and identical across arms.

## Metrics, fixed here

| metric | why |
|---|---|
| delivery ratio | the thing that matters; the control for every airtime saving |
| transmissions per message | what an airtime saving actually is |
| duplicate receptions per node | the cost of flooding, and where idea 1 should show |
| end-to-end latency | what backoff changes buy or cost |
| airtime per node | the duty-cycle question |
| collisions | the mechanism behind most of the above |

## What would make this study worthless

Stated in advance so it cannot be rationalised later.

- the control arm disagreeing with itself by more than ±20%
- any arm whose firmware fails to build being quietly dropped without being named
- reporting a delta smaller than the floor as an improvement
- comparing arms run with different seeds, fixtures or node counts


## Results, 12 August 2026

Fife strict fixture, 56 nodes running real MeshCore firmware, one flood per run,
eight seeds per arm. Two control arms from identical source were run first:
**they agreed on every seed**, so a difference measured here is the firmware.

| arm | transmissions | receptions | delivered | verdict |
|---|---|---|---|---|
| control (twice, identical source) | 93.0 | 611.9 | 56 of 55 | the baseline |
| 1. cancel a queued relay | 93.0 | 611.9 | 56 of 55 | no change **alone** |
| 2. retransmit delay cannot be zero | 93.0 | 591.7 | 56 of 55 | no change |
| 8. do not force a transmit through a busy channel | 93.0 | 591.7 | 56 of 55 | no change |
| `rx_delay_base = 10.0` | 78.9 | 670.2 | 56 of 55 | **-15.2% transmissions** |
| `rx_delay_base = 10.0` + idea 1 | 71.9 | 665.0 | 56 of 55 | **-22.7% transmissions** |

Two reports: `docs/study-report-rx-delay.md` and
`docs/study-report-relay-suppression.md`.

**The control transmitted 93 packets on every one of eight seeds.** Not a mean:
the number 93, eight times. With one originator and no receive delay, every
repeater that hears the flood relays exactly once and `markSeen()` suppresses
the second copy, so the count is a property of the topology. The ±20% floor
quoted elsewhere in this project was measured on *reach under contention from
eight simultaneous senders*; it is a property of that contention and does not
transfer to this metric. Receptions **are** noisy here: the control ranged 548
to 754, about ±17%, which is why nothing in the reports leans on them.

### The three that changed nothing, and why

**Idea 1 alone is dead code.** `rx_delay_base` ships as `0.0f`, so
`calcRxDelay()` returns zero and every node relays on receipt. A queued relay
never waits long enough for anybody to beat it, so there is nothing to cancel.
The same change on top of a receive delay removes a further 8.9%. Measuring
either alone gives the wrong answer about it.

**Idea 2 was declared a sanity arm before it ran**, and behaved like one.
`nextInt(0, 5t+1)` draws zero rarely enough at these airtimes that forbidding it
is unmeasurable. A large delta here would have meant the harness was suspect; a
null result is the expected outcome and worth having on the record.

**Idea 8's code path was never reached.** Forcing a transmit only happens after
CAD has reported busy for longer than `getCADFailMaxDuration()`, and on 56 nodes
at SF8 with one flood in flight that does not occur. The change is untested
rather than useless: it needs a scenario with genuine congestion, which means
offered load from many senders.

### The four not implemented

Ideas 3 to 7 minus 4 remain branches with no commit on them, and the reason is
the same in each case: **the harness cannot yet measure what they claim to
improve.**

- **3. cycle-aware loop detection** and **4. density-aware backoff** are
  implementable today and were cut for time, not for measurability.
- **5. region-aware duty budget** needs per-region airtime accounting.
- **6. congestion-aware hop limits** needs offered load, as idea 8 does.
- **7. ACK redundancy on poor paths** needs request and response traffic, and
  every run here floods one message one way.

Adding offered load and airtime per node to the study harness unblocks 6 and 8
together, and is the single most useful next piece of tooling.
