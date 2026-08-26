# Firmware build settings

*Last true: 25 August 2026.*

Every build in the library can carry settings of its own. They are stored
**beside the image**, in a `<image>.msim.json` file, which is what makes them
follow the build: delete it and they go with it, rename it and they come along,
copy the cache to another machine and they arrive intact.

Reach them by clicking a build's name in the **Firmware Library** — that opens
the build's own window — or with the `firmware.update` verb, which takes the
same fields under the same names.

They exist because several things that decide whether a board boots are
properties of the *firmware*, not of the *hardware*. The same LilyGo T-Deck
runs a stock MeshCore image that wants none of these and a Rust one that cannot
be got past the first fault without two of them. A board profile cannot be
right about both, so the answer belongs to the build.

## Why these are settings and not defaults

Each one makes the emulator behave in a way the part does not, or overrides a
choice the board profile made. On by default they would be lies told to every
board; off by default they are a way to look at one firmware. Where a setting
costs honesty, the entry below says what it costs.

---

## Name, role and board

The three names a build is known by, and together its identity: a board image
is stored as `board/<board>/<role>@<label>.bin` and nothing else records what
it is. **Renaming moves the file.**

Anything pinned to the old name is repointed as part of the rename. Without
that, a node would go on asking for a name nothing answers to and fail at its
next start with *no image in the cache* — about a build sitting in the library
under its new name.

Roles are the four the runner will actually select. `companion_radio_ble` is
deliberately not offered: an emulated node has no Bluetooth, so a BLE companion
image imported for a board is one that can never run here.

A build made for this machine cannot be renamed. Its role is encoded in a
filename composed from the host's OS and architecture and its version is the
directory it sits in, so "rename" would mean three different things at once —
and unlike a board image, it can be fetched again.

**Verb:** `firmware.update {version, label, new_role, new_board}`

## Coprocessors enabled at reset

`coproc_at_reset`

The Xtensa part resets with its coprocessors disabled and the firmware decides
which of its tasks may use the floating point unit. A firmware whose exception
handler saves floating point state before anything has enabled them takes a
CoprocessorDisabled trap **inside an exception vector**, which is fatal, loops
for ever, and hides everything behind it. The board looks like it simply
stopped.

On, the machine brings the coprocessors up enabled and that fault is not taken.

**What it costs:** the machine is reporting `CPENABLE` in a way silicon does
not. A firmware that genuinely mismanages that register is flattered by this
rather than caught. Treat anything measured with it on as measured on a machine
that is lying about a register. It exists to make the *next* fault visible, not
to make a board work.

Found on mesh-rs for the LilyGo T-Deck, where it took the run from 18,671,402
refused stores to none. `docs/shortcomings.md` §3.5 has the whole account.

`MESHBENCH_QEMU_COPROC_AT_RESET=1` forces it on for every board at once,
which is the form a script reaching for it once wants.

## Needs a card in the board's slot

`card_required`

A firmware that keeps its settings on removable storage boots into nothing
without a card. On, every node running this build is given one whatever its own
slot was set to.

The alternative is a boot failure several minutes into a run, in a message that
never mentions cards. A node's own card slot is set in its **Hardware** tab; a
build marked this way overrides it and the node window says so rather than
leaving a switch that will not move.

### The node's side of it

A node's own slot is set in its **Hardware** tab, beside the drawn board:
whether a card is fitted, which file it is, and erasing it. By default a card
is the node's own — `card.img` beside its flash, named after it — and 64 MB.

**"use another file..."** hands the node a card somebody prepared, or one
shared between runs. **"erase card"** is what reformatting one is, asks twice,
and is refused while the node is running.

Wiping every node's memory takes the cards with it, including the ones kept
outside the node directories — a node put back to factory with its storage
intact is not back to factory, and the firmware would find its old settings on
the card with nothing saying why.

**Verbs:** `node.card {node, fitted, file, wipe}`, and `firmware.wipe` for all
of them at once.

## Notes

`notes`

Free text: where the build came from, what it is for, what is wrong with it.
Shown in the build's window and carried with the image.

Worth using for the thing the next person will otherwise rediscover — *"traps
on `rur.fcr` inside its own exception vector"* is a sentence that saves a day.

---

## What the window shows and does not change

The **Where it is** section is read from disk rather than stored:

| line | what it is |
|---|---|
| `file` | where the image sits in the cache — the only thing that decides what a node can run |
| `settings` | where these settings are written, named whether or not any exist yet |
| `size`, `modified` | how a build imported twice under one name is told from the one before it |
| `image` | what reading the front of the image says it is, and the flash size its header declares |
| `used by` | how many nodes in this scenario run it, so a delete can say what it would break |

`image` is the one worth reading before anything else. **An application-only
build imports, lists and pins exactly like a whole flash image and then starts
nothing** — a board starts from the bootloader, and an application image
belongs at 0x10000. Imports now refuse it with the reason, but a build already
in the cache from before will say `application only - no partition table` here.

## Where these are stored

```
~/.cache/meshbench/firmware/board/<board>/<role>@<label>.bin
~/.cache/meshbench/firmware/board/<board>/<role>@<label>.bin.msim.json
```

The settings file is written only when something has been decided, and removed
again when everything returns to its default — so a build back at its defaults
is indistinguishable from one that never had settings, and the cache does not
fill with files recording that nothing was chosen.
