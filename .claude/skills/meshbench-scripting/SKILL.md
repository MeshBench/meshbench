---
name: meshbench-scripting
description: Write and debug scripts that drive the MeshBench workbench from outside, with the Python, Go or Node client, the control socket, or raw verbs. Use when writing an example, a CI regression check, a soak driver, or anything that opens a session, brings a mesh up and waits for it. Encodes what twenty-four silent failures taught, and the shape of the ones still out there.
---

# Scripting the workbench

Three clients over one line-delimited JSON socket: `pkg/client-python/meshbench`,
`pkg/client-go/meshbench`, `pkg/client-js`. They are peers; none wraps another.
All three ship the same seven examples, one per file or directory, and each is
held to still working by its own suite: `go build ./...` compiles the Go ones,
and `node --test` imports the Node ones. A broken example is a red build rather
than something somebody finds by trying it.

Read `docs/driving-the-workbench.md` for the verb-level story, and
`docs/scripting-verbs.md` for every verb and the call that covers it. This is
about writing something that works.

**Never write down how many verbs there are.** `tools/verbdoc/verbdoc.py`
generates the counts into `docs/scripting-verbs.md` and `docs/scripting-api.md`
and CI runs it with `--check`; ask `session.verbs` at runtime, or cite the
generator. Several numbers typed by hand have already gone stale, and a number
nothing checks is worse than no number.

## The one rule, in two shapes

**A parameter the workbench cannot understand is now refused. Everything else it
declines, it declines by returning a value.**

Refusals travel as `{"id":…, "error":"<the verb's own sentence>", "code":"…"}`.
The code is the closed set in `internal/app/control/codes.go`: `bad_params`,
`not_found`, `conflict`, `unavailable`, `unknown_verb`, `closing`,
`protocol_mismatch`, `unauthorised`. **Branch on the code, never on the prose** -
the message is deliberately the verb's own wording and will be reworded. Python
raises a subclass of `Refused`; Go returns a `*meshbench.Refused` that
`errors.Is` matches against `ErrBadParams` and friends; Node throws a subclass
of `WorkbenchError` that `instanceof NotFound` and friends match, with the
workbench's own sentence on `.detail`.

That half is new, and it closed a whole class: `nodes.select_many` handed
`{"names": […]}` used to match neither branch of a type switch, deselect
everything, and answer `{"selected": null}` as though it had worked. It now
refuses, names the shape it takes, and on an unknown node lists the nodes that
exist. A `nil` parameter still legitimately clears the selection.

The other half has not changed, and it is still where scripts die. Twenty-four
faults were found by running the seven examples end to end and **every one was
silent**. A verb that answers "I did not do that" with a perfectly valid result
turns into a script that sits there doing nothing, and "it hangs" is what gets
reported, about the workbench rather than about the script.

So: **read the reply of anything that looks like a command**, and when a wait
times out, suspect the premise of the wait before you suspect the simulator.

## Terrain is not downloaded until somebody says yes

A fresh machine has never been asked, and the preference
(`terrain_downloads` in `~/.config/meshbench/workbench2.json`) has three states,
not two: allowed, refused, never asked. Until it is answered, the tile store is
offline, `terrain.prefetch` refuses, and the first warm is held.

**A held warm does not hang, which is worse.** It marks the `links` job finished
*and failed* with "waiting for permission to download terrain", then sets the
session warmed so nothing is left in flight. A script that opens a fixture and
calls `wait_idle` therefore returns almost at once, with no jobs running and
**zero links measured**, and every study downstream runs over bare earth, which
is the most optimistic answer available. Nothing raises.

So a script that needs terrain does one of:

- call `terrain.allow` (the `on` parameter defaults to true) and wait for the
  warm it restarts, or
- wait on the `links` job specifically and check its `failed` flag, which is the
  only place the sentence surfaces, or
- open nothing: launch with `-fixture ""` when the measurement does not want a
  network.

There is no environment variable and no CLI flag for this. Headless is not
exempt: `cmd_headless.go` loads the same preferences the window does.

## Bring a run up with the client, never with sim.start

`sim.start` is the play button's own handler. It answers four ways:

| state | what it does | what it returns |
|---|---|---|
| already playing | **pauses** | `{playing: false}` |
| links still warming | nothing | `{playing: false, warming: true}` |
| firmware not up | starts it, **does not play** | `{starting_firmware: true}` |
| otherwise | plays | `{playing: true}` |

Only the last is what a script means, and it starts firmware only when *no*
node is running, so a mesh where you pinned a build onto two nodes is
"started" with the other fifty-six down.

Use `wb.sim.start()` / `Sim.Start(ctx)`, which wait out the warm, call
`firmware.start` for whatever is down, wait for it, then `sim.play`. If you
must drive it by hand, do those three things in that order and check each.

## Waits, and the premises that make them lie

- **Wait for the fixture, not just the socket.** The windowed build opens its
  fixture on a worker so the window appears first. `launch(fixture=…)` waits for
  nodes to exist; anything else you write should too, or `wait_idle` returns in
  0.00s having waited for work nobody had queued.
- **A finished job is sometimes removed and sometimes only marked.**
  `job.progress` with `finished` keeps the row (`infer.run`'s is one of those);
  `job.done` deletes it. Waiting for the list to *empty* waits for ever on half
  of them, which is why every client filters on the flag. `job.list` returns the
  rows with a `running` count and drops finished ones unless you pass
  `all: true`; note that all three shipped clients still read the session snapshot
  rather than that verb, so a change to one is not a change to the other.
- **A finished job is not a successful one.** Check `failed`. A read that could
  not reach the feed, and a warm held for terrain consent, both end the job.
- **Idle is not measured.** A warm that stopped to ask permission to download
  terrain finishes its own job row, so `wait_idle` returns in a moment having
  waited for nothing and no link was measured. `sim.state` answers
  `links_measured`, `warm_held` and `ground`; `wb.sim.start()` /
  `Sim.Start(ctx)` refuse on a held warm rather than carrying on. Every study
  carries its own `ground` block - `state` of `terrain`, `partial` or
  `bare-earth`, with `chosen` saying whether anybody answered the question -
  and a raster over unchosen bare earth is refused outright, because free space
  is the most optimistic answer the model has.
- **Compare like with like.** `firmware.state`'s `nodes` counts nodes that run
  firmware; an SDR observer and an emitter never boot one. Comparing `running`
  against the scenario's size asks for 58 of 58 on a mesh where 56 is
  everything.
- **A seed is not a promise on every scenario.** `sim.state` answers
  `reproducible` and `not_reproducible_why`, and the first is false wherever a
  node runs in an emulator: that node's firmware is stepped by the emulator's
  clock rather than by the run's, so one seed puts its traffic at a different
  instant every time. Measured on one repeater, three runs of one seed put its
  first transmission at 49.8 s, 45.7 s and 55.9 s. A script that diffs two runs, or
  subtracts one arm from another, has to read it first; `experiment.start`
  answers the same pair for a sweep.
- **A diagnostic can cost more than the thing it diagnoses.** `nodes.stats` is
  a `/proc` read per node. Calling it every poll during firmware startup timed
  the socket out. Enrich a wait's message rarely, every ten seconds rather than
  every fiftieth of a second.

## Ask the session what it is missing

`setup.check` is read-only, touches no network, and answers with four groups of
rows, each `{name, state, what, cost, where, do, verb, params}` over the states
`ready | needed | missing | undecided | blocked`. It covers what kind of build
this is, what firmware is installed, whether terrain has been answered, and
which emulator tools are present. A row carries the verb that would fix it, so a
first-run script can read the check and act on it rather than guessing at
`~/.cache`. `needed + undecided > 0` is what makes the workbench open its Setup
page unprompted, three seconds in.

**`session.list` answers what is running before you attach to anything.** In
the clients it is a module or package function rather than a method, precisely
because the question comes before a connection. Liveness is proved by dialling
the address: a unix socket file outlives the process that bound it, and a pid
gets reused, so a script that checks either can attach to a corpse. That is
also the verb to reach for when one run is about to trample another, because
one mesh at a time is a real constraint on this machine.

## Counts where the rows were the question

Five verbs answered with a number and left the rows where only a panel could
reach them. All five are fixed and keep the old count key beside the new list:
`nodes.stats` (`stats`), `firmware.library` (`builds`), `console.read` (`tail`),
`boundary.list` (`areas`), `resource.list` (`resources`).

**Four are still out there**, and the rows genuinely cannot be reached from a
script, because `session.snapshot` publishes counts for exactly these:

| verb | answers | where the rows go |
|---|---|---|
| `budget.for_selection` | `{budgets: n}` | `World.Budgets`, in no summary and in neither client |
| `sweep.run` | `{arms, seeds}` | `World.Matrix`, read by one panel |
| `schedule.add` | `{sends: n}` | `World.Sends`; there is no `schedule.list` |
| `assert.add` | `{assertions: n}` | `World.Assertions`; `assert.check` does return `results` |

The shape is worth recognising: **if a verb's reply is an `int` where you wanted
a list, look for a second key before assuming the data is not there** - and if
there is no second key, say so rather than working around it silently.

## Two consoles, and there is no `send`

A **repeater** reads typed text: `console.type`. A **companion** speaks the
framed protocol and its command line is meshcore-cli's vocabulary:
`console.cli`. Text typed at a companion is echoed locally and goes nowhere,
which looks exactly like a command that ran and did nothing.

The clients route by node kind, so use `node.console`. If you are driving verbs
directly, pick by kind yourself.

The vocabulary is `advert`, `floodadv`, `public <msg>`, `chan <n> <msg>`,
`infos`, `ver`, `contacts`, `sync_msgs`, `set`, `time`. **There is no `send`.**

`console.read` returns the lines under **`tail`**, capped at the last 200;
`lines` is how many there are. Reading `lines` hands you an integer where you
asked for text.

Use `ask()`, not `send()` then `read()`: a node reads its serial input on its
next loop and its loop only runs when the engine steps, so reading straight
after sending reads the moment *before* the command went out.

## Roles: two vocabularies, and only one works

Verbs are keyed on the **application name**, as MeshCore names its example
directory: `simple_repeater`, `companion_radio`, `simple_room_server` (plus
`_usb` / `_ble` for board images). Use the `Role` enum.

The published catalogue spells some of the same things differently:
`repeater`, `room-server`. Those belong to release assets. Pin a build under
one and it is installed, visible, and never run by anything.

`Firmware.Download`'s role is the *asset* name and is deliberately a plain
string. Everything else takes `Role`.

## Enums, not strings

`Kind`, `Board`, `Preset`, `Role`, `Class`, `Tab`, `Strategy`, `Transport` are
generated by `tools/clientgen` from the tree, so all three clients agree and CI
fails when they drift. Never spell one as a free string: a board name nothing
matches produces a different node, silently. The Node client's are frozen
objects of plain strings - `Board.LILYGO_TDECK` - spelled the way Python spells
its members, so a script moved between the two changes the dots and nothing
else.

`Class` is the miss-cause set, and it grew: `sent`, `received`, `half-duplex`,
`interference`, `collision`, `receiver-busy`, `floor`, `unclassified`. Code that
matched on `floor` as the catch-all now silently sees fewer of them. Group an
`events.dump` NDJSON file on the `class` field, never on the detail sentence.

## Running examples on this machine

- **One mesh at a time.** Two 58-node fixtures at once will make the socket
  time out and look like a deadlock. I lost two debugging cycles to this.
- Point `MESHBENCH_BINARY` at the build under test; the clients honour it.
- Unix socket paths are capped at 104 bytes. The scratchpad path is longer than
  that, so let the client choose the address.
- **An emulated board no longer needs a hand-built toolchain.**
  `resource.fetch` downloads `radioserver`, `qemu-system-xtensa` and `renode`
  into `~/.cache/meshbench/tools/`, which is where a boot already looks, so no
  environment variable is needed afterwards. **Pass `kind: "toolchain"`**: the
  parameter defaults to `softdevice`, and a fetch that omits it asks for the
  wrong thing. QEMU and Renode are published for linux/amd64 only; macOS gets
  `radioserver` alone and Windows nothing, and `resource.list` says which with
  a reason.
- A killed run can leave an emulator behind. Check `pgrep -f qemu-system`.
- Firmware roles on disk: `ls ~/.cache/meshbench/firmware/native/`.

## Debugging a script that has stopped

In this order, because each step invalidated a theory today:

1. **What else is running?** `pgrep -af meshbench`, or `session.list`. Load, not
   logic.
2. **Attach and ask.** A second client can attach while the first is stuck:
   `describe`, `firmware.state`, `job.list`, `setup.check`, `nodes.stats`. That
   is how the `56 of 58` and the unfinished warm job were both found in seconds.
3. **Reproduce the exact sequence, timed.** Proving the general case healthy
   says nothing about the specific one. A general responsiveness test said the
   windowed workbench was fine; example 5's actual sequence was not.
4. **Then read the verb.** Not before: twice today the code looked correct and
   the behaviour was not.

## Defects queue behind each other

Today's chain ran four deep: `sim.start` ignored, so firmware was never up, so
traffic never fired, so the assertion could not pass, and the fixture had no
traffic to fire anyway. Fixing the outer one is what makes the inner one
visible, so **"one more fix and it will work" is a bad prediction.** Budget for
the chain.

And the one that passed: example 2 exited 0, printed a cheerful summary, and
had put a local build on a 311-node national network because it decided "first
run" by asking whether the session was empty, and a launched workbench never
is. **A green exit code is not evidence.** Check the numbers say what the script
claims.

## What CI does not cover

- The `race` job runs on tags and on request, not on push. A data race that
  killed the process lived through five green pipelines. Run
  `go test -race ./internal/app/session/` when you touch anything the store
  goroutine and a worker both reach.
- Nothing in CI brings a mesh up and waits for it. Green means it compiles and
  the unit tests pass; it does not mean an example runs.
- The live emulator tests skip unless `MESHBENCH_LIVE=1`, and they still gate on
  `MESHBENCH_QEMU` or `PATH` rather than on the lookup a boot uses. A machine
  set up entirely by `resource.fetch` therefore skips them while being perfectly
  able to run a board.
