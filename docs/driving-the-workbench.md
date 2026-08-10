# Driving the workbench from outside

Findings from the first real agent-driven session, written down because each
one cost a wasted run.

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

1. `boundary.set` + `boundary.accept` — **before** the import. The import
   filters at fetch time, so a boundary set afterwards prunes rather than
   filters, and the fetch has already paid for nodes it will discard.
2. `import.set_source` → `import.fetch` → poll `import.commit` until it
   stops erroring (the commit refuses until a preview exists).
3. `boundary.prune` if the boundary changed after the import.
4. `infer.run` → poll `infer.result` → `infer.apply`.
5. `firmware.set` per version, then `sim.play`/`sim.step`.

## Firmware comes from meshcore-native, not from your own build

`A13xB0/meshcore-native` publishes a native build per MeshCore tag -
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
