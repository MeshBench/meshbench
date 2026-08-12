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

## Waves 1 and 2, after the study

**Wave 1 is done.**

*Headless, ADR-0019: a headless mode rather than a virtual display.* The spike
went the interesting way. Xvfb is not the obstacle - it offers GLX and Mesa
reports direct rendering through llvmpipe, and the application starts under it
and binds its control socket. But no verb answered, and the cause was not
isolated, which the ADR says rather than implying a verdict the spike did not
reach. The decision does not rest on that: control verbs are serviced on the
frame thread, so a CI harness driving them is hostage to the renderer, and a
virtual display hides that coupling in the environment where it is hardest to
debug. MeshCore ships a devcontainer, so firmware developers meet it too.

*The tool manifest*, `docs/tools-manifest.md`: four tools, why each is pinned
rather than taken from a distribution, and why PATH comes last.

**Wave 2 is started.**

*QEMU builds in CI.* `MeshBench/qemu` produces a packaged `qemu-system-xtensa`,
Linux only, with macOS and Windows as visible skipped rows carrying reasons. It
asserts the SX1262 device is in the tree it built.

*The Fife fixture is real*, `docs/fixtures.md` and `fixtures/`. 55 nodes from
live CoreScope, regions inferred from 20,500 packets over a week and applied to
53 of them, in strict and permissive variants. The permissive one adds
`region allowf *` so a first run works before anyone learns the scope rules, and
says so in its provenance.

**The firmware A/B tooling is built**, `tools/firmware-ab/ab.sh`, and it encodes
the traps rather than leaving them to whoever runs it: storage per arm through
the new `MESHCORESIM_NODEFS`, `-count=1`, and a build per role in every arm.
Verified by reproducing the result without the manual process.

## What did not happen

No packaging, no docs site, no app-testing harness, no Companion bench, no
PlatformIO hook. The medium and large fixtures are not built: same recipe, more
waiting.

**Seven of the eight ideas have no report**, and that needs saying properly
rather than as a shortfall. Two of them are pre-registered as expected nulls - a
sanity arm and a negative control - and a null earns a line in the study, not a
report. The other five cannot be measured by the current harness at all: it
sends one advert and counts what the mesh does with it, so it can see
flood-relay changes and nothing else. Ideas 5, 6 and 8 need sustained offered
load and airtime accounting; idea 7 needs request and response traffic, which
the harness does not generate.

Building that is the next real piece of work, and it is the same thing the
app-testing harness needs. Running four more arms through a harness that cannot
see what they change would produce four more nulls and no knowledge.

The study ran on the **saved 308-node import from the earlier study**, not on a
freshly built fixture, because the fixtures are Wave 2. It is the right network -
Scotland and Ireland, real topology - but its provenance is the old import.

The **report is written**: `relay-suppression/index.html` in the reports
repository, on a branch called `relay-suppression` rather than on main, because
publishing was your call the last time and this is a Pages site where main is
live. It reuses the house style of the listen-before-talk report, carries two
figures, and states the three faults that produced false results before the real
one. The index has a card for it.

I could not render it to check: the only browser on the machine is a snap, and
its headless screenshot lands inside the snap's private tmp where I could not
retrieve it. The style block is byte identical to the published report, so it
will look like its sibling, but nobody has actually looked at it yet.

## Two more gaps found by using it

Both in the control socket rather than the model, both found while building the
fixture:

- `nodes.place` refuses `room-server` and `emitter`: *unknown kind "emitter";
  have repeater, companion, observer*. Both kinds exist in the model, so a
  fixture cannot carry one of every kind through the socket.
- `firmware.set` with a role and no node applies to **every** node that runs
  firmware and sets its role, so three calls to pin three roles convert the whole
  mesh three times and the last one wins. The UI's "use for role" filters by
  kind; the verb does not. This happened here and had to be repaired per node.

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
