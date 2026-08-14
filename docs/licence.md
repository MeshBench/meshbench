# MeshBench is GPL-3.0-or-later

**Decided 14 August 2026.** The licence decision and its reasoning, kept here
rather than as a numbered ADR: the "no licence chosen yet" notes scattered
through the docs pointed at an ADR number Plane already uses for something
else, so they pointed nowhere useful and are gone.

## The question

MeshBench had no licence, which is not a neutral state: a repository without
one grants nobody any rights, so it could not be released to end users at all.
The packaging work (`docs/release-packaging-plan.md`) turned that from a
paperwork item into a blocker, and the release pipeline refuses to publish
until this decision exists.

## What constrained the choice, and what did not

The original worry, recorded in the old CLAUDE.md paragraph, was that
**MeshCore's licence is linked into our binary and constrains the choice**.
That is no longer true, and the fix was deliberate: MeshCore is built in
`MeshBench/meshcore-native`, whose NOTICE says so in as many words - the
linking happens there, under MeshCore's own MIT terms, and MeshBench downloads
the result at runtime. Nothing of MeshCore is compiled into this binary.

The emulator forks are likewise **separate processes**, spoken to over sockets:
the QEMU fork is GPL-2.0 and tlib is LGPL, but neither is linked, and shipping
them beside our binary in one archive is aggregation rather than a combined
work. GPL-2.0-only would have been incompatible with GPL-3.0 *if* it were
linked; it is not.

What is actually linked is permissive: MIT, BSD-2/3, Apache-2.0, Unlicense -
all one-way compatible with GPL-3.0 - with one that needed checking:

- **`eclipse/paho.mqtt.golang` is dual-licensed EPL-2.0 *or* EDL-1.0.**
  EPL-2.0 alone is not GPL-compatible. EDL-1.0 is the Eclipse Distribution
  License, which is BSD-3-Clause verbatim, and is. **MeshBench takes the EDL
  branch**, which the dual licence explicitly permits. The licence window says
  so on that entry, and `tools/licgen` fails the build if a future dependency
  arrives under EPL alone.

## The decision

**GPL-3.0-or-later.** Every published version stays free: anyone who receives
a MeshBench binary can get its source, study the RF model, and check the
numbers against the code that produced them - which for a simulator whose
output is used to argue about real deployments is the property that matters
most. Modified versions handed on to others come back as source, so a
divergent fork cannot quietly become the one people cite.

Alex holds the copyright alone, so this is not a one-way door: he can
relicense, dual-licence, or sell an exception to any individual party at any
time. What is already published stays published under GPL-3.0 - that part is
not retractable, and is the point.

## What this obliges us to do

1. **Binaries carry a source offer.** GPL-3.0 §6: conveying object code means
   the recipient can get the Corresponding Source. **The repository is
   private for now**, so the pipeline attaches a source archive
   (`meshbench-<tag>-source.tar.gz`) to every release. When the repository is
   made public the archive can go, and a link will do.
2. **The licence window states it** - the first entry, from this LICENSE.
3. **Contributions.** While Alex is the only substantial author the relicensing
   freedom above holds. A significant outside contribution freezes it unless
   it arrives under a CLA. Decide that before merging one, not after.
4. **Nothing changes for the forks.** They keep their own upstream licences;
   `docs/repositories.md` and the licence window remain the inventory.
