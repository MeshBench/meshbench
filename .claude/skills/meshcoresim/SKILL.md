---
name: meshcoresim
description: Drive MeshBench to answer RF and mesh-network questions, such as link viability, coverage, why a packet missed, site selection, solar survival, firmware A/B. Use when asked about MeshCore network behaviour, repeater placement, coverage, or radio settings. Encodes the honesty rules the simulator's own results depend on.
---

# MeshBench

An RF-accurate MeshCore simulator: real firmware, sample-accurate LoRa baseband,
real terrain. Plane project **MSIM**; decisions are recorded as ADRs, in the
project's Pages and, where a decision is about the tree, under `docs/`.

Load `plane-conventions` if you are also updating tickets.

## Driving it

It is a **native desktop app**, not a CLI or a service, so the socket needs a
workbench already running (`meshbench workbench`, or `meshbench headless` for a
session with no window). You drive it over
`$XDG_RUNTIME_DIR/meshbench.sock`, newline-delimited JSON,
`{"id":1,"method":"<verb>","params":{}}`. `session.describe` lists every verb
and `session.list` names the workbenches actually running; read the first before
inventing a way to do something, because there is almost always a verb for it.

**Some questions do not need a session at all.** `meshbench link`, `profile`,
`coverage` and `terrain` are one-shot subcommands over their own tile store, and
they are the quickest route to a number. They do not read the workbench's
preferences, which is both why they work on a machine that has never opened the
app and why their defaults are their own: `meshbench link` defaults to
869.525 MHz, the **deprecated** preset, so pass `-freq 869.618` to compare with
anything the app produced.

Prefer the verb over the file. Verbs drive the same code paths a person clicks,
so the panel opens and the operator can see what you did; editing config or
scenario JSON behind the app's back does not.

**Launching it over a remote session** takes the whole environment of the
running desktop process, not a guessed subset of it: a display, its compositor
socket and its authority cookie are all needed, and dropping the cookie gives
"Authorization required, but no authorization protocol specified", which reads
like a display problem and is not. Machine-specific paths for that live outside
this repository.

**Do not restart the app to pick up a build while someone is watching it.** Ask.

**Capture is started per session, not per run.** `capture.wireshark` opens the
UDP stream on 127.0.0.1:5555 and launches Wireshark; it survives the engine
rebuild each sweep run does, but a restarted workbench has no capture at all.
"Wireshark shows nothing" after a restart means nobody started it.

## Terrain is off until somebody allows it, and that is silent

A machine that has never been asked runs with the tile store **offline**. The
first warm is then held: it marks the `links` job finished *and failed* with
"waiting for permission to download terrain" and marks the session warmed, so a
wait returns at once with **zero links measured**, and every study runs over
bare earth, which is free space, which is the most optimistic answer there is.
Nothing raises, and the map still draws.

`terrain.allow` answers the question and restarts the held warm; `setup.check`
reports it as `undecided` alongside everything else a fresh install is missing.
**Check one of the two before quoting any margin**, because a result computed
over absent terrain looks exactly like a result computed over flat ground, and
the difference is every hill between the two ends.

## Building a scenario from CoreScope: the whole order

Do these in order. Every step below was skipped at least once, and each failure
looks like bad RF rather than a missing step.

1. **Boundaries.** `boundary.set` (place) then `boundary.accept`, once per
   region. The chosen set unions, so Scotland plus Ireland is two accepts.
2. **Import.** `import.set_source`, `import.fetch`, `import.commit` with
   `strategy: "replace-all"` (plain `"replace"` is not a strategy name and
   leaves the demo nodes in, on a different preset).
3. **Firmware, per role.** `firmware.set` with `role: "simple_repeater"` for
   everything, then again per companion with `role: "companion_radio"`. Or set
   `repeater_version` / `companion_version` on `experiment.base`.
4. **Regions: `infer.run` then `infer.apply`.** This is the step that gets
   forgotten, and it is the one that decides whether anything relays at all.
5. `firmware.start`, then check `firmware.state` says `running == total`.
6. Only then define and start the sweep.

### A study area of your own

`boundary.load {path}` or `{geojson}` takes a Polygon, MultiPolygon, Feature or
FeatureCollection and puts it in the study area. `boundary.set` searches
Nominatim, which needs the network and needs the area to have an administrative
name; a catchment, a valley or something drawn in QGIS has neither.

**Before the import, either way.** The import filters at fetch time, so a
boundary set afterwards prunes what has already been fetched. `boundary.list`
answers `areas` with each area's name and ring count; the snapshot only carries
how many.

### Finding a node you cannot type

Imported names carry emoji and accents, `🏔️ West Lomond 📡` being one real node,
so `nodes.search {"query": "west lomond"}` is how you get a handle on one. It
answers `matches[]` ranked best first with a `score`; the tighter name wins, so
the exact one beats the one that merely starts the same way. **Check the score.**
Taking the top result unconditionally is how a command ends up going to a node
that shared one word with the query, silently.

### Getting your own build in

`firmware.import {path, role, board, label}`. The `label` is what the library
will know it by and what a node pins; leave it out and it is a timestamp.
Never assume two imports of the same file are the same build: they are two, on
purpose, so you can move a node onto the new one and then `firmware.delete` the
old by its `path`. Delete only *after* the node is on the replacement; a pin
nothing can honour does not fail until the node next starts.

### The repeater console

`console.type {"node": …, "command": …}` runs a line on a node's CLI and returns
what it said, which is the fastest way to find out what a node actually
believes rather than what you think you configured. `get name`, `get repeat`,
`get flood.max`, `get path.hash.mode`, `get loop.detect` all read back;
`region put <r>`, `region allowf <r>`, `region default <r>`, `region save`
configure regions.

**The command reference is at <https://docs.meshcore.io/cli_commands/>**; check
it before concluding a setting did not apply. There is no `region list` and no
`help`; both answer `Err - ??`, which looks like a broken node and is just a
command that does not exist.

Console replies come back empty while a sweep is driving the engine: the reply
is collected after a 50 ms step, and the experiment owns the clock. Call
`experiment.stop` first.

### Regions decide whether the mesh relays anything

MeshCore repeaters only forward flood traffic for regions they have been told
about. A freshly imported node has none, so a scoped message is transmitted by
its sender and dropped by all 300 repeaters: **8 transmissions, 0 relays, and a
ledger of 137 events.** That reads exactly like a network with no propagation.

Regions are not in the node API. They are **inferred from days of CoreScope
packet traffic**: `infer.run {"hours": 168}`, wait for its job, then
`infer.apply`. Applying is a separate call and returns how many nodes it
touched; "0 applied" means you inferred and walked away. On ScotMesh a week of
traffic is around 150,000 packets and yields `#sco`, `#ioi`, `#ioi-admin`,
`#fif`, `#wls`, `#noc`, `#per`, `#gla`.

Wait on the **job** called `infer`, not on `infer.result`. That verb is the
reading goroutine's own callback and refuses when called from anywhere else;
before it refused, calling it from outside replaced a finished inference with
an empty one. The job is marked finished rather than removed, so a waiter that
waits for the job list to *empty* waits for ever, and one that ends when the
list stops changing has to check `failed`: a read that could not reach the
feed ends the job too.

The `hours` window is honoured. It used to be accepted, echoed back and
discarded, every import reading the most recent 40,000 packets whatever it
said, which is under two days on ScotMesh, drops the quiet regions entirely and
reads as a mesh that has gone silent.

### The `#` asymmetry: write the scope with it, the region without

This one cost a whole session. A region is spelled **two different ways** and
both are correct:

| where | form | example |
|---|---|---|
| repeater CLI | **bare** | `region put sco`, `region allowf sco` |
| scope on the wire | **`#`-prefixed** | scope `#sco` |

The key in the packet is `sha256("#sco")[:16]`. Ask a companion to send with
scope `"sco"` and it keys its packets `sha256("sco")`, which matches no
repeater in existence. Every repeater receives the packet, derives a different
key, and declines to forward.

**There is no error anywhere.** The senders transmit, the ledger fills with
"first time this node heard the message", and nothing relays: 8 transmissions,
0 relays, 137 events. It reads exactly like a mesh with no propagation, and the
temptation is to go hunting through RF, firmware roles and regions, all of
which will look correct, because they are.

`experiment.define {"scope": "#sco"}`. The workbench now canonicalises it, but
say it with the `#` anyway.

Then **send on a scope the nodes actually hold**. `experiment.define`'s `scope`
is applied to the senders only; the repeaters relay it or not according to what
inference gave them. Check the holder counts `infer.apply` reports before
choosing: `#sco` and `#ioi` are the two big ones, and a scope only a handful
hold will look like a dead mesh for the same reason as above.

## Antennas: a scalar gain is a wrong answer, not a rough one

Every node stands under a real pattern now, and the engine is directional in
azimuth, so what a node is pointing at changes the result.

- `node.antenna {node}` reads one back. A node with **no** antenna answers
  `pattern: ""` and `peak_dbi: 0`, deliberately distinguishable from an omni at
  0 dBi. Check which you have before quoting a gain.
- `nodes.antenna` is the only verb that changes one, for a node, a kind, or
  everything. It is a **partial overlay**: named fields replace, the rest stay,
  so a bearing sweep is one call per step rather than a full restatement. An
  unrecognised `polarisation` is refused rather than stored, because an
  unrecognised value reads as orthogonal to everything and would take the link
  off the air.
- `node.aim {node, at}` computes the great-circle bearing, sets it, and returns
  `gain_dbi`, which is what the turn actually won. On an omni that is nothing,
  and saying so is the point.

Two limits to quote with any directional result: the patterns are analytic
Gaussians with a flat front-to-back floor, right on boresight and roughly right
for the first 20 degrees or so, with no side lobes and no nulls; and
polarisation mismatch is charged once per pair in the engine and the link
budget but **not at all in the coverage raster**, so a raster and a budget over
the same crossed pair disagree by up to 20 dB and the raster is the optimistic
one.

## Read the firmware before explaining a result

MeshCore is public and the tags match our firmware refs exactly
(`repeater-v1.17.0`, `companion-v1.17.0`). Clone
`github.com/meshcore-dev/meshcore`, check out the tag under test, and read the
code before writing down a mechanism: a plausible story about what the firmware
"probably does" is worth nothing next to twenty lines of it.

Worth knowing, from `examples/simple_repeater/MyMesh.cpp` at v1.17.0:

- `allowPacketForward()` **only ever returns false**. Loop detection cannot
  cause more forwarding; if totals go *up* when you enable it, the cause is
  second-order (timing, congestion, which nodes hear what) and needs measuring,
  not explaining.
- The loop thresholds are **indexed by path-hash size**:
  `minimal {_,4,2,1}`, `moderate {_,2,1,1}`, `strict {_,1,1,1}`. At a **3-byte
  hash all three settings are 1 and therefore identical by construction**, so
  arms that vary `loop.detect` at 3-byte are measuring nothing, and if they
  differ, the simulator has a reproducibility problem rather than a finding.
- `isLooped()` counts how many times *this node's own hash* already appears in
  the packet's path, not whether a hash repeats generally.
- `getPathHashSize() = (path_len >> 6) + 1`, `getPathByteLen() = count × size`,
  which is where the per-hop airtime cost of a wider hash comes from.

**Design a control into the matrix.** Two arms the firmware guarantees are
identical are free reproducibility checks, and one of ours failed, which is how
we learned the seed does not capture all the run-to-run variation.

## Regions come from GeoJSON, and there are saved ones

`~/.config/meshbench/boundaries/*.geojson`; Scotland and Ireland are already
there. `boundary.set` searches for a place, `boundary.accept` adds it to the
chosen set, `boundary.prune` deletes every node outside it. **The chosen set
unions**, so a two-region scenario is two accepts and one prune. Never
hand-roll a lat/lon rectangle: it silently keeps null-island nodes and cuts real
coastline wrong.

The import source is **not persisted between launches** and has to be set each
time: `import.set_source`, `import.fetch`, `import.commit`. ScotMesh's
CoreScope is `https://scotmesh-corescope.mm7roq.compute.oarc.uk`, and it covers
Scotland, northern England *and* Ireland, around 640 nodes, of which some tens
sit at lat/lon 0 and some have no position at all. Say how many you dropped.

## Experiments: the Bench workspace

A matrix, not an A/B. `experiment.vary` **crosses** the arms it already has, so
calling it three times gives the full product; `experiment.base` holds the
constants. Then `experiment.seeds`, `experiment.senders`, `experiment.start`,
poll `experiment.state`, and `experiment.export` writes an HTML report.

**Pin the firmware even when you are not varying it.** `experiment.base` takes
`repeater_version` / `companion_version`. **List the cache rather than trusting
a written list**: `ls ~/.cache/meshbench/firmware/native/`, or ask
`firmware.library`, which now returns `builds` as well as a count. Freshly
imported nodes carry no firmware ref at all, which resolves to MeshCore `main`,
for which nothing is published; a sweep that varies something else then dies on
its first run with "firmware on 0 of N nodes".

**Role is the MeshCore application, not the node kind.** Repeaters run
`simple_repeater`; companions run **`companion_radio`**; room servers run
**`simple_room_server`**. "companion" is not a role, and `firmware.set` used to
take it without complaint; the run then failed minutes later with "<node> runs
no firmware", which reads as a firmware problem and is a typo. The binary names
in the cache are the authority (`meshcore-<role>-linux-amd64`).

**A native version is a per-role release tag, not a bare version.** Upstream
tags one role at a time, and so do our native builds: `repeater-v1.17.0`,
`companion-v1.17.0`, `room-server-v1.17.0`. Asking for `v1.17.0` resolves
nothing and reports "no native builds published for MeshCore v1.17.0", which
points at the release rather than at the string. A scenario mixing roles has to
pin each one separately.

**A companion has no command line.** It speaks the companion protocol over its
serial link, so typing `advert` at one does nothing at all: no error, no
packet. That reads as a mesh dropping the first hop. Use a repeater when a test
needs to originate from a console, or the companion transport when it needs to
be a companion.

**A room server is a repeater that does not forward.** Same console, same admin
password, same place in a scenario; the difference is only on the air. Model one
as a repeater and the result overstates the mesh's reach. `RoomServer` is its
own node kind and has its own fleet-console target.

**`MESHBENCH_NATIVE` may name a directory of per-role builds**, which is what
a mixed-role scenario needs. Naming a single binary overrides every node
regardless of role, so a mesh of repeaters and room servers quietly becomes a
mesh of one application.

**A node that stops answering ticks leaves a log.** Native stderr is at
`~/.cache/meshbench/nodefs/<node>/stderr.log`, and an emulated node's boot
output at `console.log` in its work directory. Read it before theorising: a
stale published build stalling after three seconds and a firmware bug look
identical from the ledger, and the log tells them apart in one line. A current
build prints `radio_init: entering std_init`; its absence means the binary
predates the host radio shim and needs rebuilding upstream.

**Check the radio preset after an import.** Imported nodes take the app default,
`EU/UK (Narrow)`: 869.618 MHz, 62.5 kHz, SF8, CR4/8, which is what ScotMesh
runs, and what the earlier CAD and hash studies used too. Quote it as provenance
rather than assuming; a scenario built by hand may sit on the deprecated
869.525 / 250 kHz / SF10, and results either side of that are not comparable.

**Diff against a scenario that worked.** When a run produces nothing and the
configuration all looks right, load the last project that *did* work and compare
the two JSON files under `~/.config/meshbench/projects/`: radio, regions,
firmware refs, scopes. It is far faster than reasoning forwards, and the answer
is usually one field.

Three traps, all paid for:

- **Arm fields that are pointers are pointers for a reason.** Mode 0 is a real
  value *and* the zero value, so an arm built a field short silently forced
  1-byte hashes while the panel said 3.
- **Constants then arm, for *every* field.** Anything the arm-resolution
  consults directly rather than through the base is a field the operator can set
  and watch be ignored. Firmware was exactly that.
- **Measure the thing the change is supposed to affect.** Reach was a set of
  nodes per message, so an arm that suppressed nine duplicate copies scored the
  same as one that suppressed none: every loop-detection comparison came back
  "no difference" for a whole session before anyone noticed the metric could not
  see it.

## Emulation: running the bytes people flash

Two backends run MeshCore. **Native** compiles it for the host and is what most
of the above is built on. **Emulated** runs the published board image
unmodified, under QEMU or Renode, and exists as the cross-check on the native
one (ADR-0010). A node runs emulated when its `Firmware.Board` is set; empty
means the host build.

**The toolchain is a download now, not a build.** `resource.fetch` with
`kind: "toolchain"` fetches `virtual-sx1262`, `qemu-system-xtensa` and `renode`
into `~/.cache/meshbench/tools/`, which is step three of the lookup a boot
already performs, so nothing has to be set afterwards. The `kind` parameter
**defaults to `softdevice`**, so a fetch that omits it asks for the wrong thing.
`resource.list` says what is present and what it cost; `setup.check` says the
same beside everything else that is missing. QEMU and Renode are published for
linux/amd64 only, macOS gets the chip model alone, and Windows nothing, each with
its reason. An emulated nRF52 board additionally needs the Nordic s140
SoftDevice, which is its own `softdevice` resource.

**`EmulatableBoards()` is the authority on what runs**, and it returns both the
boards that do and the reason each of the rest does not. Do not carry a list in
your head: it has already moved twice. As things stand it covers plain ESP32,
ESP32-S3 under the fork's `esp32s3` machine, and nRF52840 under Renode. **There
is no role gate**: `emulated.Runnable` filters on image format, on whether the
board has verified wiring, and on transport. Anything that once read as "only
repeaters" was board coverage, and board coverage moves.

**BLE companions are excluded deliberately.** There is no Bluetooth here, so one
boots and then waits forever for a phone, which looks like a hang rather than an
unsupported build. Published companion assets carry their transport in the name
(`..._companion_radio_usb-v1.17.0-...`), and the USB one is the only usable one.

Two constraints follow from an emulator being in the loop. It runs on **wall
time**, so the engine cannot race the clock ahead; pace `Run` roughly 1:1 or it
will look as though no frames arrived. And two runs of one seed will not produce
identical ledgers, so **the determinism the rest of the simulator guarantees
does not hold** for a scenario containing one. Run boards one at a time: several
at once will take a twelve-core machine down.

The second is not left to be discovered. `sim.state` answers `reproducible` and
`not_reproducible_why`, `experiment.start` answers the same pair, and the sweep
says it before the run as well as over the results. Read it before quoting one
run's timings against another's: an arm on emulated firmware is worth watching
and is not worth subtracting from another arm.

### Where the pieces live

| | |
|---|---|
| QEMU with our SX1262, GPIO and fixes | `MeshBench/qemu` branch `meshbench-sx1262` |
| Renode with the SEVONPEND fix | `MeshBench/renode` and `MeshBench/tlib` |
| The chip model | `MeshBench/virtual-sx1262`, MIT, its own repository |
| Per-board wiring | `internal/firmware/board/board_<name>.go` |

### The chip model is a submodule, and a submodule does not follow anything

`virtual-sx1262` is pinned by commit wherever it is used, so **a change pushed
to that repository reaches nobody until each consumer's pin is moved.** Nothing
fails when they lag: a consumer keeps building, keeps passing its tests, and
keeps running the old chip. That is the whole hazard - the DIO1 routing fix
existed and did nothing for as long as the pin behind it did not move.

So a change to `MeshBench/virtual-sx1262` is not finished when it merges there.
It is finished when every pin below has been moved and the result has been seen
running:

| consumer | where the pin is | how to move it |
|---|---|---|
| `MeshBench/meshcore-native` | `vendor/virtual-sx1262` | `git -C vendor/virtual-sx1262 fetch && git -C vendor/virtual-sx1262 checkout origin/main`, then commit the new gitlink |

`virtual-sx1262`'s own CI asks GitHub what each consumer pins and says which
ones lag, so the list above is checked rather than remembered - add a row there
in the same commit that adds one here. Keep the two in step; a consumer nobody
wrote down is a consumer that silently stays behind.

**A pin moved is not a pin proven.** The model is the thing every emulated board
talks to, so bump it and then boot one board and watch it forward. Green tests
in `virtual-sx1262` say the chip is right on its own terms; they say nothing
about the firmware above it.

Board wiring is **per board and verified per board**, never inferred from the
MCU. `Heltec_v2` carries an **SX1276**, not an SX1262, despite sitting beside
the V3 in every shop; its firmware speaks SX127x register access, and the
giveaway is the firmware sending `0x42`, which is RegVersion on an SX127x.

### Three things that are not obvious and cost hours

**RadioLib drives NSS as an ordinary GPIO**, not the SPI controller's chip
select. Without NSS the controller clocks bytes one at a time and the chip gets
an unframed byte stream it cannot answer, so the driver reports no chip.

**Arduino's default-constructed `SPIClass` is HSPI**, controller 2, not VSPI.
`std_init(NULL)` picks the global `SPI` (VSPI); `static SPIClass spi;` does not.
Which controller a board's radio sits on is a property of that board's
*firmware*, not of the chip, so read the variant rather than generalising.

**A merged image's flash-size header is at 0x1000**, not 0, because the image
starts with padding. Read it from the wrong offset and you pad to the wrong
size and get `Detected size(4096k) smaller than the size in the binary image
header(8192k)`. QEMU accepts only 2/4/8/16 MB images.

### The chip model is clocked two ways and must agree

`VirtualSX1262` takes a whole buffer for the native path and single bytes for
the emulated one. `variants/host/streaming_test.cpp` holds the two to identical
answers; if they ever diverge, an emulated node is a different radio from a
native one and any comparison measures our code rather than MeshCore's.

That test immediately found `GetPacketStatus` guarding its fields one byte too
strictly, so `signalRssiPkt` read zero on every native run.

## Working on the desktop app

**Never launch it with `go run`.** It relinks the cgo in Gio and wgpu every
time, which costs far more per restart than a prebuilt binary, and that gap
decides whether a layout gets checked or guessed at:

    go build -o msim ./cmd/meshbench && ./msim workbench

**Screenshots need a grabber the compositor supports.** Under Wayland the app
does not render on Xwayland, so an X11 grab captures a solid black screen that
looks exactly like a crashed application. Use the compositor's own tool.

**Look at the window before claiming it works.** One pass over the firmware
library found it taking a viewport of its own and being sized to the display,
two tables with fixed pixel heights that would not scale, and a window created
without `WindowFlagsMenuBar`, which silently costs it `panelChrome`, and with
it the pop-out and dock verbs.

## Before you report any number

**The model is optimistic, and you must say so.** It omits multipath, fading,
body loss, oscillator error, and non-LoRa interference beyond what is loaded.
Every omission makes real links *worse*. The bias is one-directional, and
`docs/shortcomings.md` is the current list.

So: never present a simulated margin as a measurement. "Predicted +2.5 dB, which
is marginal, and real conditions will be worse" is honest. "It works" is not.

## The four rules that make results meaningful

1. **Both directions, always.** Reachability is asymmetric. A result that does
   not say *which direction works* is wrong even when the arithmetic is right.
   "It can hear you but you cannot hear it" is usually the most useful sentence
   available.
2. **Uncertain position, no verdict.** A node imported at ±5 km gets distance and
   bearing, never a reach verdict. Say the position is too uncertain instead of
   producing a confident number from an unconfident input.
3. **One run is not evidence.** The channel is stochastic. Vary the seed and
   report the spread, or say explicitly that you ran once.
4. **Quote provenance.** Firmware ref, seed, terrain zoom, region, radio preset,
   and whether terrain was allowed. A figure without provenance is an anecdote.

## Workflows

### "Will this link work?"

```
meshbench link -from-lat … -from-lon … -to-lat … -to-lon … -freq 869.618
```

It prints the distance, the path loss, the margin in **both** directions, and
which of workable / one-way / fails it is. Report the weaker direction and the
dominant loss term; if it fails, `meshbench profile` over the same path names
the worst obstruction, and "that ridge at 4.2 km costs 31 dB" is actionable
where "no path" is not.

From a session, `link.pair {a, b}` and `link.profile` compute the same thing and
draw it. **They do not hand the numbers back**: the reply is only the endpoints,
`budget.for_selection` answers a bare count, and the margins reach no snapshot
and no client. So read a budget with the CLI, or from the panel, and say plainly
that the socket cannot yet return one rather than inventing a value.

### "Why did that packet not arrive?"

Every miss now carries the cause the engine *established*, on `class`, and it is
a closed set (`internal/sim/engine/events.go`, mirrored into both clients by
`tools/clientgen`). Group on the class; never match on the detail sentence,
which is prose and is reworded.

| class | what it means | what fixes it |
|---|---|---|
| `sent` | this node transmitted it | nothing, it is the origin |
| `received` | decoded, first time | nothing |
| `half-duplex` | its own transmitter was keyed | timing, not power |
| `interference` | would have decoded alone; something louder took it | less traffic, or separation |
| `collision` | header decoded, then more symbols destroyed than the CR repairs | less traffic |
| `receiver-busy` | the demodulator was already following another packet | less traffic |
| `floor` | under the threshold **on its own** | power, antenna, lower SF |
| `unclassified` | the engine did not establish a cause | investigate, do not guess |

**`floor` is no longer the catch-all, and that is the point.** It used to absorb
receiver-lock and collision misses, and an operator reading the floor card
bought antennas for a mesh that was actually too busy. `unclassified` exists so
that an unestablished cause reads as a question rather than as a confident wrong
answer; waveform mode produces it often, because the receive chain reports what
it did and not what beat it. A path with no terrain data is `unclassified` too,
not `floor`: nothing about that signal was measured.

Read them with `events.recent {limit}` or `events.dump {path}`, which writes
NDJSON with `class` on every line. Never say "collision" without checking, and
never average `unclassified` into anything.

### "Where should the next repeater go?"

`internal/study/planning` places sites and scores them, and `meshbench coverage`
writes a raster from one station, but **the site search is reachable from no
verb and no subcommand today**. Say so rather than describing a workflow that
does not exist. What can be done from outside is to propose candidates, compute
a coverage raster for each with `coverage.compute`, and compare, reporting the
per-term consequences rather than one aggregate: a site that gains coverage but
flattens in December is not a site, and a site that adds interference to two
existing repeaters may be a net loss.

### "Will this solar node survive winter?"

`node.energy` (which takes the node's name as its whole parameter, not an
object) and `energy.for_selection` produce the budget, and both refuse when the
energy model is disabled. The answer is not a percentage. It is
*when it flattens and what fixes it*, usually a bigger panel rather than a
bigger battery. Say which, because people reliably buy the wrong one.

### "Did that firmware change break relaying?"

Pin `repeater_version` on `experiment.base`, vary it with `experiment.vary`,
lock `experiment.seeds` and script the traffic with `schedule.add`, or the
comparison means nothing. Report the first diverging event, not just the totals,
and remember that `experiment.export` is the only place the matrix comes back as
a document: `sweep.run` answers with arm and seed counts and leaves the matrix
where only a panel can read it.

### "Is my site deaf?"

Load emitters, then compute the link. If the noise floor is raised, check whether
the interferer is **in band or out of band** before suggesting a filter: a
filter cannot help in-band interference, and saying so plainly saves real money.

## Do not

- Do not report a coverage raster as ground truth. At terrain zoom 11 a pixel is
  ~30 m; buildings, hedges and vans are invisible to it, and the raster charges
  no polarisation mismatch even where the link budget does.
- Do not average away an asymmetry.
- Do not run one seed and call it a result.
- Do not recommend a site without checking energy and interference.
- Do not silently drop nodes excluded for position uncertainty; say how many.
- Do not quote a margin without knowing whether terrain was allowed.

## When the simulator disagrees with reality

`validate.fetch` and `validate.compare` put predictions against observer data.
If agreement is poor, that is a **finding about the model**, not a data problem
to explain away. Report the residual and its sign. ADR-0015 is why the
comparison exists at all: the omissions in `docs/rf-chain.md` section 9 all
make reality worse than simulation, so we expect to read optimistic, and a
residual in the *other* direction is the interesting one, to be investigated
rather than smoothed.
