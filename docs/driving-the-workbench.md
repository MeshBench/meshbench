> **Working note, last true on 12 August 2026.** Kept for the thinking in it, not maintained as a description of the code. **1 of the 1 package paths it names no longer exist**, the seven-layer restructure of 19 August having moved them. Where this disagrees with the tree, the tree is right; the authority is the verb list the control socket answers.

# Driving the workbench from outside

Findings from agent-driven sessions, written down because each one cost a
wasted run.

The same knowledge is packaged as a skill at
`.claude/skills/meshcoresim/SKILL.md`, which is what an agent working in this
repository loads. This page is the prose version for people; the skill is the
operational checklist. Keep them in step, and put anything newly learned in the
skill first, because that is the copy which gets read before the mistake rather
than after it.

## The socket

`$XDG_RUNTIME_DIR/meshcoresim.sock` — on elite's desktop session that is
`/run/user/1000/meshcoresim.sock`. One JSON request per line,
`{"id":1,"method":"...","params":{...}}`, one JSON reply. The switch is
File → Preferences → *Agent control*; off means no socket file exists at all.

## The rule that matters most

**Restarting the workbench loses the scenario.** Nodes, boundary, inference
and firmware assignments live in the running process, not on disk. So:

- Never rebuild mid-pipeline. Finish, or start over.
- After any rebuild, re-run **the whole sequence** from the boundary onward.
- If a pipeline is long, save a project (`File → Save project`) before
  anything that might restart the process.

The first attempt at the ScotMesh study ran inference against a workbench
that had been restarted after the import — an empty scenario, a meaningless
result, and no error anywhere to say so.

## Ask what has already happened

`session.journal` (MCP: `session_journal`) returns every command the running
workbench has been driven with, newest last, with the node count at the time
and any error. The first entry of every session is `session.start`, so a
restart is visible rather than inferred - which is the failure that wasted
the first ScotMesh run: the process had been rebuilt between the import and
the inference, and nothing in the state said so.

Call it before assuming anything about a session you did not start.

## Order is load-bearing

1. `boundary.set` + `boundary.accept`, or `boundary.load` — **before** the
   import. The import filters at fetch time, so a boundary set afterwards
   prunes rather than filters, and the fetch has already paid for nodes it
   will discard.

   `boundary.load {path}` or `{geojson}` takes your own polygon: a Polygon,
   MultiPolygon, Feature or FeatureCollection. `boundary.set` searches
   Nominatim, so it needs the network and needs the area to have an
   administrative name; a catchment, a valley or something drawn in QGIS has
   neither. `boundary.list` says what the study area is made of.
2. `import.set_source` → `import.fetch` → poll `import.commit` until it
   stops erroring (the commit refuses until a preview exists).
3. `boundary.prune` if the boundary changed after the import.
4. `infer.run {hours}` → wait for its job → `infer.apply`. The window is the
   feed's own past and it is honoured: reading a week of ScotMesh is around
   150,000 packets. Do not call `infer.result` yourself - it is the reader's
   callback, and it refuses when called from outside.
5. `firmware.set` per version, then `sim.play`/`sim.step`.

## Firmware comes from meshcore-native, not from your own build

`MeshBench/meshcore-native` publishes a native build per MeshCore tag -
`repeater-v1.16.0`, `companion-v1.17.0`, and so on - all from one pipeline.
Those are the ones to use, and `firmware.Resolve` fetches them into
`~/.cache/meshcoresim/firmware/native/<tag>/`.

Building one by hand to fill a gap is how a study gets quietly invalidated. A
locally built 1.16.0 compiled against a stale copy of the shim answered console
output with `0x06` where the host expects `0x07`; it connected, failed to
behave, and exited. Worse than a build that fails to compile: two arms of a
comparison speaking different wire protocols measure the shim, not the
firmware. If a tag is missing, add it to meshcore-native rather than filling the
cache locally.

Downloads arrive without the execute bit. `chmod +x` them.

## Starting firmware is asynchronous

`firmware.start` returns immediately with `{starting, done, total}`; poll
`firmware.state` until `starting` is false. It also reports `configured`, the
number of nodes that took their provisioning.

It was synchronous once, and on 155 nodes that froze the window and this socket
together for as long as it was left - the handler runs on the frame thread. It
read as a crash and was reported as one. If a driven step ever leaves the
workbench silent and pegged at 0% CPU, that is the shape of the fault: an
unbounded wait on the frame thread, not slowness. Take a goroutine dump
(`kill -QUIT`) before killing it, because the dump names the line.

## A board remembers, and what it printed is readable

An emulated board keeps its flash between runs, exactly as hardware does: its
identity, its preferences and its contacts survive a stop and a start, and only
a change of build reflashes the chip. That is what you want when you configure
a node and come back to it, and what you do **not** want between the arms of a
comparison - so `node.wipe` puts one board back to factory and `firmware.wipe`
does all of them.

`node.output` reads what a node printed, from whichever of four voices you ask
for. They answer different questions and a merged log answers none of them
well:

| source | what it is |
|---|---|
| `serial` | the board's own port; a native node's standard error |
| `boot` | the ROM's own output, on a board whose application talks over USB |
| `emulator` | what QEMU or Renode said about *running* it |
| `radio` | what the radio model beside it logged |

Reach for `emulator` first when a board says nothing. An emulator that refused
a machine property or could not open a drive says so there, and that used to be
mixed into the board's own output where it read as something the firmware had
printed.

When even that is quiet, `MESHCORESIM_QEMU_DEBUG=unimp,guest_errors` in the
workbench's environment makes the emulator name every register the machine does
not implement. A single address with millions of hits is a firmware waiting for
an answer that cannot arrive. It is off by default because the output is
megabytes a second - one run of it once filled a 16 GB tmpfs, and the space
stayed allocated until the emulator holding the file was killed.

## Writing at a port is not the same as being heard

A companion is driven by writing frames at its serial port, and **writing
succeeds whether or not anything is reading**. A board whose firmware never
started takes every frame and answers none, so commands used to report
themselves sent against a node that was doing nothing at all - "it says advert
sent and nothing transmits", with the interface the last thing anybody would
suspect.

A node that has never answered a single frame is refused now. If you see that
refusal, the node is not the problem to debug: read its Output tab and find out
what its firmware is doing.

## You cannot type an imported node's name

The names come from the people running the mesh, so on ScotMesh they carry
emoji either side and sometimes a Gaelic accent: `🏔️ West Lomond 📡` is one
node's actual name. Nothing pastes it reliably and nobody types it.

`nodes.search {query, limit}` answers with `matches[]` - `name`, `score`,
`kind`, `lat`, `lon` - ranked best first. Matching is on letters and digits
alone, with accents folded, word order ignored, and the tighter name winning:
`west lomond` puts `🏔️ West Lomond 📡` above `West Lomond Relay Two`.

Check the score before acting on the top result. Taking it unconditionally is
how a script ends up adverting from a node that merely shared a word with the
query, and it does that silently. Both clients wrap this as `find`, which
refuses under 0.5 and names what it did find.

## CoreScope's real endpoints

- Region names: **`/api/scope-stats?window=7d`**, `byRegion[].name`.
  `/api/regions` does not exist and answers with the single-page app's HTML,
  which decodes as nothing and looks like "this mesh has no regions".
  HopReach has always used scope-stats (`internal/corescope/scope.go`).
- Names are MeshCore's publicly-known hashtag regions — `#sco`, `#fif`,
  `#ioi`, `#ioi-admin`, `#wls`, `#noc`, `#per`, `#gla` on ScotMesh — and the
  key is `sha256(name)[:16]` over the name **including the hash**
  (`TransportKeyStore::getAutoKeyFor`).
- Packets: `/api/packets` answers `{"packets":[...]}`, carries `_parsedPath`
  (the hop count) and `raw_hex`.
- `/api/channels` is a *different* thing — chat channels, not transport
  regions. They look alike and are not.

## Screenshots

Capture the workbench **by window name**, which gets the window itself rather
than whatever the compositor thinks is focused:

    DISPLAY=:1 XAUTHORITY=/run/user/1000/xauth_DqaJas \
      import -window "MeshBench - main window" shot.png

It works because the workbench is an XWayland client, so it has a real X
window even though the desktop is Wayland. `import -window root` does not:
under a Wayland compositor the X root is empty. `spectacle -b -n -f` grabs
the whole desktop, which is usually the wrong thing - it caught Discord on
the other monitor rather than the workbench.

## Making the work visible

Commands that start work reveal the panel that reports it — an import or an
inference driven from outside otherwise leaves the window apparently idle,
which is indistinguishable from a hang. Anything added here that takes more
than a second should do the same.

## ScotMesh, measured

- 546 nodes published; **153 inside the Scotland boundary**.
- 48 h of traffic: 11,135 packets, 38 nodes seen relaying.
- 7 days: 10,018 scoped transmissions, **zero unscoped** — this mesh is
  entirely transport-scoped, so region membership is the whole story.

## Setting up the ScotMesh CAD study

The exact sequence, because getting it wrong is silent every time.

    project.open scotmesh-cad-study     # or build it: boundary -> import -> infer -> apply
    firmware.wipe                        # BEFORE the arm, always
    sim.seed  {"seed": 4417}
    sim.reset
    firmware.set {"version":"repeater-v1.16.0","role":"simple_repeater"}
    firmware.set {"node":"<each companion>","version":"companion-v1.16.0","role":"companion_radio"}
    firmware.start                       # poll firmware.state until starting=false
    sim.run to a fixed instant           # poll sim.state until playing=false
    companion.connect / configure / add_channel "#sco"   per sender
    sim.run to the SAME absolute instant in every arm, then send
    events.dump -> compare

Rules learned the hard way:

- **Wipe between arms.** The firmware persists prefs, channels and contacts to
  its working directory and reads them back at boot. Without a wipe, arm two
  starts with arm one's state and the comparison is meaningless while looking
  fine.
- **Send at a fixed absolute simulated time.** Companion setup costs a different
  number of engine steps each run, so "settle then send" puts the burst at a
  different instant in each arm - a confound worth nothing.
- **Vary the seed if you want statistics.** Repeats of one seed are identical by
  design. Three seeds is enough to see whether a difference is real.
- **One message tests nothing.** CAD only acts under contention. Six companions
  sending on the same simulated instant gives roughly 3:1 loss to collision and
  self-deafness, which is the regime worth measuring.
- **Check the flood actually happened.** If every node transmitted exactly once,
  nothing relayed and you are measuring adverts. `tx` around 100 on a 154-node
  scenario means no flood; around 480 means a real one.

### What this study could not measure, and what fixed it

This section used to say that `HostRadio` never implemented `isReceiving()`, so
`Dispatcher::checkSend()` always took the channel-is-clear branch, no
listen-before-talk ran, and any CAD comparison reported "no difference"
regardless of the firmware.

That is no longer the case. `HostRadio` has been replaced by MeshCore's own
driver over real RadioLib on a virtual SX1262, so `isReceiving()` is answered
from the engine's channel state and the listen-before-talk paths execute. See
`docs/virtual-sx1262.md`.

The comparison still reported no difference, for a better reason: on a radio
that behaves, 1.16's two-line check and 1.17's state machine agree. Separating
them needed a radio that misbehaves, which is what the `-faultyirq` builds are
for.

Two later findings that change how this study should be read:

- **Six senders on one instant makes the scenario chaotic.** Configurations that
  ought to be equivalent land up to 20% apart, deterministically. It is the
  regime worth measuring for contention, but it is not an instrument that can
  resolve a small effect. Quote between-arm differences against that floor, or
  use a single sender.
- **Repeats of one seed are identical**, and in the simultaneous arms so are
  different seeds. Three seeds is three runs, not three samples. To get a real
  interval, vary something that perturbs the mesh rather than the noise.

## Watching more than one log at once

*Added 25 August 2026.*

A node's **Output** tab shows one of its four voices at a time: what the board
printed (`serial`), what the ROM printed before the board's own console existed
(`rom`), what the emulator said about running it (`emulator`), and what the
radio model logged (`radio`).

**"pop out"** puts the one being shown into a window of its own, so a board's
screen and two of its logs can be watched together — which is what a
misbehaving board actually needs: what it printed beside what the emulator said
about running it.

    node.output_window {node, source}

Every popped-out window keeps its own subscription. Panes no longer share one
slot in the world, so two windows on two nodes stay filled, and switching
source no longer blanks the pane it switched away from. Nothing on disk was
ever lost when they did — the files under `~/.cache/meshcoresim/nodefs/<node>/`
hold everything, and the pane's footer names the one it is reading.
