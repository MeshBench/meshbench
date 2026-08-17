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

## The Nordic SoftDevice: emulation is not a licence problem

**Confirmed 17 August 2026, Nordic DevZone case 362437** (opened 14 August).
This is not the decision above - it is a different licence, a third party's,
governing something MeshBench does not ship. Recorded here because it is the
same kind of question and the answer should live where the next one like it
will be looked for.

### The question

Published nRF52 firmware (`docs/packaging-emulation.md`) boots as MBR →
SoftDevice S140 6.1.1 → the MeshCore application. Nordic's 5-Clause Licence for
the SoftDevice restricts its use to "a Nordic Semiconductor ASA integrated
circuit" - clause 4. An emulated nRF52840, under Renode or QEMU, is not one.
Whether *running* an unmodified, user-supplied SoftDevice inside an emulator for
firmware testing falls foul of that clause was genuinely unclear, and MeshBench
does nothing with the SoftDevice that clause 4 obviously anticipates: no
reverse engineering, no modification, no extraction beyond filling unprogrammed
flash with `0xFF` so emulated memory starts erased the way real flash does.

Asked directly rather than assumed, with the specifics: what MeshBench does
with the binary, the three ways it could be unblocked in order of preference,
and an offer to accept whatever conditions helped.

### The answer

Nordic's Product Management Team, via DevZone: **"We don't object this... as
long as the end customer will use Nordic hardware in their end products, and no
reverse engineering / modification is done on the binary, we don't see a
problem."** They pointed at how Zephyr's Babblesim project - which simulates
Nordic's nRF5x hardware the same way MeshBench emulates it - handles the same
shape of question as a reference: MeshBench is being treated under the same
terms Nordic already applies there, not a bespoke exception.

### What this settles

**Emulating the SoftDevice for firmware testing is not a licensing problem.**
The RAK4631 path that already works under Renode, and the ~40 other nRF52
boards that do not yet have verified emulation wiring, were never blocked on
anything Nordic objects to - only on the open legal question above, now closed.
The remaining gap for those boards is entirely the engineering work of wiring
and verifying each one (`internal/scenario/boards.go`'s `EmulationVerified`);
nothing about it waits on Nordic's terms any more.

**Getting a copy is also settled, provided MeshBench fetches it rather than
ships it.** "This requires no redistribution by us at all" was the first of
the three options put to Nordic, and it is the one to build: MeshBench should
download the SoftDevice from Nordic's own site at runtime and cache it, the
same way firmware images are already fetched from GitHub releases rather than
bundled (`docs/packaging-emulation.md`, whose own checklist has the fetcher as
not-yet-built work). Nordic hosts it themselves for anyone to fetch; nothing in
their answer, or in the SoftDevice's own licence, turns *pointing MeshBench at
that download* into MeshBench distributing it - the file never touches our
infrastructure or our release archive.

Hosting a copy of the SoftDevice in MeshBench's own release archive - rather
than fetching it from Nordic at runtime - is a different act, was not put to
Nordic, and nothing here answers it. A runtime fetch does not need it answered,
because MeshBench is never the one distributing the file.
