# Restoring `rx_delay_base`, and what it costs the mesh to have it off

**Result: 15% fewer transmissions, delivery unchanged.** On a 56-node import of
the real Fife network, running real MeshCore firmware, one flood originated per
run, eight seeds per arm.

| arm | transmissions | receptions | delivered |
|---|---|---|---|
| control, `repeater-v1.17.0` unmodified | 93.0 | 611.9 | 56 of 55 |
| `rx_delay_base = 10.0` | **78.9** | 670.2 | 56 of 55 |

**−15.2% transmissions. Delivery identical in every seed of every arm.**

## The measurement floor, and why this clears it

The ±20% floor quoted throughout this project was established on a different
measurement: reach, with around eight simultaneous senders, where contention
between the senders dominates. It is a property of that contention rather than
of the simulator, and it does not transfer unexamined to a different metric.

Here it can be stated directly instead of assumed. **The control transmitted 93
packets on every one of eight seeds.** Not a mean of 93: the number 93, eight
times. With one originator and no receive delay, every repeater that hears the
flood relays it exactly once, `markSeen()` suppresses the second copy, and the
count is a property of the topology rather than of the run.

Receptions are the noisy metric on this scenario: the control ranged from 548 to
754 across the same eight seeds, about ±17% around its mean, because how many
relays collide depends on timing that the seed does move.

So a 15% change in transmissions against a control with **zero** spread is a
real effect, and a 9% change in receptions against a control with ±17% spread is
not something to lean on. The two are quoted differently for that reason.

## The mechanism, from the code

`Dispatcher::calcRxDelay()`:

    int Dispatcher::calcRxDelay(float score, uint32_t air_time) const {
      if (_prefs.rx_delay_base <= 0.0f) return 0;
      return (int)((pow(_prefs.rx_delay_base, 0.85f - score) - 1.0) * air_time);
    }

A better-scoring node gets a shorter delay, so the better-placed relay goes
first and the others hear it while they are still waiting. That is the whole
design, and **it is switched off in the shipped build**: `rx_delay_base` is
`0.0f`, the guard returns zero, and the score is never used at all. Every node
in range relays on receipt.

The arm sets it to `10.0f` and changes nothing else. The comment above the field
says this is what it used to be.

## Why receptions rise while transmissions fall

This looks wrong at first. Fifteen per cent fewer packets are sent and *more*
are received: 611.9 becomes 670.2.

Both follow from the same cause. With every node relaying on receipt, relays
collide, and a collided packet is a transmission that cost airtime and delivered
nothing. Spreading the relays out in time by score means fewer of them overlap,
so a larger fraction of a smaller number of transmissions actually arrives
somewhere.

Duplicate receptions rise with total receptions, 568.0 to 614.2, for the same
reason: the copies that used to be destroyed in collisions now survive to be
counted as duplicates. That is not an improvement. It is the cost side of this
change and the reason the next report exists.

## What was held identical

The point of a control arm is that everything except the firmware is the same,
and three things quietly break that. All three are handled in
`tools/firmware-ab/ab.sh` rather than left to whoever runs it:

- **Saved node state beats a compiled default.** A node keeps its preferences
  between runs, as hardware does, so a node that has run before loads its old
  `rx_delay_base` and never reaches the changed one. Both arms then return
  identical numbers and the change looks inert. Every arm gets its own storage
  through `MESHCORESIM_NODEFS`.
- **The Go test cache replays the previous arm.** The cache keys on the package
  and the environment variables the test reads, not on the contents of a binary
  that a variable merely points at. Hence `-count=1`.
- **An arm needs every role, not just the one it changes.** The companion and
  room server binaries are copied in from the release cache, so exactly one
  thing differs.

**The control was run twice, from two branches of identical source.** Both
produced the same numbers on every seed: 93 transmissions, and receptions of
535, 661, 579 on seeds 1 to 3. Had they differed, nothing else in this report
would mean anything.

## Limits

**One originator.** Every run floods a single message from one node. This says
nothing about a mesh under offered load from many senders at once, which is
where a receive delay might instead add latency without saving anything.

**Fife, not the country.** 56 nodes over one region. The larger fixtures exist
now and this has not been run on them.

**The simulator is kinder than the air.** No multipath, no body loss, no
oscillator error. The biases run one way, so treat the absolute numbers as a
best case and the comparison between arms as the finding.

**A repeater default is not a free choice upstream.** Turning this on makes
every node wait before relaying, which trades latency for airtime on a network
where somebody may be relying on the current behaviour. That is a conversation
with the MeshCore developers rather than a patch to send them.

## Reproducing it

    tools/firmware-ab/ab.sh build 09-enable-rx-delay study/09-enable-rx-delay
    STUDY_SCENARIO=fixtures/fixture-fife-strict.json \
      tools/firmware-ab/ab.sh run 09-enable-rx-delay 1,2,3,4,5,6,7,8

The branch is local to the MeshCore clone at `~/msim/MeshCore` and is not
pushed anywhere.
