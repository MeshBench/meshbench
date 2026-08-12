# Overnight, 12 August 2026

There is a real performance result, a user-facing feature that works, and three
methodological traps found the hard way. Everything is on branch `plan-2026-08`.

## The result

**Transmissions fall about 30% on the 308-node Scotland and Ireland network,
with delivery unchanged.**

| arm | transmissions (seed 1 / 2) | duplicate receptions | delivered |
|---|---|---|---|
| control, as shipped | 518 / 518 | 3526 / 3562 | 308 / 308 |
| delay restored | 450 / 443 | 4496 / 4640 | 308 / 308 |
| delay + suppression | **327 / 395** | 2690 / 3406 | 308 / 308 |

Two changes, and they separate cleanly.

**The delay is already in the firmware and ships turned off.**
`examples/simple_repeater/MyMesh.cpp` reads
`_prefs.rx_delay_base = 0.0f;   // turn off by default, was 10.0;`, and
`calcRxDelay()` returns zero unless it is set. That function is what makes the
**better-placed node relay first** - it scales the hold time by the received
score. With it off, every node in range relays on receipt and the score is not
used at all. The companion firmware carries the same value with a stronger note:
`//_prefs.rx_delay_base = 10.0f;  enable once new algo fixed`.

**Suppression is what the delay was missing.** While a node holds a packet, it
now drops its own relay if it hears somebody else send the same one. Alone this
does nothing - measured, exactly zero - because with the delay off there is no
holding window to suppress in. Together they are the 30%.

Duplicate receptions are noisier: down 24% on one seed and 4% on the other, so I
would report transmissions as the finding and duplicates as directionally down.
Delivery was 308 of 308 in every arm and every seed, which is the number that
had to not move.

## The user-facing feature

**The repeater console had no `help`.** Typing it returned `Err - ??` - the same
answer as gibberish - so the first thing a newcomer tries reads as a broken node,
and the command set is discoverable only from documentation you have to know
exists.

    help          -> help <region|radio> | get <name> | set <name> <val> | advert
                     | reboot | clock | password | log start/stop | erase | ver
    help region   -> regions: put <r> | allowf <r> | denyf <r> | default <r> |
                     load | save. Note: type the name bare here (sco), the hash
                     form (#sco) is what goes on air.
    help radio    -> radio: get/set freq bw sf cr tx rxdelay f_txdelay d_txdelay
                     agc_int hash_mode. 'get radio' prints them all.

Tested the way a person would use it: type the word at a real node's console and
read the answer (`TestConsoleHelpAnswersAPerson`). The `help region` line carries
the hash-versus-bare distinction that has cost us a session before.

## Three traps, all found by running rather than reasoning

**1. Persisted node state overrides firmware defaults.** All three arms first
returned byte-identical numbers on the large network. The 308 nodes carried
`prefs.json` from the earlier loop-detect study with `rxdelay:0` in them, so the
firmware loaded the old value and the changed default was never reached. Moved
to `nodefs.prestudy` rather than deleted - identities regenerate from the seed.

This one generalises: **an A/B on a compiled default measures nothing on any
scenario whose nodes have run before**, and it fails silently and symmetrically,
which is the worst way to fail. The firmware tooling has to wipe or namespace
node state per arm.

**2. Go's test cache returned a stale arm.** It keys on the package, its inputs
and the environment variables a test reads - not on the contents of a binary the
environment merely points at. Rebuilding an arm and re-running with the same
`MESHCORESIM_NATIVE` replayed the previous arm's numbers, which reads exactly
like a change that did nothing. Fixed with `-count=1` and a comment saying why.

**3. I implemented the idea in the wrong queue first.** Cancelling the outbound
queue does nothing, because the flood delay is on the inbound side and nothing
is queued outbound at the moment a duplicate arrives. The control arm made that
legible in one run: exactly identical numbers rather than a plausible small
delta.

## What the skill saved

Reading it before running caught four things that would have wasted the large-mesh
runs: originate from a repeater because a companion has no command line and
typing at one fails silently; pin versions per role because a companion asked for
`repeater-v1.17.0` resolves nothing; give every arm the companion and room-server
builds it does not change, since `MESHCORESIM_NATIVE` as a directory needs every
role present; and use the network's own EU/UK Narrow preset rather than the
synthetic one.

## Also landed

- **The plan**, `docs/plan-2026-08.md`, with progress recorded in the file.
- **Eight ideas pre-registered**, `docs/study-protocol-ideas.md`, each naming the
  line of firmware that prompted it, written before anything ran, including what
  would make the study worthless.
- **Twelve local MeshCore branches** (`study/*`). **Nothing pushed** - confirmed
  by `git ls-remote --heads origin 'study/*'` returning zero.
- **The A/B harness**, which is workstream G in prototype: `buildarm.sh` builds an
  arm, `arm.sh` and `big.sh` run one, and arms are pinned per node through
  `MESHCORESIM_NATIVE`. You spotted that this is the firmware tooling; it should
  be cleaned up and given a UI rather than designed again.
- The **control arm agrees with itself exactly** - 518 transmissions both seeds -
  so the ±20% floor from the earlier study is a property of contention with eight
  simultaneous senders, not of the simulator. Single-originator floods are
  deterministic and small deltas there are real.

## What did not happen

Waves 1 to 3 are untouched: no headless decision, no tool manifest, no fork CI,
no packaging, no fixtures, no docs site. The night went to Wave 4 because the
goal named it the priority.

The study ran on the **saved 308-node import from the earlier study**, not on a
freshly built fixture, because the fixtures are Wave 2. It is the right network -
Scotland and Ireland, real topology - but its provenance is the old import.

The **report is not written yet**. The numbers, the mechanism and the figures it
needs are all here; it wants an hour and the report style, and I would rather
write it awake than at four in the morning.

## What needs you

1. **The `rx_delay_base` finding is worth telling the MeshCore developers.** A
   mechanism the firmware implements, that decides which node relays, disabled by
   default in both repeater and companion, with a note saying it awaits a fix -
   and restoring it plus one suppression check is a 30% airtime reduction with no
   delivery cost. That is worth an issue on their tracker whatever we do with it.
2. **Confirm the feature arms** are additions rather than replacements. I did not
   rewrite the pre-registered eight after seeing a result; `help` is arm 11.
3. `nodefs.prestudy` holds your previous node identities. Say the word and I will
   restore or delete it.
