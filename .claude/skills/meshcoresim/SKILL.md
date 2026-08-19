---
name: meshcoresim
description: Drive MeshcoreSim to answer RF and mesh-network questions — link viability, coverage, why a packet failed, site selection, solar survival, firmware A/B. Use when asked about MeshCore network behaviour, repeater placement, coverage, or radio settings. Encodes the honesty rules the simulator's own results depend on.
---

# MeshcoreSim

An RF-accurate MeshCore simulator: real firmware, sample-accurate LoRa baseband,
real terrain. Plane project **MSIM**; decisions are ADR-0001…ADR-0018.

Load `plane-conventions` if you are also updating tickets.

## Driving it

It is a **native desktop app**, not a CLI or a service, so it needs a machine
with a display and it has to already be running. You drive the *running* app
over its control socket at
`$XDG_RUNTIME_DIR/meshcoresim.sock`, newline-delimited JSON,
`{"id":1,"method":"<verb>","params":{}}`. `session.describe` lists every verb;
read that before inventing a way to do something, because there is almost always
a verb for it.

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

## Building a scenario from CoreScope — the whole order

Do these in order. Every step below was skipped at least once, and each failure
looks like bad RF rather than a missing step.

1. **Boundaries.** `boundary.set` (place) → `boundary.accept`, once per region.
   The chosen set unions, so Scotland + Ireland is two accepts.
2. **Import.** `import.set_source` → `import.fetch` → `import.commit` with
   `strategy: "replace-all"` (plain `"replace"` is not a strategy name and
   leaves the demo nodes in, on a different preset).
3. **Firmware, per role.** `firmware.set` with `role: "simple_repeater"` for
   everything, then again per companion with `role: "companion_radio"`. Or set
   `repeater_version` / `companion_version` on `experiment.base`.
4. **Regions — `infer.run` then `infer.apply`.** This is the step that gets
   forgotten, and it is the one that decides whether anything relays at all.
5. `firmware.start`, then check `firmware.state` says `running == total`.
6. Only then define and start the sweep.

### The repeater console

`console.type {"node": …, "command": …}` runs a line on a node's CLI and returns
what it said — the fastest way to find out what a node actually believes, rather
than what you think you configured. `get name`, `get repeat`, `get flood.max`,
`get path.hash.mode`, `get loop.detect` all read back; `region put <r>`,
`region allowf <r>`, `region default <r>`, `region save` configure regions.

**The command reference is at <https://docs.meshcore.io/cli_commands/>** — check
it before concluding a setting did not apply. There is no `region list` and no
`help`; both answer `Err - ??`, which looks like a broken node and is just a
command that does not exist.

Console replies come back empty while a sweep is driving the engine — the reply
is collected after a 50 ms step, and the experiment owns the clock. Call
`experiment.stop` first.

### Regions decide whether the mesh relays anything

MeshCore repeaters only forward flood traffic for regions they have been told
about. A freshly imported node has none, so a scoped message is transmitted by
its sender and dropped by all 300 repeaters: **8 transmissions, 0 relays, and a
ledger of 137 events.** That reads exactly like a network with no propagation.

Regions are not in the node API — they are **inferred from days of CoreScope
packet traffic**: `infer.run {"hours": 168}`, poll `infer.result`, then
`infer.apply`. Applying is a separate call and returns how many nodes it
touched; "0 applied" means you inferred and walked away. On ScotMesh a week of
traffic is ~38,000 packets over ~420 nodes and yields `#sco`, `#ioi`,
`#ioi-admin`, `#fif`, `#wls`, `#noc`, `#per`, `#gla`.

### The `#` asymmetry — write the scope with it, the region without

This one cost a whole session. A region is spelled **two different ways** and
both are correct:

| where | form | example |
|---|---|---|
| repeater CLI | **bare** | `region put sco`, `region allowf sco` |
| scope on the wire | **`#`-prefixed** | scope `#sco` |

The key in the packet is `sha256("#sco")[:16]`. Ask a companion to send with
scope `"sco"` and it keys its packets `sha256("sco")` — which matches no
repeater in existence. Every repeater receives the packet, derives a different
key, and declines to forward.

**There is no error anywhere.** The senders transmit, the ledger fills with
"first time this node heard the message", and nothing relays: 8 transmissions,
0 relays, 137 events. It reads exactly like a mesh with no propagation, and the
temptation is to go hunting through RF, firmware roles and regions — all of
which will look correct, because they are.

`experiment.define {"scope": "#sco"}`. The workbench now canonicalises it, but
say it with the `#` anyway.

Then **send on a scope the nodes actually hold**. `experiment.define`'s `scope`
is applied to the senders only; the repeaters relay it or not according to what
inference gave them. Check the holder counts in `infer.result` before choosing —
`#sco` and `#ioi` are the two big ones, and a scope only a handful hold will
look like a dead mesh for the same reason as above.

## Read the firmware before explaining a result

MeshCore is public and the tags match our firmware refs exactly
(`repeater-v1.17.0`, `companion-v1.17.0`). Clone
`github.com/meshcore-dev/meshcore`, check out the tag under test, and read the
code before writing down a mechanism — a plausible story about what the firmware
"probably does" is worth nothing next to twenty lines of it.

Worth knowing, from `examples/simple_repeater/MyMesh.cpp` at v1.17.0:

- `allowPacketForward()` **only ever returns false**. Loop detection cannot
  cause more forwarding; if totals go *up* when you enable it, the cause is
  second-order (timing, congestion, which nodes hear what) and needs measuring,
  not explaining.
- The loop thresholds are **indexed by path-hash size**:
  `minimal {_,4,2,1}`, `moderate {_,2,1,1}`, `strict {_,1,1,1}`. At a **3-byte
  hash all three settings are 1 and therefore identical by construction** — arms
  that vary `loop.detect` at 3-byte are measuring nothing, and if they differ,
  the simulator has a reproducibility problem rather than a finding.
- `isLooped()` counts how many times *this node's own hash* already appears in
  the packet's path — not whether a hash repeats generally.
- `getPathHashSize() = (path_len >> 6) + 1`, `getPathByteLen() = count × size`,
  which is where the per-hop airtime cost of a wider hash comes from.

**Design a control into the matrix.** Two arms the firmware guarantees are
identical are free reproducibility checks, and one of ours failed — which is how
we learned the seed does not capture all the run-to-run variation.

## Regions come from GeoJSON, and there are saved ones

`~/.config/meshcoresim/boundaries/*.geojson` — Scotland and Ireland are already
there. `boundary.set` searches for a place, `boundary.accept` adds it to the
chosen set, `boundary.prune` deletes every node outside it. **The chosen set
unions**, so a two-region scenario is two accepts and one prune. Never
hand-roll a lat/lon rectangle: it silently keeps null-island nodes and cuts real
coastline wrong.

The import source is **not persisted between launches** and has to be set each
time: `import.set_source` → `import.fetch` → `import.commit`. ScotMesh's
CoreScope is `https://scotmesh-corescope.mm7roq.compute.oarc.uk`, and it covers
Scotland, northern England *and* Ireland — around 640 nodes, of which some tens
sit at lat/lon 0 and some have no position at all. Say how many you dropped.

## Experiments: the Bench workspace

A matrix, not an A/B. `experiment.vary` **crosses** the arms it already has, so
calling it three times gives the full product; `experiment.base` holds the
constants. Then `experiment.seeds`, `experiment.senders`, `experiment.start`,
poll `experiment.state`, and `experiment.export` writes an HTML report.

**Pin the firmware even when you are not varying it.** `experiment.base` takes
`repeater_version` / `companion_version`, and builds are cached at
`~/.cache/meshcoresim/firmware/native/` — currently `repeater-` and
`companion-v1.16.0`, `v1.17.0`, and `-faultyirq` variants of both. Freshly
imported nodes carry no firmware ref at all, which resolves to MeshCore `main`,
for which nothing is published; a sweep that varies something else then dies on
its first run with "firmware on 0 of N nodes". The MCP server exposes the same
builds if you would rather ask than list the directory.

**Role is the MeshCore application, not the node kind.** Repeaters run
`simple_repeater`; companions run **`companion_radio`**; room servers run
**`simple_room_server`**. "companion" is not a role, and `firmware.set` used to
take it without complaint — the run then failed minutes later with "<node> runs
no firmware", which reads as a firmware problem and is a typo. The binary names
in the cache are the authority (`meshcore-<role>-linux-amd64`).

**A native version is a per-role release tag, not a bare version.** Upstream
tags one role at a time, and so do our native builds: `repeater-v1.17.0`,
`companion-v1.17.0`, `room-server-v1.17.0`. Asking for `v1.17.0` resolves
nothing and reports "no native builds published for MeshCore v1.17.0", which
points at the release rather than at the string. A scenario mixing roles has to
pin each one separately.

**A companion has no command line.** It speaks the companion protocol over its
serial link, so typing `advert` at one does nothing at all — no error, no
packet. That reads as a mesh dropping the first hop. Use a repeater when a test
needs to originate from a console, or the companion transport when it needs to
be a companion.

**A room server is a repeater that does not forward.** Same console, same admin
password, same place in a scenario; the difference is only on the air. Model one
as a repeater and the result overstates the mesh's reach. `RoomServer` is its
own node kind and has its own fleet-console target.

**`MESHCORESIM_NATIVE` may name a directory of per-role builds**, which is what
a mixed-role scenario needs. Naming a single binary overrides every node
regardless of role, so a mesh of repeaters and room servers quietly becomes a
mesh of one application.

**A node that stops answering ticks leaves a log.** Native stderr is at
`~/.cache/meshcoresim/nodefs/<node>/stderr.log`, and an emulated node's boot
output at `console.log` in its work directory. Read it before theorising: a
stale published build stalling after three seconds and a firmware bug look
identical from the ledger, and the log tells them apart in one line. A current
build prints `radio_init: entering std_init` — its absence means the binary
predates the host radio shim and needs rebuilding upstream.

**Check the radio preset after an import.** Imported nodes take the app default,
`EU/UK (Narrow)` — 869.618 MHz, 62.5 kHz, SF8, CR4/8 — which is what ScotMesh
runs, and what the earlier CAD and hash studies used too. Quote it as provenance
rather than assuming; a scenario built by hand may sit on the deprecated
869.525 / 250 kHz / SF10, and results either side of that are not comparable.

**Diff against a scenario that worked.** When a run produces nothing and the
configuration all looks right, load the last project that *did* work and compare
the two JSON files under `~/.config/meshcoresim/projects/` — radio, regions,
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
  same as one that suppressed none — every loop-detection comparison came back
  "no difference" for a whole session before anyone noticed the metric could not
  see it.

## Emulation: running the bytes people flash

Two backends run MeshCore. **Native** compiles it for the host and is what
everything so far is built on. **Emulated** runs the published board image
unmodified, under QEMU, and exists as the cross-check on the native one
(ADR-0010).

An emulated node is now a node on the mesh. A published `Generic_E22_sx1262`
v1.17.0 image boots, adverts, and a native repeater decodes it off the same
channel (`TestEmulatedAndNativeShareAChannel`). A node runs emulated when its
`Firmware.Board` is set; empty means the host build.

Two constraints that follow from an emulator being in the loop. It runs on
**wall time**, so the engine cannot race the clock ahead — pace `Run` roughly
1:1 or it will look as though no frames arrived. And two runs of one seed will
not produce identical ledgers, so the determinism the rest of the simulator
guarantees does not hold for a scenario containing one.

**Only the repeater role can be emulated today, and it is board coverage rather
than role code.** The one verified board publishes the repeater alone. Every
plain-ESP32 SX1262 board that publishes all three roles is a T-Beam, and all of
them stall *before* `radio_init`: the I2C bus never comes up, the AXP PMU init
spins, and the radio is never touched — zero SPI transactions, which reads as a
broken emulator rather than a missing PMU model. T-Beam SX1262 also publishes a
BLE-only companion. Every board publishing all three with a USB companion is an
ESP32-S3, so the unlock is an `esp32s3` machine, not more board entries.

**BLE companions are excluded deliberately.** There is no Bluetooth here, so one
boots and then waits forever for a phone, which looks like a hang rather than an
unsupported build. Published companion assets carry their transport in the name
(`..._companion_radio_usb-v1.17.0-...`), and the USB one is the only usable one.

### Where the pieces live

| | |
|---|---|
| QEMU with our SX1262, GPIO and fixes | `MeshBench/qemu` branch `meshbench-sx1262` |
| The chip model, and the socket server | `meshcore-native`, `VirtualSX1262` + `bridge/radioserver.cpp` |
| Per-board wiring | `internal/world/scenario/boards.go`, `QEMUWiring` |

Build QEMU with **`--enable-gcrypt`** or the `esp32` machine dies with
`unknown type 'misc.esp32.rsa'`: the RSA device is gated on gcrypt while the
machine references it unconditionally.

    ./configure --target-list=xtensa-softmmu --disable-werror --enable-slirp --enable-gcrypt

### Only plain ESP32, and only some boards

`hw/xtensa/esp32.c` instantiates SPI0 to SPI3, so a radio has a bus to sit on.
**ESP32-S3 models only `spi1`**, the flash controller, so an S3 board needs a
GP-SPI controller written before anything can be attached. Published nRF52
images are linked above a proprietary Nordic SoftDevice and are out of scope.

Board wiring is **per board and verified per board**, never inferred from the
MCU. `Heltec_v2` carries an **SX1276**, not an SX1262, despite sitting beside
the V3 in every shop — its firmware speaks SX127x register access, and the
giveaway is the firmware sending `0x42`, which is RegVersion on an SX127x.
`EmulatableBoards()` returns what can run and why the rest cannot.

### Three things that are not obvious and cost hours

**RadioLib drives NSS as an ordinary GPIO**, not the SPI controller's chip
select. Without NSS the controller clocks bytes one at a time and the chip gets
an unframed byte stream it cannot answer, so the driver reports no chip. The
GPIO model had an empty write handler and had to be implemented.

**Arduino's default-constructed `SPIClass` is HSPI**, controller 2 — not VSPI.
`std_init(NULL)` picks the global `SPI` (VSPI); `static SPIClass spi;` does not.

**A merged image's flash-size header is at 0x1000**, not 0, because the image
starts with padding. Read it from the wrong offset and you pad to the wrong
size and get `Detected size(4096k) smaller than the size in the binary image
header(8192k)`. QEMU accepts only 2/4/8/16 MB images.

### Two bugs in Espressif's QEMU

Both in the SPI model, both affecting any non-flash peripheral:

- **RX bounds check** indexed by the transferred byte's value instead of the
  loop position, so replies were dropped whenever the byte just sent was
  numerically larger than the read length. Already fixed by their open PR #144,
  which is cherry-picked onto our branch with authorship intact.
- **Stale command-phase bitlen**: the USR path enabled a command phase when
  `SPI_USER.COMMAND` *or* a leftover `COMMAND_BITLEN` was set, injecting a
  spurious `0x00` in front of every transfer. Ours, not reported yet.

### The chip model is clocked two ways and must agree

`VirtualSX1262` takes a whole buffer for the native path and single bytes for
the emulated one. `variants/host/streaming_test.cpp` holds the two to identical
answers; if they ever diverge, an emulated node is a different radio from a
native one and any comparison measures our code rather than MeshCore's.

That test immediately found `GetPacketStatus` guarding its fields one byte too
strictly, so `signalRssiPkt` read zero on every native run.

## Working on the desktop app

**Never launch it with `go run`.** It relinks the cgo in Gio and wgpu every
time, which costs far more per restart than a prebuilt binary — and that gap
decides whether a layout gets checked or guessed at:

    go build -o msim ./cmd/meshcoresim && ./msim workbench

**Screenshots need a grabber the compositor supports.** Under Wayland the app
does not render on Xwayland, so an X11 grab captures a solid black screen that
looks exactly like a crashed application. Use the compositor's own tool.

**Look at the window before claiming it works.** One pass over the firmware
library found it taking a viewport of its own and being sized to the display,
two tables with fixed pixel heights that would not scale, and a window created
without `WindowFlagsMenuBar` — which silently costs it `panelChrome`, and with
it the pop-out and dock verbs.

## Before you report any number

**The model is optimistic, and you must say so.** It omits multipath, fading,
body loss, oscillator error, and non-LoRa interference beyond what is loaded.
Every omission makes real links *worse*. The bias is one-directional.

So: never present a simulated margin as a measurement. "Predicted +2.5 dB, which
is marginal — real conditions will be worse" is honest. "It works" is not.

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
4. **Quote provenance.** Firmware ref, seed, terrain zoom, region. A figure
   without provenance is an anecdote.

## Workflows

### "Will this link work?"

```
evaluate_link A B
```
Read the *whole* budget, not the verdict. Report: margin in both directions, the
dominant loss term, and — if it fails — which diffracting edge cost the most.
"That ridge at 4.2 km costs 31 dB" is actionable; "no path" is not.

### "Why did that packet not arrive?"

```
reception_ledger <packet-id>
```
Distinguish the five outcomes; they have different fixes:

| Outcome | What it means |
|---|---|
| out of range | nothing arrived — terrain or distance |
| too weak to demodulate | raise power, lower SF, better antenna |
| demodulated, CRC failed | marginal — interference or collision |
| received, dropped (dedup) | working as designed, not a fault |
| received, relayed | fine |

Never say "collision" without checking the waterfall — a weak signal and a
collision look identical in delivery statistics and have opposite fixes.

### "Where should the next repeater go?"

```
find_sites --region <geojson> --objective coverage,redundancy,energy
```
Report the per-term scores, not the aggregate. A site that gains coverage but
flattens in December is not a site, and a site that adds interference to two
existing repeaters may be a net loss. Both are visible in the terms.

Check `energy_forecast` on the winner before recommending it.

### "Will this solar node survive winter?"

```
energy_forecast <node> --months 12
```
The answer is not a percentage. It is *when it flattens and what fixes it* —
usually a bigger panel, not a bigger battery. Say which, because people reliably
buy the wrong one.

### "Did that firmware change break relaying?"

```
run --firmware-ab <refA> <refB> --seed <n> --traffic scripted
compare
```
Lock the seed and script the traffic, or the comparison means nothing. Report the
first diverging event, not just the totals.

### "Is my site deaf?"

Load emitters, then `evaluate_link`. If the noise floor is raised, check whether
the interferer is **in band or out of band** before suggesting a filter — a
filter cannot help in-band interference, and saying so plainly saves real money.

## Do not

- Do not report a coverage raster as ground truth. At terrain zoom 11 a pixel is
  ~30 m; buildings, hedges and vans are invisible to it.
- Do not average away an asymmetry.
- Do not run one seed and call it a result.
- Do not recommend a site without checking energy and interference.
- Do not silently drop nodes excluded for position uncertainty — say how many.

## When the simulator disagrees with reality

`validate` compares predictions against observer data. If agreement is poor,
that is a **finding about the model**, not a data problem to explain away. Report
the residual and its sign; per ADR-0003 we expect to read optimistic, so a
negative bias is more interesting than a positive one and worth investigating
rather than smoothing.
