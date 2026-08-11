# Running an experiment

The **Bench** view compares configurations: one network, several settings,
repeated, with the answer stated in words at the end.

## The shape of it

    Scenario   the network. Fixed.
      + Traffic    who transmits, what, when
      + Arms       named parameter sets - what is being varied
      x Repeats    seeds
      = Runs       one per arm per seed

An **arm** states only what it varies. Everything else stays as the scenario or
as the experiment's constants, so the difference between two arms *is* the
independent variable.

## Setting one up

**Sweep** panel, top to bottom:

1. **Scenario** — a saved project, or whatever is loaded.
2. **Senders** — the companions that originate traffic. *spread 6* takes six as
   far apart as the network allows, which matters: a cluster of neighbours
   contends with itself rather than with the mesh, and contention is usually the
   thing being measured.
3. **Observer** — a companion that only listens. Its Companion tab is the
   human-sized answer to "did it arrive".
4. **Channel and scope** — where the message goes and under which transport
   scope.
5. **Fired** — 0 seconds apart is every sender on the same simulated instant,
   which is maximum contention. A spread is the same traffic arriving politely.
6. **Message size** — airtime scales with payload, and airtime is what collides.
7. **Fire at / measure for** — every arm fires at the same simulated instant,
   which is not negotiable and is why the runner owns it.

Then **vary a parameter across arms**: pick one, type its values, *add arms*.
Nobody builds six arms by hand twice. A second parameter crosses with the first,
and the panel says what that costs before you agree to it.

**Repeats**: two or more seeds. Repeats of one seed are identical by design — the
same seed is one measurement no matter how many times you run it.

## Constants versus variables

The **Configuration** panel holds what every arm shares. "Make sure every
repeater is on minimal loop detect" belongs there, not repeated in six arms
where it can be forgotten in one.

It also shows what the network is actually set to — how many nodes carry
regions, and which. If that says nothing carries a region, stop: scoped traffic
will reach nobody and the run will look like an RF problem.

## The one that catches people out

**A repeater's path hash setting only affects the adverts it originates.**

What a message carries is stamped by the **companion that sent it**, and every
hop honours that for the life of the packet. So vary *companion path hash* and
hold the repeaters still. Varying the repeaters and expecting the traffic to
change measures nothing while looking entirely reasonable.

## Reading the result

**Matrix**: arms down, metrics across, as deltas from a pinned baseline.

- **to repeaters / to companions** — delivery per message, split by what the
  node is. Six messages across 136 repeaters is 816 chances to arrive; reaching
  the repeaters says the mesh carried it, reaching the companions says somebody
  read it.
- **collisions**, **airtime**, **to quiet** — what it cost and how long it took.

**Runs** shows every run individually, including each message's own reach, so
one that got nowhere is visible rather than averaged away by five that did well.

**Timelines** is one flood shape per arm on a shared axis. Whether one arm
spreads its transmissions further than another is visible without reading a
number.

### The line under the matrix

> the seeds disagree by more than the arms do on receptions (±2.5% within an
> arm, 1.2% between them). Not a result yet - add repeats.

Believe it. That is the difference between a finding and a draw, and it is the
mistake that is hardest to catch by eye and easiest to publish.

### The verdict

When the sweep finishes it says, in a sentence, whether it made a difference.
When it did not, it investigates *why not* — because a parameter that changed
nothing because it never reached the firmware looks exactly like one that
genuinely does not matter. It checks whether the ledgers are byte-identical,
whether the arms actually asked for different things, whether packet sizes
changed, and whether there was any contention to arbitrate.

**Export report** writes all of it as a self-contained HTML file.

## Things the runner does so you do not have to

- Wipes every node's persistent firmware state between runs. Without it the
  second arm boots with the first arm's channels and contacts.
- Fires every arm at the same absolute simulated instant, however long setup
  took.
- Flags a run where nothing relayed instead of averaging it in.
