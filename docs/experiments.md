> **Working note, last true on 11 August 2026.** Kept for the thinking in it, not maintained as a description of the code. Where this disagrees with the tree, the tree is right; the authority is the experiment verbs in `internal/app/session/`.

# Experiments

One scenario, many parameter sets, repeated — and results you can actually
explore.

## Why

The ScotMesh CAD study was six identical pipelines typed by hand, and the parts
that decided whether the numbers meant anything were the parts easiest to leave
out:

- **Wiping persisted firmware state between arms.** Forgotten once. The second
  arm booted with the first arm's channels and contacts, and nothing in the
  results said so.
- **Sending at the same simulated instant in every arm.** Forgotten once. Setup
  costs a different number of engine steps per run, so the burst landed at 32.4 s
  in one arm and 37.8 s in another — different advert phase, different result.
- **Varying the seed rather than repeating one.** Repeats of one seed are
  identical by design, so three runs of the same seed look like agreement and
  are one measurement.
- **Checking the flood happened at all.** One run reported plausible totals while
  every node transmitted exactly once: no repeater relayed, and the "result" was
  six lone transmissions and some adverts.

Each of those produced numbers that looked entirely reasonable. A pipeline that
gets them right by construction is worth more than any single comparison it runs.

The other half is that the interesting questions are not two-sided. "1-, 2- or
3-byte path hash", "off / minimal / moderate / strict loop detect", "SF8 against
SF10" — same question, same shape, more than two arms. A runner that only
understands A and B answers the narrowest version of it.

## The model

    Scenario   the network: nodes, positions, regions, boundary. Fixed.
      +
    Schedule   what happens and when: who transmits, what, to which channel
      +                and scope, at which simulated instant.
    Matrix     the arms: named parameter sets, one run each.
      x
    Repeats    seeds. Same arm, different node identities and boot phase.
      =
    Runs       arms x seeds, each producing a ledger and a row of metrics.

A **scenario** is what already exists and is saved as a project. An
**experiment** is a scenario plus a schedule plus a matrix plus repeats, saved
alongside it, so a question can be re-asked after the tool changes.

### Parameters an arm can set

Anything provisioned, because that is what an arm is: a configuration applied to
every node before the run starts. Unset means "leave the scenario's own value
alone", so an arm states only what it varies — and the diff between two arms
reads as the experiment's independent variable.

**Firmware**

| Parameter | Repeaters | Companions | Why it is interesting |
|---|---|---|---|
| Version, per role | build tag | build tag | the original question |

**Radio**

| Parameter | Repeaters | Companions | Why it is interesting |
|---|---|---|---|
| Frequency, SF, BW, CR | `set radio …` | `CMD_SET_RADIO_PARAMS` | sensitivity against airtime; the whole shape of a flood |
| TX power | `set tx …` | `CMD_SET_RADIO_TX_POWER` | reach against interference — more is not always better on a dense mesh |
| `radio.rxgain` | CLI | — | boosted gain against noise |

**Medium access — what 1.17 changed**

| Parameter | Repeaters | Companions | Why it is interesting |
|---|---|---|---|
| `cad on/off` | `set cad …` (1.17+) | — | hardware CAD against the listen-before-talk path; new in 1.17 and directly comparable |
| `int.thresh` | CLI | — | how loud the channel must be before a node holds off |
| `rxdelay`, `txdelay`, `direct.txdelay` | CLI | — | the jitter that stops a flood becoming a synchronised collision |
| `dutycycle` | CLI | — | regulatory budget; a node out of budget goes quiet and looks like a coverage hole |
| `agc.reset.interval` | CLI | — | recovery after a strong signal |

**Flooding and duplicate suppression**

| Parameter | Repeaters | Companions | Why it is interesting |
|---|---|---|---|
| `path.hash.mode` | `set path.hash.mode n` | `CMD_SET_PATH_HASH_MODE` | 1, 2 or 3 bytes per hop: fewer hash collisions against more airtime on every hop |
| `loop.detect` | `set loop.detect …` | — | duplicate suppression; interacts directly with hash size, which is why they belong in one experiment |
| `flood.max` | CLI | — | how far any flood may travel |
| `flood.max.advert` | CLI | — | how far adverts travel, which is most of the background load |
| `flood.max.unscoped` | CLI | — | how far unscoped traffic travels |
| `advert.interval`, `flood.advert.interval` | CLI | — | background load, and whether adverts collide with traffic |
| `multi.acks` | CLI | — | acknowledgement behaviour |

**Transport regions and scope**

The variable that matters most on a mesh like ScotMesh, where seven days of
traffic showed 10,018 scoped transmissions and zero unscoped: region membership
*is* the routing.

| Parameter | Repeaters | Companions | Why it is interesting |
|---|---|---|---|
| Region membership | `region put` / `region remove` | — | "what if these repeaters also carried `#fif`?" is a real operational question |
| Flood permission per region | `region allowf` / `region denyf` | — | a region that exists but does not allow flooding relays nothing — a configuration failure that looks exactly like an RF one |
| Default scope | `region default <name>` | `CMD_SET_DEFAULT_FLOOD_SCOPE` | which scope a node sends under when nothing says otherwise |
| Scoped or unscoped | — | per send, or default | the counterfactual: what would this mesh do with no transport regions at all? |
| Region source | inference / study area / none | — | compare regions inferred from live traffic against a flat configuration |

#### Region plans

An arm does not have to change every repeater the same way. A **region plan** is
a named, saved mapping of node to regions and default scope — the whole
network's transport configuration as one artefact — and an arm selects one.

That makes the interesting comparison sayable:

    inferred     what ScotMesh actually runs, from seven days of its own traffic
    flat         one region, every node in it
    none         no regions at all; everything unscoped
    proposed     somebody's redesign - Fife split out, the Central Belt merged

Four arms, one scenario, and the question is "which transport plan moves this
mesh's traffic best", which is a network-design question rather than a firmware
one. It is the same machinery either way.

Plans come from inference (the existing Import → infer → apply, saved rather
than applied straight to the nodes), from an export of the current network, or
from editing one by hand. A plan is diffable against another, which is worth
having on its own: "what did applying this actually change?"

This also answers the per-node-group question the earlier draft left open —
an arm carries a plan, not a per-group override, so arbitrary per-node
differences cost nothing extra to express.

**Network shape**

Not provisioning, but the same kind of question, and cheap to support because
the runner already rebuilds the scenario per arm.

| Parameter | Applies to | Why it is interesting |
|---|---|---|
| Antenna height | any node set | the largest single factor after terrain |
| Nodes enabled | any node set | resilience: what does this mesh do with Ben Vrackie switched off? |
| Boot stagger | engine | whether every node's advert timer fires on the same millisecond |

### The schedule

Traffic as data rather than as a script: a list of `{atMs, from, kind, channel,
scope, text}`. The senders in a burst share one `atMs`, because simultaneity is
usually the thing under test. This is the existing Schedule panel, extended to
carry channel and scope and to accept several senders on one instant.

## The runner

A state machine advanced once per frame, never a loop that blocks.

    prepare      wipe node storage; set seed; apply the arm; rebuild engine
    start        firmware, asynchronously, with progress
    settle       run to the schedule's first event
    connect      claim and configure every sender before any of them sends
    fire         every event at this instant, together
    run          to the end of the measurement window
    collect      reduce the ledger to a row; keep the ledger
    next         arm x seed

Rules the runner enforces rather than documents:

- Node storage is wiped between every run, always.
- Every arm fires at the same absolute simulated instant.
- A run whose flood never propagated is flagged, not averaged in. The check is
  cheap: transmissions barely above the node count means nothing relayed.
- Firmware that fails to attach fails the run loudly instead of running short.

## The interface

A fifth view beside Plan / Run / Debug / Verify, because comparing
configurations is a distinct task somebody spends an hour inside, with its own
panels. Call it **Bench**.

    ┌ MeshBench ──────────────────────────────────────────────────────────────┐
    │ File  View  Simulation  Repeaters  Planning  Window  Help               │
    │ Plan │ Run │ Debug │ Verify │ BENCH        154 nodes   seed varies      │
    ├─────────────────────────────────────────────────────────────────────────┤
    │ ┌ Sweep ─────────────────┐ ┌ Runs ──────────────────────────────────┐   │
    │ │ scenario  scotmesh-cad │ │ ● 1-byte  4417      done      1m 52s   │   │
    │ │ schedule  6 senders    │ │ ● 1-byte  9001      done      1m 49s   │   │
    │ │           #sco @ 45 s  │ │ ◐ 1-byte  20260810  running   0m 41s   │   │
    │ │                        │ │   2-byte  4417      queued             │   │
    │ │ vary  path.hash.mode   │ │   2-byte  9001      queued             │   │
    │ │ values  0, 1, 2        │ │   …                                    │   │
    │ │ repeats 3 seeds        │ │                     [ pause ] [ stop ] │   │
    │ │                        │ └────────────────────────────────────────┘   │
    │ │ 3 arms x 3 seeds       │ ┌ Log ───────────────────────────────────┐   │
    │ │ = 9 runs, about 14 min │ │ 1-byte 20260810  wiped node storage    │   │
    │ │                        │ │ 1-byte 20260810  154 firmware in 3.1 s │   │
    │ │ [ + add a parameter ]  │ │ 1-byte 20260810  settling to 45.0 s    │   │
    │ │ [    run sweep     ]   │ │ 1-byte 20260810  6 sent at 45.0 s      │   │
    │ └────────────────────────┘ └────────────────────────────────────────┘   │
    ├─────────────────────────────────────────────────────────────────────────┤
    │ ┌ Matrix ───────────────────────────────── baseline: 1-byte ▾ ───────┐  │
    │ │ arm       runs   tx      rx        reached  collisions  airtime    │  │
    │ │ 1-byte      3    580     8234       154       4242       409 s     │  │
    │ │ 2-byte      3    +3.1%   -1.2%      154       +0.4%      +7.8%     │  │
    │ │ 3-byte      3    +5.9%   -4.0%      153       +1.1%     +15.2%     │  │
    │ │                                                                    │  │
    │ │ ! the seeds disagree by more than the arms do on rx (+-6.1% vs     │  │
    │ │   4.0%). Not a result yet - add repeats.            [ add 3 more ] │  │
    │ └────────────────────────────────────────────────────────────────────┘  │
    └─────────────────────────────────────────────────────────────────────────┘

### Varying a parameter, not building arms

An arm is a diff, so the gesture is picking a setting and the values to try.
Right-click any provisioning setting anywhere in the workbench:

    set path.hash.mode  [ 1 ]  ▾
    ┌──────────────────────────────┐
    │  set for every node          │
    │  vary across arms…           │  ←
    │  what is this?               │
    └──────────────────────────────┘

           ↓

    ┌ Vary path.hash.mode ─────────────────────────┐
    │ values   0, 1, 2                             │
    │          1, 2, 3 bytes of hash per hop       │
    │                                              │
    │ arms     1-byte hash                         │
    │          2-byte hash                         │
    │          3-byte hash                         │
    │                        [ cancel ] [ add ]    │
    └──────────────────────────────────────────────┘

Nobody builds three arms by hand twice. Picking a parameter and its values is
one gesture and is how the question is said out loud: "does a bigger hash help?"

Adding a second parameter asks whether to cross it (every combination) or pair
it, and says what that costs before agreeing to it — 3 x 4 x 3 seeds is 36 runs
and the better part of an hour, which is worth knowing in advance rather than
discovering.

### Reading the result

Pin a baseline arm; every other cell reads as a delta with direction colour.
Absolute numbers in a matrix are work; "+5.9%" is not. It also makes *no
difference at all* unmistakable, which was the actual answer to the CAD study
and took an evening to be sure of.

The warning line under the matrix is the most important thing in this view.
When the spread between seeds is larger than the difference between arms, it
says so in words and offers to run more repeats. That is the mistake that is
hardest to catch by eye and easiest to publish.

A run that did not really happen is marked rather than averaged in:

    │ 3-byte    20260810   flagged   nothing relayed - 154 tx, 0 second hops │

### Comparing two runs

    ┌ Compare ───────────────────────────────────────────────────────────┐
    │  A  1-byte hash, seed 4417        B  3-byte hash, seed 4417        │
    │                                                                     │
    │  first divergence   event 395 at 16.270 s                          │
    │    A   tx  Drumcarrow Craig NWR   134 bytes, 1295 ms on air        │
    │    B   tx  Drumcarrow Craig NWR   132 bytes, 1295 ms on air        │
    │                                                                     │
    │  3 of 14,234 events differ. Timing is identical.                   │
    └─────────────────────────────────────────────────────────────────────┘

Totals say something changed; the first differing event says where to look.
"Timing is identical" is a sentence worth printing on its own — it is the
difference between a firmware that behaves the same and a simulator that never
asked it to behave differently.

## Watching it, without eighteen windows

Connecting a sender currently opens its node window and brings its Companion tab
to the front. That is right when a person is sending a message and wrong when
the runner is claiming six senders a run, thirty runs deep — the screen fills
with windows nobody asked for and the thing worth watching is buried.

So: **the senders go headless.** The claim, the configuration and the send all
happen exactly as they do now, but silently; the operator can still open any
node window and watch that node if they want to. What the experiment shows
instead is one panel:

- a **live log** of what the runner is doing — prepare, wipe, start, settle,
  fire, run, collect — with the arm and seed on every line, so a stall is
  attributable rather than mysterious;
- a **grid of packet timelines**, one cell per arm, drawn as small multiples on
  a shared time axis. Same burst, same axis, four cells: whether one arm spreads
  its transmissions further than another is visible without reading a number.
  This is the view that would have shown "these two are identical" instantly;
- the **matrix table** underneath, for the numbers.

The grid, on a shared axis, one cell per arm:

    ┌ Timelines ──────────────────────────────────────────────────────────┐
    │  1-byte hash                     2-byte hash                        │
    │  ▁▃█▇▅▃▂▁▁ ▁    ▁                ▁▃█▆▅▄▂▁▁▁ ▁   ▁                   │
    │  0        20 s        40 s       0        20 s        40 s          │
    │                                                                      │
    │  3-byte hash                     no regions                          │
    │  ▁▂▆█▆▅▃▂▁▁▁  ▁  ▁               ▁▁▂▂▂▂▂▂▂▁▁▁▁▁▁▁▁▁▁                │
    │  0        20 s        40 s       0        20 s        40 s          │
    └──────────────────────────────────────────────────────────────────────┘

Small multiples rather than one overlaid plot: overlaying four floods produces a
solid block, and the question is nearly always "how does this arm's shape differ
from that one's", which the eye answers well when the axes match.

## Charts as an output

The figures in the CAD report — receptions and transmissions per second,
cumulative reach, the breakdown of why receptions failed — are computed from the
ledger, which means the tool can draw them rather than having them hand-built
afterwards.

`Export report` on a finished experiment writes a self-contained HTML file: the
matrix, one small-multiple per arm, those charts per run, and the first
divergence between any two arms. SVG, so it stays sharp and can be pulled into a
write-up; no external assets, so it opens anywhere. The same data goes out as
CSV and JSON for anyone who would rather plot it themselves.

That is the difference between a tool that answers a question and one whose
answer can be shown to somebody else.

## Results

Three levels, because three different questions get asked:

1. **The matrix.** Arms down, metrics across, averaged over seeds, with the
   spread shown — a mean over three seeds that vary by 20% is not a result.
   Metrics: transmissions, receptions, nodes reached, collisions, deafness,
   airtime, time to quiet, time to last node.
2. **A run.** One row expands to its own ledger: the timeline, the map, the
   packet list. This is the existing Verify view, pointed at a stored run.
3. **A comparison.** Any two runs, with the **first divergence** — the first
   event where the ledgers stop agreeing. Totals say something changed; the
   first differing event says where to look. The engine already has this
   (`Diverge` in the assert tests); it needs surfacing.

Export is JSON and CSV: the matrix for a report, the ledger for anything else.

## What this would have done for the CAD study

`experiment.start` with two arms, three seeds — one action — and the answer
would have been immediate and unmistakable: every metric identical, and the
first divergence three advert packets in, differing only in length. That is the
signature of "the changed code never ran", and it took an evening to establish
by hand.

## Build order

1. **Arm parameters as provisioning.** `path.hash.mode` and `loop.detect` are
   in; the rest of the table follows the same shape. Useful immediately, on its
   own, and independently testable. Region plans come with this step, because
   they are the parameter most worth varying and the one that needs a place to
   live.
2. **Headless senders**, so claiming six companions stops opening six windows.
   Small on its own and needed by everything after it.
3. **The runner**, driving the existing schedule, writing rows. UI is one live
   log line at a time.
4. **The matrix view**, the small-multiple grid, and per-run drill-down.
5. **First divergence**, surfaced from the engine.
6. **Export**: self-contained HTML with the charts, plus CSV and JSON.
7. **Clients**: `experiment.define`, `.start`, `.state`, `.results`, `.compare` from the Go and Python clients.

Each step is worth having alone, which is the test of the ordering: (1) makes
hash-size questions answerable by hand today, (3) makes them repeatable, (4)
makes them readable, (5) makes them explicable, (6) makes them shareable.

## What I would check before building

- Whether a run should keep its full ledger by default. A 154-node run is around
  14,000 events; thirty runs is half a million. First divergence needs them, and
  the small multiples do not — a per-second histogram is a few hundred bytes.
  Likely answer: keep histograms always, ledgers on disk with a cap and an
  eviction rule, and say plainly when one has been dropped.
- Whether the sweep builder should offer a **paired** mode as well as a crossed
  one. "1-byte with minimal loop detect, 3-byte with strict" is two arms, not
  six, and it is often the comparison somebody means.
- Whether region plans want their own editor now or later. Selecting a saved
  plan is enough to run the interesting arms; editing one by hand is a bigger
  piece of UI, and inference plus export covers most of it.
