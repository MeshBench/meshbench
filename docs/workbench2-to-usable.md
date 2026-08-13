# What workbench2 needs before a human can use it

Written after being told, correctly, that it is "great for an AI" and does not
work for a person. This is the plan to fix that, and the diagnosis it rests on.

**workbench1 is untouched and still works.** No commit in the last forty has
changed `internal/ui`, and it builds clean. Nothing here is urgent in the sense
of restoring something lost.

## The diagnosis

Parity was measured as verbs answered and controls dispatching. Both reached
100%, and neither measures whether somebody can run a mesh.

The failure has one shape. A verb takes parameters, so every panel became a
form: type a role, type a version, type a path, press a button. The thing being
acted on is already on screen - the build in the table, the node on the map,
the endpoint that was just created - and none of it can be clicked.

Everything below follows from that, plus the three bugs it hid.

## Fixed while writing this

- **Right-clicking a node did nothing.** The map's menu dispatcher was a switch
  with no default, so changing an entry's action silently disconnected it. Now
  every entry reaches its verb, and one that does not is impossible to write.
- **Play with real firmware hung for ever.** It set playing immediately and
  started firmware asynchronously, so the store's ticker drove an engine whose
  nodes had no process behind them. The run now begins when the mesh is up, and
  refuses to start at all if any node has no build - naming the nodes.
- **The control socket published eleven scalars.** Counts and flags, not state:
  endpoints, jobs, residuals, the firmware library, node statistics and the
  console were invisible to anything driving the workbench, including every
  test that claimed to verify them.
- **Firmware was filed under Repeaters** although companions, room servers and
  observers all run it. It is under File with the rest of the session.

## 1. Direct manipulation, everywhere

The largest item, and the one that makes the rest worth having.

- **Select a row, act on it.** Firmware: click a build, then use-for-role,
  delete, or pin to the selected nodes. No typing a path that is already in the
  table. The same for Runs, Events, Nodes, Contacts and Channels.
- **Act on the map.** A node's menu should carry everything the Inspector can
  do to it, because that is where somebody's cursor already is.
- **Drag to move, click to place.** The tool strip exists and selects a tool;
  place, move and link do nothing yet.
- **Copy anything worth copying.** An endpoint address exists to be pasted into
  another application's configuration.

## 2. Running a mesh, start to finish

The journey nobody has walked end to end.

- Open a saved network. `project.save` and `project.list` exist as verbs; there
  is no window listing what is saved, and File's "open" points at the live
  import panel instead.
- Choose what the nodes run, from the library, before starting.
- Start, and see it start: which node is on, which failed, why.
- Watch traffic: trails that fade, a legend saying what the colours mean, and
  filters on that legend. The trail code exists and is unverified in the app;
  there is no legend at all.
- Stop, reset, and know the state has gone back.

## 3. Companions as first-class

Named specifically as missing, and the reason the tool exists for application
developers.

- Serve TCP and virtual serial, with the address shown and copyable. Serving
  works and the endpoint was invisible until an hour ago.
- Show whether a client is attached, and what it has sent.
- The radio and channel settings, applied to the node and reflected back from
  it.
- meshcore-cli in the node window, which is done.

## 4. Say what is happening

- A job with a name, a count and a cancel. Jobs exist in the model and no panel
  draws them.
- Errors where the action was, not only in a status line at the bottom.
- Progress for anything slower than a second: firmware starting, links being
  measured, a sweep running, terrain downloading.

## 5. The panels that are still tables

Compare, Matrix, Timelines, Runs, Experiment log, Waterfall, Packet timeline,
Budget, Energy, Link. Each displays and none acts. The old build could select a
run and re-run it, brush a timeline and have the others follow, click a
waterfall feature and select the packet that caused it.

## 6. How this gets verified from now on

The rule that was missing: **a feature is not done until it has been used
through the interface, by hand or by a synthetic click on a real layout, and
seen to do the thing.**

- Screenshot every panel after every change and look at it, rather than
  confirming it rendered.
- Drive the workflows, not the verbs.
- A control that cannot be reached by pointing at what it acts on is not
  finished, however well its verb is tested.
