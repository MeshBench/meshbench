# Building a network

From nothing to 154 Scottish repeaters running real firmware, in about five
minutes of clicking and twenty of waiting.

## 1. Draw the boundary first

**Plan → Boundary**, type a place, press search, accept the result.

Do this *before* importing. The import filters at fetch time, so a boundary set
afterwards prunes what has already been downloaded rather than avoiding it — and
on a national mesh that is a few hundred nodes fetched only to be thrown away.

The RF margin (30 km by default) keeps nodes that sit outside the study area but
still reach into it. They are simulated and not reported on, which is right: a
repeater just over the border still relays your traffic and still interferes
with it, and dropping it produces a mesh that behaves better than reality.

## 2. Import the real network

**Plan → Import**, choose **corescope**, give it a URL, *fetch preview*.

The preview tells you what it had to assume before you commit to it — a node
with no recorded antenna height is placed at 10 m and says so, because after
terrain that is the largest single factor in whether it reaches anything.

Then *commit*. Nodes with no position at all are counted and left out: they
exist, but nothing can be computed about where.

## 3. Work out the transport regions

This is the step people skip, and on a scoped mesh it decides everything.

In the Import panel, **Read the traffic too** → set hours → *read the traffic*.
It reads real packets and works out which transport regions each node actually
carries, from its own traffic rather than from a guess.

Then **apply to the matching nodes**.

Notes from doing this on ScotMesh:

- **Ask for a week, not two days.** Small regions are the interesting ones and
  they are quiet. `#fif` does not appear at all in 48 hours.
- Nodes are matched by **name**, not public key. Simulated nodes generate their
  own keys on boot, so the key from the live network is not the key here.
- Regions come from `/api/scope-stats`, and they are MeshCore's public hashtag
  regions — `#sco`, `#fif`, `#ioi`. They are not the same thing as chat
  channels, which look identical and are not.

**Check it landed.** Open a repeater's node window → Console → type `region`.
The firmware will tell you what it actually holds. If it says nothing, nothing
was applied, and scoped traffic will reach nobody:

    > region
      -> *^ F
     ioi F
     sco F
     fif F
     wls F

## 4. Run the firmware

The toolbar's **▶** with *real firmware* ticked starts MeshCore on every node
that runs it — 154 processes in about three seconds. Provisioning applies as
they come up: names, positions, regions, the clock.

If a node reports nothing on its console, it is not running firmware. If every
node transmits exactly once and nothing is relayed, its regions never applied —
go back to step 3.

## 5. Save it

**File → Save project.** Everything above lives in the running process, and any
rebuild of the tool takes it with it. An import plus a week of inference is the
better part of an hour to redo.
