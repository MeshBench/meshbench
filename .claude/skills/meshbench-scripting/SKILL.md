---
name: meshbench-scripting
description: Write and debug scripts that drive the MeshBench workbench from outside — the Python or Go client, the control socket, or raw verbs. Use when writing an example, a CI regression check, a soak driver, or anything that opens a session, brings a mesh up and waits for it. Encodes what twenty-four silent failures taught, and the shape of the ones still out there.
---

# Scripting the workbench

Two clients — `clients/python/meshbench` and `clients/go/meshbench` — over a
line-delimited JSON socket. They are peers; neither wraps the other. Examples
live beside each, one per directory, and `go build ./...` compiles the Go ones
so a broken example is a red build rather than something somebody finds by
trying it.

Read `docs/driving-the-workbench.md` for the verb-level story. This is about
writing something that works.

## The one rule

**The workbench answers "no" by returning a value, not by raising.**

Twenty-four faults were found by running the seven examples end to end. Every
single one was silent. Not one raised. A script that ignores a reply turns a
clear refusal into a script that sits there doing nothing, and "it hangs" is
what gets reported — about the workbench, not about the script.

So: **read the reply of anything that looks like a command**, and when a wait
times out, suspect the premise of the wait before you suspect the simulator.

## Bring a run up with the client, never with sim.start

`sim.start` is the play button's own handler. It answers four ways:

| state | what it does | what it returns |
|---|---|---|
| already playing | **pauses** | `{playing: false}` |
| links still warming | nothing | `{playing: false, warming: true}` |
| firmware not up | starts it, **does not play** | `{starting_firmware: true}` |
| otherwise | plays | `{playing: true}` |

Only the last is what a script means, and it starts firmware only when *no*
node is running — so a mesh where you pinned a build onto two nodes is
"started" with the other fifty-six down.

Use `wb.sim.start()` / `Sim.Start(ctx)`, which wait out the warm, call
`firmware.start` for whatever is down, wait for it, then `sim.play`. If you
must drive it by hand, do those three things in that order and check each.

## Waits, and the premises that make them lie

- **Wait for the fixture, not just the socket.** The windowed build opens its
  fixture on a worker so the window appears first. `launch(fixture=…)` now
  waits for nodes to exist; anything else you write should too, or `wait_idle`
  returns in 0.00s having waited for work nobody had queued.
- **`wait_idle` ignores finished jobs.** Some jobs are removed when they end
  and some are only marked. Waiting for the list to *empty* waits for ever on
  half of them.
- **Compare like with like.** `firmware.state`'s `nodes` counts nodes that run
  firmware; an SDR observer and an emitter never boot one. Comparing `running`
  against the scenario's size asks for 58 of 58 on a mesh where 56 is
  everything.
- **A diagnostic can cost more than the thing it diagnoses.** `nodes.stats` is
  a `/proc` read per node. Calling it every poll during firmware startup timed
  the socket out. Enrich a wait's message rarely — every ten seconds, not every
  fiftieth of a second.

## Two consoles, and there is no `send`

A **repeater** reads typed text: `console.type`. A **companion** speaks the
framed protocol and its command line is meshcore-cli's vocabulary:
`console.cli`. Text typed at a companion is echoed locally and goes nowhere,
which looks exactly like a command that ran and did nothing.

The clients route by node kind, so use `node.console`. If you are driving verbs
directly, pick by kind yourself.

The vocabulary is `advert`, `floodadv`, `public <msg>`, `chan <n> <msg>`,
`infos`, `ver`, `contacts`, `sync_msgs`, `set`, `time`. **There is no `send`.**

`console.read` returns the lines under **`tail`**; `lines` is how many there
are. Reading `lines` hands you an integer where you asked for text.

Use `ask()`, not `send()` then `read()`: a node reads its serial input on its
next loop and its loop only runs when the engine steps, so reading straight
after sending reads the moment *before* the command went out.

## Roles: two vocabularies, and only one works

Verbs are keyed on the **application name**, as MeshCore names its example
directory: `simple_repeater`, `companion_radio`, `simple_room_server` (plus
`_usb` / `_ble` for board images). Use the `Role` enum.

The published catalogue spells some of the same things differently —
`repeater`, `room-server`. Those belong to release assets. Pin a build under
one and it is installed, visible, and never run by anything.

`Firmware.Download`'s role is the *asset* name and is deliberately a plain
string. Everything else takes `Role`.

## Counts where the rows were the question

Four verbs answered with a number and left the rows in the snapshot where only
a panel could reach them: `nodes.stats`, `firmware.library`, the study area,
and `console.read`. All four are fixed, and the shape is worth recognising —
if a verb's reply is an `int` where you wanted a list, check for a second key
before assuming the data is not there.

## Enums, not strings

`Kind`, `Board`, `Preset`, `Role`, `Class`, `Tab`, `Strategy`, `Transport` are
generated by `tools/clientgen` from the tree, so both clients agree and CI
fails when they drift. Never spell one as a free string: a board name nothing
matches produces a different node, silently.

## Running examples on this machine

- **One mesh at a time.** Two 58-node fixtures at once will make the socket
  time out and look like a deadlock. I lost two debugging cycles to this.
- Point `MESHBENCH_BINARY` at the build under test; both clients honour it.
- Unix socket paths are capped at 104 bytes. The scratchpad path is longer than
  that, so let the client choose the address.
- A killed run can leave a QEMU emulator behind. Check `pgrep -f qemu-system`.
- Firmware roles on disk: `ls ~/.cache/meshbench/firmware/native/`.

## Debugging a script that has stopped

In this order, because each step invalidated a theory today:

1. **What else is running?** `pgrep -af meshbench`. Load, not logic.
2. **Attach and ask.** A second client can attach while the first is stuck:
   `describe`, `firmware.state`, `jobs`, `nodes.stats`. That is how the
   `56 of 58` and the unfinished warm job were both found in seconds.
3. **Reproduce the exact sequence, timed.** Proving the general case healthy
   says nothing about the specific one. A general responsiveness test said the
   windowed workbench was fine; example 5's actual sequence was not.
4. **Then read the verb.** Not before — twice today the code looked correct and
   the behaviour was not.

## Defects queue behind each other

Today's chain ran four deep: `sim.start` ignored → firmware never up → traffic
never fired → the assertion could not pass → the fixture had no traffic to
fire. Fixing the outer one is what makes the inner one visible, so **"one more
fix and it will work" is a bad prediction.** Budget for the chain.

And the one that passed: example 2 exited 0, printed a cheerful summary, and
had put a local build on a 311-node national network because it decided "first
run" by asking whether the session was empty — and a launched workbench never
is. **A green exit code is not evidence.** Check the numbers say what the script
claims.

## What CI does not cover

- The `race` job runs on tags and on request, not on push. A data race that
  killed the process lived through five green pipelines. Run
  `go test -race ./internal/app/session/` when you touch anything the store
  goroutine and a worker both reach.
- Nothing in CI brings a mesh up and waits for it. Green means it compiles and
  the unit tests pass; it does not mean an example runs.
