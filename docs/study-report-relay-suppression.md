# Cancelling a relay somebody else already sent

Idea 1 of the eight, and the one with the largest predicted saving: when a node
is holding a flood packet to retransmit and hears a neighbour transmit that same
packet, drop the queued copy.

**Result: it works, and only when there is a window to cancel in.** Alone it
changed nothing at all. On top of a receive delay it removes a further 9% of
transmissions.

| arm | transmissions | delivered |
|---|---|---|
| control, `repeater-v1.17.0` unmodified | 93.0 | 56 of 55 |
| suppression alone | **93.0** | 56 of 55 |
| `rx_delay_base = 10.0` | 78.9 | 56 of 55 |
| `rx_delay_base = 10.0` plus suppression | **71.9** | 56 of 55 |

56 nodes from the real Fife network, real MeshCore firmware, one flood per run,
eight seeds per arm. **−22.7% against the control for the pair; −8.9% for
suppression against the receive delay alone.**

## The null result is the interesting one

Suppression alone did not change one number. Not "no significant change": the
same 93 transmissions on all eight seeds, and receptions of 535, 661, 579, 754,
611, 604, 603, 548, which is the control's list digit for digit.

That is exactly what the code says should happen. `rx_delay_base` ships as
`0.0f`, so `calcRxDelay()` returns zero and every node relays the moment it
receives. **A queued relay never sits in the queue long enough for anybody to
beat it.** There is nothing to cancel, so a cancellation path is dead code.

The saving only exists once relays are spread out in time, which is what the
receive delay does. The two changes are not independent, and measuring either
alone would have given the wrong answer about it: suppression looks worthless,
and the receive delay looks like it has found all there is.

## The mechanism, from the code

`Mesh::allowPacketForward()` only ever returns `false`, and `_tables->markSeen()`
stops a *second* copy being queued. Neither of them touches a copy already
queued. `removeOutboundByIdx()` exists on the packet manager and is never called
for this purpose anywhere in `Mesh.cpp`, `Dispatcher.cpp` or the repeater.

The arm adds a small ring of pending flood hashes to `Dispatcher`, marks one as
heard when the same hash arrives from somebody else, and drops the queued copy
instead of transmitting it:

    bool Dispatcher::takePendingFloodHeard(const uint8_t* hash) {
      int i = findPendingFlood(hash);
      if (i < 0) return false;
      bool heard = pending_flood_heard[i];
      pending_flood_used[i] = false;   // done with it either way
      return heard;
    }

Cost is one hash comparison per received flood packet against a queue of at most
a few entries, and `MAX_PENDING_FLOOD` hashes of RAM.

## What it does not fix

Duplicate receptions stay high: 568.0 in the control, 609.0 in the combined arm.
Fewer packets are sent, and more of what is sent survives to be heard, so the
node at the end of several paths still receives the same message several times.
Suppression removes *redundant transmissions*, not *redundant arrivals*. Those
are different problems and only the first one is addressed here.

## Limits

Everything in the receive-delay report applies unchanged: one originator, one
region, no offered load, and a simulator that is kinder than the air. The two
arms share a scenario and a harness, so they share its limitations.

One more that is specific to this idea. **A cancelled relay is a decision made
with incomplete information.** Hearing a neighbour transmit the packet does not
mean everybody who needed it heard that neighbour. On this topology delivery was
identical in every seed, but a sparser mesh with a cut edge is exactly where
cancelling the second copy could cost a delivery, and this scenario does not
contain one.

## Reproducing it

    tools/firmware-ab/ab.sh build 01-cancel-queued-relay study/01-cancel-queued-relay
    tools/firmware-ab/ab.sh build 10-rx-delay-plus-suppress study/10-rx-delay-plus-suppress
    STUDY_SCENARIO=fixtures/fixture-fife-strict.json \
      tools/firmware-ab/ab.sh run 10-rx-delay-plus-suppress 1,2,3,4,5,6,7,8

Both branches are local to `~/msim/MeshCore` and are not pushed anywhere.
