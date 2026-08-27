# The scripting API

The design for [#209](https://github.com/MeshBench/meshbench/issues/209): what
a script can do, what it holds while it does it, and what every call is
underneath.

Three documents:

- **this one** — the shape, the connection, and every call, by namespace.
- [scripting-types.md](scripting-types.md) — every object and every field on it.
- [scripting-verbs.md](scripting-verbs.md) — all 213 verbs, what each takes and
  returns, and which call covers it. Nothing is orphaned; the mapping is
  complete in both directions.

Written against `210d9ec`. **This describes what is being built, not what
exists.** Nothing in `pkg/` is written yet — the issues under #209 are the
work, and this is the thing they are held to.

---

## The shape

### Two layers

**Generated bindings.** One typed function per verb, emitted from the manifest
([#213](https://github.com/MeshBench/meshbench/issues/213)), with the verb's
own description as the doc comment. Nobody reads these. They are the floor, and
they are never wrong.

**A hand-written façade.** Objects and methods over the ~60 verbs a script
actually uses. This is where "simple and human readable" lives, and it is small
enough to be written with taste and reviewed line by line.

**The escape hatch is public and documented.** `wb.call(verb, params)` in
Python, `wb.Call(ctx, verb, params)` in Go. Anything the façade does not cover
is one line away, so the façade never has to be complete in order to be useful,
and a verb added tomorrow is usable today.

### Why two layers rather than one

215 addressable methods hand-written across two languages is 430 things to keep
in step. Keeping them in step is what the verb manifest (`docs/verbs.json`) is for: a
hand-written surface that names a verb the tree no longer has is exactly the
drift it catches, where once a tool could call a deleted verb and nothing
failed because there was no schema to compare a surface against.

### Two languages, Go and Python

Go is the reference implementation, because the manifest is Go. Python is the
one people will actually write — MeshCore's tooling is Python, `tools/soak` is
Python, and a firmware developer writing a regression test will reach for
pytest.

A third is deliberately not being written; the reasoning is on #209 and in
[#219](https://github.com/MeshBench/meshbench/issues/219). The generator still
emits two from the start, which is what keeps a third cheap.

They live side by side, as peers: `pkg/client-go/meshbench` and
`pkg/client-python/meshbench`, with runnable examples under `pkg/client-*/examples`.
Neither wraps the other; both speak the socket.

### Naming

Go is `CamelCase` and takes a `context.Context` first; Python is `snake_case`,
synchronous, and uses keyword arguments freely. Only the Python form is spelled
out below unless the two genuinely differ. Both are generated from the same
manifest, so they cannot drift in what they cover — only in how they read.

---

## Connecting

```python
from meshbench import Workbench

Workbench.attach()               # the workbench already running
Workbench.launch(fixture=None)   # start one, own it, close it on exit
Workbench.headless(seed=9001)    # no window, no display, no GPU
Workbench.attach_or_launch()     # for a script run repeatedly by hand
```

```go
wb, err := meshbench.Attach(ctx)
wb, err := meshbench.Launch(ctx, meshbench.Fixture(path))
wb, err := meshbench.Headless(ctx, meshbench.Seed(9001))
```

Every form takes `socket=` / `meshbench.Socket(path)`, because one socket per
user is not enough for CI
([#211](https://github.com/MeshBench/meshbench/issues/211)).

Both clients are context managers / `Close()`-able. `launch` and `headless` own
the process and stop it; `attach` never does — a script must not be able to
close the workbench somebody is looking at by falling off the end.

### The first thing a connection does

Calls `session.hello` ([#212](https://github.com/MeshBench/meshbench/issues/212))
and checks the protocol number. A client older or newer than the build it
connects to fails **here**, with a sentence naming both versions, rather than
halfway through a script with a verb returning something unexpected — which in
a CI run reads as a firmware regression.

`wb.hello` keeps the answer. `wb.hello.mode` is `workbench` or `headless`, and
it is what a script checks before touching `wb.ui`.

### The rule that matters most

**Restarting the workbench loses the scenario.** Nodes, boundary, inference and
firmware assignments live in the running process, not on disk. `wb.hello.pid`
and `started_at` are how a reconnecting script tells a restart from a
reconnect — the first ScotMesh study ran inference against a workbench that had
been rebuilt after the import, and nothing in the state said so.

---

## The namespaces

Every call below names the verb it runs. Where a call is composite — several
verbs, or a verb plus a wait — that is said, because a script author who thinks
one call is one round trip will write a loop that is forty.

### `wb` — the session

| call | verb |
|---|---|
| `wb.describe()` | `session.describe` |
| `wb.status()` | `session.status` |
| `wb.verbs()` | `session.verbs` *(socket-owned)* |
| `wb.snapshot()` | `session.snapshot` *(socket-owned)* |
| `wb.subscribe(*topics)` | `session.subscribe` *(socket-owned; streams, see [Being told](#being-told))* |
| `wb.checkpoint(name)` | `session.checkpoint` |
| `wb.restore(name)` | `session.restore` |
| `wb.checkpoints()` | `session.checkpoints` |
| `wb.say(text)` | `ui.said` |
| `wb.save_run(path)` | `run.save` |
| `wb.quit()` | `app.quit` |
| `wb.log.path` | `log.path` |
| `wb.log.export(path)` | `logs.export` |
| `wb.call(verb, params)` | anything |

### `wb.project`

```python
wb.project.new(place="Fife")          # project.new — an empty network
wb.project.open("fixtures/x.json")    # project.open
wb.project.save("study-a")            # project.save
wb.project.list()                     # project.list
```

`new(place=)` looks the place up the same way a study area is looked up, sets
it as the boundary and frames the map on it — so "Fife" means the same thing
everywhere. Without a place you get a blank network and a map in the middle of
the Atlantic, which is why the argument exists.

### `wb.nodes` and `Node`

The collection is iterable, indexable by name, and filterable:

```python
for n in wb.nodes: ...
wb.nodes["Bishop Hill"]
wb.nodes.of_kind(Kind.COMPANION)
wb.nodes.running
wb.nodes.selected
len(wb.nodes)
```

Imported names carry emoji and accents — `🏔️ West Lomond 📡` is one real node
— so a name is something you search for rather than type:

```python
wb.nodes.search("west lomond")       # -> [NameMatch], best first, with a score
wb.nodes.find("west lomond")         # -> Node, or a refusal naming the near misses
wb.nodes.near(node, 12)              # -> the twelve closest, nearest first
```

Ranking happens at the workbench so both clients agree which result is the top
one, and `find` refuses below a score of 0.5 rather than handing back the
nearest thing — taking the top result unconditionally is how a script adverts
from a node that merely shared a word with the query, silently.

```python
deck = wb.nodes.place("Deck", kind=Kind.COMPANION, lat=56.19, lon=-3.17,
                      height_m=10, tx_dbm=22, board=Board.LILYGO_TDECK)
wb.nodes.place_many([{"name": "R1", "kind": Kind.SIMPLE_REPEATER, ...}, ...])
wb.nodes.delete("R1", "R2")          # nodes.delete_many, all or none
wb.nodes.keep("Glenrothes", deck)    # names or handles
wb.nodes.select("R1", "R2", add=False)
wb.nodes.stats()
```

A placed node inherits its neighbours' regions and their firmware, because
somebody dropping a repeater on the map is adding a repeater to *this* network,
not choosing a firmware strategy. `place_many` does one warm at the end rather
than one per node; `keep` and multi-`delete` are single verbs, all-or-none, so
a name that is not there refuses and removes nothing — half a deletion leaves a
scenario nobody described and no way to tell which half survived. Each warm
cancels the one before it so the cost is one measurement of the matrix, not N.

The `Node` object's fields and methods are in
[scripting-types.md](scripting-types.md#node). The ones a script reaches for
most:

```python
node.start(); node.stop()
node.firmware = build          # node.set_firmware: stop, provision, restart
                               # build carries version, board and role together
node.set_firmware(b, apply=False)   # node.set_firmware_only: at next start
node.move(lat, lon)
node.regions = ["ScotMesh"]
node.true_rf = True
node.wait_running(timedelta(minutes=3))
node.inject()                  # originate without firmware
node.delete()
```

`node.firmware = build` **restarts the node.** That is what the verb does — stop,
provision, start — and it is said here because a script that pins firmware
mid-run has restarted a node mid-measurement.

### `wb.sim`

```python
wb.sim.start()                 # wait out the warm, start every node, then play
wb.sim.play() / .pause() / .toggle() / .step()
wb.sim.run(timedelta(minutes=10))   # sim.run, then wait for it to finish
wb.sim.settle(steps=20)        # sim.settle
wb.sim.reset()
wb.sim.seed = 9001
wb.sim.step_ms = 10
wb.sim.real_firmware = True
wb.sim.state()
```

`run()` is the composite one: it calls `sim.run` and then waits. Waiting is
polling `sim.state` today and a subscription once
[#214](https://github.com/MeshBench/meshbench/issues/214) lands, and no script
changes when it switches — which is why events are sequenced *after* the
clients.

`start()` is the other composite, and it is deliberately **not** one call to
`sim.start`. That verb is the play button's own handler and answers four ways:
it *pauses* if the run is already playing, declines while links are still being
measured, starts firmware and does **not** play, or plays. None of those is an
error, so a script that pressed it once and moved on waited for firmware
nothing had been asked to start — which reads as a hang in the workbench. It
also starts firmware only when *no* node is running, so a mesh where two nodes
were pinned individually is "started" with the rest down.

So `start()` waits out the warm, calls `firmware.start` for whatever is not up,
waits for it, and then calls `sim.play`, which cannot pause.

`wb.sim.playing`, `now_ms`, `realtime_x`, `firmware_running` are live reads off
the snapshot and cost nothing.

**Determinism is the point.** Same seed, same scenario, same result. A script
that does not set a seed gets the fixture's, and `Provenance` carries whichever
it was.

### `wb.firmware`

```python
wb.firmware.library                       # firmware.library
wb.firmware.installed                     # firmware.installed
wb.firmware.scan()                        # firmware.published
wb.firmware.needed()                      # firmware.needed
wb.firmware.download("companion", "v1.17.0")     # -> Job
wb.firmware.import_(path, role="repeater")
wb.firmware.build("~/src/MeshCore")       # NEW, #216 -> {role: Build}
wb.firmware.use("companion-v1.17.0", role="companion")
wb.firmware.start()                       # firmware.start -> Job
wb.firmware.wait_started(timedelta(minutes=5))
wb.firmware.delete(build); wb.firmware.wipe()
```

**Firmware comes from `meshcore-native`, not from a hand-rolled build.** A
locally built 1.16.0 compiled against a stale shim once answered console output
with `0x06` where the host expects `0x07`: it connected, misbehaved and exited.
Two arms of a comparison speaking different wire protocols measure the shim,
not the firmware. `download` fetches published builds; `build` (#216) builds a
checkout the same way for every role in one call, so both arms are built alike.

`firmware.start` is **asynchronous** and always has been. It returns
`{starting, done, total}`; on 155 nodes it once froze the window and the socket
together, which read as a crash and was reported as one. The façade returns a
`Job` and `wait_started` polls `firmware.state`.

### `node.console` and `wb.fleet`

```python
node.console.send("advert")
node.console.read()          # the scrollback, newest last
reply = node.console.ask("get region")
wb.fleet.send("set region ScotMesh", kind="repeater")   # -> [FleetReply]
```

`ask` is the important one. **A node answers on its next loop, and its loop
only runs when the engine steps.** Reading straight after sending reads the
moment before the command was sent — every script that has done this by hand
got an empty reply and concluded the console was broken. `ask` sends, gives the
mesh its own time, and then reads.

**There are two consoles, and which you get depends on what the node is.** A
repeater reads typed text, over `console.type`. A companion does not: it speaks
the framed companion protocol, and its command line is meshcore-cli's
vocabulary, over `console.cli` — `advert`, `floodadv`, `public <msg>`,
`chan <n> <msg>`, `infos`, `ver`, `contacts`, `sync_msgs`, `set`, `time`.

Text typed at a companion is echoed locally and goes nowhere, which looks
exactly like a command that ran and did nothing. `node.console` picks the right
one from the node's kind, so a caller never has to know — and **there is no
`send` command** in either vocabulary, which is what several scripts assumed
before anybody ran them.

`fleet.send` warns when the command changes what the nodes *are* — `region`,
`set`, `reboot`, `clock`, `advert`, `erase`, `reset` — because a region added
halfway through a run makes the second half a different mesh. The façade
surfaces that warning as a field on the result, not a log line.

### `node.companion`

```python
c = node.companion
c.connect()
c.send("hello", channel=0)
c.advert(flood=True)
c.messages(channel=0)
c.contacts; c.channels
c.scope = "ScotMesh"
c.configure(name="Deck", lat=..., lon=..., tx_dbm=...)
c.cli("contacts")                     # meshcore-cli's vocabulary
c.raw(b"\x01\x02")
```

`c.path_hash_bytes == 0` means the node has not said — firmware older than v10
does not report it, and guessing 1 there would be a confident wrong answer.

### `node.device` — driving a running board

```python
d = node.device                      # distinct from node.board, the model name
d.screen()                           # board.screen  -> what it is showing, as numbers
d.screenshot()                       # board.screenshot -> a PNG, path returned
d.press(pin, down=True); d.press(pin, down=False)   # board.press, held
d.tap(pin)                           # press and release
d.type("hello")                      # board.key
d.touch(120, 200); d.tap_at(120, 200)               # board.touch
d.radio()                            # node.radio, on the node itself
new = d.wait_screen(timeout=timedelta(seconds=30))  # block until the frame changes
```

All of it works **headless**: the screen it reads is the framebuffer the
controller holds, not a picture of anybody's desktop, so a board test runs in
CI without a display. `node.device` is the running hardware; `node.board` is
the model name that hardware is (`LilyGo_TDeck`), and the two are deliberately
separate. Serial and the emulator's own output are read through
[`node.output`](#node-console-and-wb-fleet), which every board and every native
node has.

Held rather than clicked, because the firmware cares: MeshCore wakes a sleeping
display on a press and powers the board off on a long one — so `press` takes a
`down`, and `tap` is the press-and-release for when the hold does not matter.

**Half duplex eats stimuli.** A board handed a packet while it is transmitting
never hears it. A script that taps a button and immediately reads the screen
will intermittently read the frame from *before* the tap landed;
`wait_screen(timeout=)` is the honest way to do it — it blocks until the frame's
digest changes, so a redraw that keeps the same number of lit pixels still
counts. Capturing an arbitrary desktop *window* (as opposed to a board's own
display) is not here yet: it needs OS-level capture rather than a headless
verb, and is tracked separately.

### `wb.events` and `wb.packets`

```python
wb.events.recent(limit=500)          # events.recent
wb.events.counts                     # from the snapshot
wb.events.dump("round.ndjson")       # events.dump
p = wb.packets.open(event.packet_id) # packet.open
p.ledger                             # why each receiver did or did not get it
p.journey                            # the message across every relay
wb.packets.close()
```

The event log is a **tail**, and a long run has millions. Anything that needs
all of them dumps per round, the way `tools/soak` does — reading only
`events.recent` after a busy flood samples the most congested moment of it, and
that mistake has already been made once here.

`p.ledger` is the type a test reaches for when it wants to say *why* nothing
arrived. It is what makes a scripted assertion able to fail with a diagnosis.

### `wb.links` and `wb.study`

```python
wb.links.recompute()                 # links.recompute — a warm
wb.links                             # the measured pairs
wb.links.pair("A", "B")              # link.pair -> Link
wb.links.profile("A", "B")           # link.profile -> Profile
wb.links.budget()                    # budget.for_selection -> [Budget]

wb.study.margin_km = 25
wb.study.coverage(node="Bishop Hill") # -> Coverage
wb.study.coverage(mode="combined")
wb.study.coverage_cells = 800
wb.study.energy(node=)                # -> Energy
wb.study.plan("A", "B")               # -> [Route]
```

**Both directions, always.** `Link` carries `AtoB`, `BtoA` and the weaker
`MarginDB`, and any helper that formats a link prints both. A result that does
not say which direction is wrong even when the arithmetic is right.

A warm on a national network is minutes and 48,000 terrain profiles. Every call
here that triggers one returns a `Job`, and `wb.links.recompute().wait()` is
how a script waits for it rather than guessing at a sleep.

### `wb.boundary` and `wb.live`

```python
wb.boundary.use("Fife")              # a place name, or a path to GeoJSON
wb.boundary.use("bounds/tay.geojson")
wb.boundary.load(polygon, name="Tay catchment")   # a dict, a document, a path
wb.boundary.list()                   # what the study area is made of
wb.boundary.prune(margin_km=25)      # -> how many nodes went

wb.live.pull(url)                    # fetch, commit, infer, apply - all four
wb.live.fetch(url)                   # -> ImportPreview, changing nothing
wb.live.commit(Strategy.REPLACE)
wb.live.infer(timedelta(days=7))     # reads the feed's own past
wb.live.apply_regions()              # -> how many nodes took one
```

**Order is load-bearing**, and the façade enforces it rather than documenting
it: the boundary goes in **before** the import, because the import filters at
fetch time and a boundary set afterwards prunes rather than filters — having
already paid to fetch nodes it will discard. `commit` refuses until a preview
exists; the façade turns that refusal into a sentence saying which step is
missing.

`pull` runs all four steps because the last two are the ones that get skipped,
and skipping them does not fail: the mesh comes up with regions inferred but
never applied, which transmits, relays nothing, and reports no error at all.
It reads as bad RF.

`boundary.use` takes either a place name — searched for at Nominatim, so it
needs the network and needs the area to have an administrative name — or a
path to GeoJSON, which is the only way to study a catchment, a valley, or
something drawn in QGIS this morning, and the only one that works offline.

### `wb.experiment` and `wb.sweep`

```python
wb.experiment.define(run_for_ms=..., send_at_ms=..., arms=[...], seeds=[...])
wb.experiment.vary("rxdelay.base", [0, 100, 200])
wb.experiment.seeds = [1, 2, 3, 4, 5]
wb.experiment.start()                # -> Job
wb.experiment.state()                # -> [RunRow], watchable while it runs
wb.experiment.results(arm=)          # -> [ArmSummary]
wb.experiment.compare("a", "b")      # -> Comparison
wb.experiment.export(path)
```

`compare` **refuses to name a winner when `RXSpread` is zero across the arms**,
because that is one draw repeated rather than a spread, and a difference cannot
be called larger than a noise nobody has measured. It returns the numbers and
says so; it does not return a verdict it cannot support.

### `wb.validate`

```python
wb.validate.fetch(url, hours=24)     # -> Job, real receptions
wb.validate.compare()                # -> Residuals
wb.validate.calibrate()              # fit the excess-loss term
wb.validate.uncalibrate()
```

`Residuals` is a bias and a spread, never a pass or a fail: "3 dB optimistic on
this network" is something somebody can correct for, and "validation failed" is
not.

### `wb.rf` and `wb.radio`

```python
wb.rf.mode = "waveform"              # or "calculated"
wb.rf.realism(osc_ppm=2, multipath_db=6, fading_hz=1)
wb.rf.excess_loss_db = 8
wb.rf.environment = "/path/to/tiles"
wb.rf.environments                   # environ.list
wb.radio.presets                     # 20 community presets
wb.radio.apply("EU/UK (Narrow)", node=None)
```

**The simulator is kinder than the air.** With every realism switch at zero
there is no multipath, no body loss and no oscillator error, and the measured
biases are nearly all in one direction — which is what makes a result usable:
treat it as a best case. Every one of these settings lands in `Provenance`, so
a number a script prints carries the model it came from.

Changing a preset changes the frequency, which invalidates the cached path
loss: the façade returns the `Job` for the re-warm rather than letting a script
measure against a stale matrix.

### `wb.capture`, `node.serve`

```python
wb.capture.start("run.pcapng"); wb.capture.stop()
wb.capture.wireshark()
wb.capture.waterfall(node)           # -> an image
ep = node.serve("tcp")               # -> Endpoint; point meshcore-cli at it
node.unserve()
src = node.serve_sdr()               # observers: an rtl_tcp source
```

A waterfall taken at an arbitrary moment is a picture of noise — a LoRa packet
is tens of milliseconds and the channel is idle between them. `waterfall`
retries until the channel has something on it, rather than returning a correct
and useless answer.

`Endpoint.addr` is the machine's own address, never the `0.0.0.0` a listener
was bound to. `endpoint.addrs` is every address it answers on, because which
one the far end can reach is not this program's to guess.

### `wb.schedule`, `wb.assertions`, `wb.provisioning`

```python
wb.schedule.add(node="C2", command="send hello", at=0, every=20)  # seconds
wb.schedule.clear()

wb.assertions.add("delivered", at_least=40)
wb.assertions.add("sent", node="R1", at_most=12)
report = wb.assertions.check()
report.write_junit("results.xml")

wb.provisioning.settings
wb.provisioning.set(loop_detect=..., cad=..., advert_hops=...)
wb.provisioning.apply()
node.provisioning                    # the exact lines this node is told
```

Repeating traffic is `schedule.add(every=)` and has worked all along — it is
example 3's "every twenty seconds" exactly, and it is undocumented, which to a
script author is the same as missing.

**An assertion whose kind this build does not understand fails.** It does not
quietly pass. A green run that checked nothing is the worst outcome available
here.

### `wb.boards`, `wb.resources`, `wb.terrain`, `wb.gpu`, `wb.jobs`

```python
wb.boards.list(); wb.boards.matrix(version); wb.boards.probe(board, version)
wb.resources; wb.resources.fetch(kind, name, version)
wb.resources.licence(kind, name, version)
wb.terrain.cache_gb = 10; wb.terrain.cache_dir = path; wb.terrain.prefetch()
wb.gpu; wb.gpu.enabled = True
for job in wb.jobs: job.cancel()
```

`wb.gpu.used` is what the **last warm** actually did, not the setting. "GPU
acceleration: on" over a run that quietly fell back to the cores is the claim
this project does not make.

**Board probes run one at a time.** The façade serialises them and says so; a
script that fans them out takes a twelve-core machine down, and the failure
looks like flakiness rather than resource exhaustion.

### `wb.ui` — windowed sessions only

`None` when `wb.hello.mode == "headless"`, so a script fails at the client with
a clear sentence rather than at the socket.

```python
wb.ui.view = "Run"                   # Plan Run Debug Validate Bench App
wb.ui.panel("Nodes running").open()
wb.ui.panel("Map").pop_out()
wb.ui.map.fit(); wb.ui.map.centre(node="Deck", zoom=12)
wb.ui.map.layers["Coverage"] = True
wb.ui.map.tool = "place"
wb.ui.layouts.save("study"); wb.ui.layouts.load("study")
wb.window(node, tab="Hardware")      # node.window — tab= needs #216
```

Those 23 verbs are already gated on one check and already refuse with *"this
session has no interface attached, so there is nothing to show"*. That is most
of what [#215](https://github.com/MeshBench/meshbench/issues/215) asks for and
is worth knowing before estimating it.

---

## Waiting

Every wait is a method, never a sleep in a script.

```python
node.wait_running(timedelta(minutes=3))
wb.firmware.wait_started(timedelta(minutes=5))
wb.sim.run(timedelta(minutes=5))      # returns when the run has finished
job.wait(timedelta(minutes=20))
wb.events.wait(kind="rx", to="Glenrothes", timeout=timedelta(seconds=60))
node.device.wait_screen(timeout=timedelta(seconds=30))
```

They poll today. The events they will move onto are landing
([#214](https://github.com/MeshBench/meshbench/issues/214)) — see *Being told*
below — and **no script changes when a wait moves from polling to a
subscription**, which is the whole reason the clients were sequenced before the
events.

Every timeout is required to be explicit or to have a documented default, and
every one that expires raises with what it was waiting for and what the state
actually was — not `TimeoutError`. `tools/soak/soak.py` hand-writes this loop
three times in 72 lines, each with its own interval and its own timeout, and
each one is a chance to sample the wrong moment.

**Two clocks, and they are not the same one.** The length of a run is the
mesh's: `sim.run(timedelta(minutes=5))` is five minutes of its time, and on 155
emulated nodes that is a great deal more than five of yours. Every *timeout* is
yours. Nothing that means simulated time is called `timeout`, and every
duration is the language's own type — `datetime.timedelta` and
`time.Duration` — rather than a string mini-language nothing completes and
nothing checks.

**A wait is only as good as its premise.** Three of them were wrong at once
before anybody ran an example: one waited for a job list to empty when half the
jobs are marked finished rather than removed; one compared running firmware
against every node, including an SDR observer and an emitter that never boot
one; and one returned instantly because it was called before the fixture had
been opened. All three read as the workbench hanging. When a wait expires,
suspect what it is comparing before you suspect the simulator.

---

## Being told

The socket is request/reply, and stays that way: a script sends a verb and reads
its answer. A subscription is the other shape — the workbench writing a line the
moment something changes, rather than a script asking again — and it does not
fit a call, so it is given a connection of its own to stream on. A client that
never subscribes speaks exactly the request/reply protocol it always did.

```python
with wb.subscribe("status", "snapshot") as stream:
    for note in stream:                 # blocks until the next event
        print(note.topic, note.data)
```

```go
sub, err := wb.Subscribe("status", "snapshot")
for e := range sub.Events() { use(e.Topic, e.Data) }
```

Each notification is `{"event": ..., "data": ...}` with **no id**. The absent id
is the whole distinction: a reply carries the id it answered, a notification
never does, so a stream and a reply can never be mistaken for one another —
which is why a subscription is opened on its own connection rather than
multiplexed onto the calling one.

Topics today:

| Topic | Sent | Data |
|---|---|---|
| `status` | a line was added to the session log | `{"line": ...}` |
| `snapshot` | after a publish, **coalesced** | `{"seq", "now_ms", "playing", "run_until_ms", "nodes", "jobs"}` |

**Coalescing is the server's job, not the client's.** A run publishes a snapshot
every tick, which on a large network would flood a slow reader, so at most one
snapshot notification is sent per connection every 200 ms; the count that were
dropped in between rides along on the next one as `dropped`. `status` is never
dropped — a log line missed is a log line gone.

The `wait_*` methods above still poll today; the subscription is the mechanism
they move onto, and **no script changes when they do** — a wait is a wait
whether it asked or was told.

---

## Checkpoints

```python
wb.checkpoint("before the flood")     # freeze the whole session under a name
wb.checkpoints()                      # ["before the flood", ...]
r = wb.restore("before the flood")    # rebuild and replay back to that moment
```

A checkpoint is the whole session frozen: the network, how it is being run
(seed, RF mode, realism, calibration, real-firmware or the fast model), and
where the clock had got to. Restoring one takes the mesh **back to that exact
moment** — and it does so by rebuilding the session and **replaying
deterministically** to the checkpoint's time, not by thawing a saved image.

That is the honest mechanism, and it follows from *determinism is a feature*:
same seed, same scenario, same result, so there is nothing to store that the
seed does not already reproduce — not the firmware's own RAM, not the waveforms
in flight. The cost is that the replay runs in **the mesh's own time**: restore
returns as soon as the replay is under way (`replaying: true`, `target_ms` set),
and the sim reaching `target_ms` is when it has actually arrived. Restoring a
long run therefore takes the run's own length, shown as a run in progress — an
*instant* restore would have to freeze the emulators mid-write, which a native
firmware process cannot do at all.

Checkpoints live in `~/.config/meshbench/checkpoints/`. A name is a label, not
a path; `restore` also accepts an explicit `{path}` for a file kept elsewhere.

---

## Errors

One exception hierarchy, mapped from the codes in #212.

| code | Python | Go | means |
|---|---|---|---|
| `unknown_verb` | `UnknownVerb` | `ErrUnknownVerb` | not in this build — check `wb.hello.version` |
| `bad_params` | `BadParams` | `ErrBadParams` | the verb refused what it was given |
| `not_found` | `NotFound` | `ErrNotFound` | no node, build or area of that name |
| `conflict` | `Conflict` | `ErrConflict` | wrong state: no simulation, nothing running, no preview yet |
| `unavailable` | `Unavailable` | `ErrUnavailable` | headless, or no hardware for it |
| `internal` | `WorkbenchError` | `ErrInternal` | |
| `closing` | `Closing` | `ErrClosing` | the workbench is shutting down |

The message stays as the verb wrote it. The verbs in this tree write good
prose — *"no node is running firmware, so there is nothing to send to"* — and a
client that replaced that with a code would be making the experience worse.

Client-side, before the wire: `NotConnected`, `ProtocolMismatch`, `Timeout`.

---

## Honesty

Anything that returns a measurement returns a `Provenance` with it: RF mode,
realism switches, the excess-loss term and whether it was fitted or defaulted,
the building environment, whether the fixture is permissive, and the seed.

This is not decoration. A script's output gets pasted into a report with the
caveats stripped, so **the caveats have to be in the value**, not beside it.
`str(provenance)` is a one-line summary meant to be printed above any number a
script emits, and the pytest plugin puts it in the failure output whether or
not the test asked for it.

A permissive fixture answers a reach question more generously than the real
network. `provenance.permissive` is true then, and every reporting helper says
so — loudly, every time, the way the CLI already does.

---

## What has deliberately no façade

28 verbs. They are the store talking to itself: a warm publishing its matrix, a
fetch reporting a failure, a worker retiring its own progress row. They are
listed with their reasons in
[scripting-verbs.md](scripting-verbs.md), and they stay reachable through
`wb.call` — hidden from the façade, never hidden from the wire.

The line is: **if calling it from outside would corrupt the session or means
nothing to a caller, it gets no façade.** `job.progress` is the clearest case;
what a script wants is `wb.jobs`, and a script that calls `job.progress` is
inventing work that did not happen.

---

## What this design depends on

| dependency | issue |
|---|---|
| A socket per instance | [#211](https://github.com/MeshBench/meshbench/issues/211) |
| `session.hello`, error codes | [#212](https://github.com/MeshBench/meshbench/issues/212) |
| The manifest and its parity test | [#213](https://github.com/MeshBench/meshbench/issues/213) |
| Events, so waits stop polling | [#214](https://github.com/MeshBench/meshbench/issues/214) |
| Headless | [#215](https://github.com/MeshBench/meshbench/issues/215) |
| Board on place, node-window tab, build-from-checkout, bulk node ops | [#216](https://github.com/MeshBench/meshbench/issues/216) |

Six calls in this document are marked as needing #216 and do not work without
it: `nodes.place(board=)`, `node.board = `, `wb.window(tab=)`,
`wb.firmware.build()`, `wb.nodes.keep()` as one call, and bulk
`wb.nodes.delete()`. Everything else maps onto a verb that exists today.
