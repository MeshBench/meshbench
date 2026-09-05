# The control socket, verb by verb

Generated. Run `tools/verbdoc/verbdoc.py` to rewrite it and
`tools/verbdoc/verbdoc.py --check` to fail when it is stale.

The store registers 255 verbs: 218 a script may call and
37 the workbench calls on itself, which the socket refuses. Of those,
255 say what they are for and 0 do not yet; the ones that
do not are marked, and what is printed for them is read out of the handler
rather than said by it.

Each entry is written in the `<basename>.verbs.json` beside the file that
registers the verb, and a description naming a verb the tree no longer
registers fails the build. Every example is a request line for the socket. The ones not marked otherwise are
made against a live session by the test suite, so an example that has stopped
working fails the build rather than the reader.

A call is one line of newline-delimited JSON:

```json
{"id":1,"method":"sim.state","params":{}}
```


## Session and lifecycle

### `app.quit`

Close the workbench, stopping firmware on the way out.

**Takes** nothing.

**Answers** `closing`, `headless`. It answers before anything has closed, the quit running on its own goroutine, so a caller sees the reply and then the socket go. This is the one interface verb that does not refuse without an interface: a headless driver asking to quit means it, so firmware is stopped anyway and `headless` says that is what happened.

**Example** - end the session, firmware and all

```json
{"id":1,"method":"app.quit","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.quit()`

### `log.path`

Find this launch's full status log without knowing the naming scheme, which is everything the session has said rather than the last twenty lines the status strip keeps.

**Takes** nothing.

**Answers** `path`. One file per launch, timestamped, still being written to. Refuses where no log was opened, which is the case for a session started without one rather than a fault.

**Example** - tail this run's log from a shell

```json
{"id":1,"method":"log.path","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.log.path`

Planned, not written: no client defines `wb.log` yet - the session log's path and its export. Call the verb itself in the meantime.

### `logs.export`

Put a copy of the log as it stands somewhere the operator chose, so a run can be attached to a report without the reader needing the machine it ran on.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `to` | string | required, primary | where to write the copy; an empty or missing destination is refused, and an existing file there is overwritten |

**Answers** `path`. The whole file on disk, not the tail the Logs panel holds, so a long run exports everything it said. Refuses where no log was opened, and where the copy cannot be written.

**Example** - keep a run's log beside its results

```json
{"id":1,"method":"logs.export","params":"/tmp/meshbench-run.log"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.log.export(path)`

Planned, not written: no client defines `wb.log` yet - the session log's path and its export. Call the verb itself in the meantime.

### `session.checkpoint`

Freeze the whole session to a named checkpoint.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | what to call this checkpoint |

**Answers** `checkpoint`, `path`, `now_ms`, `nodes`. `path` is the file it wrote, in the checkpoints directory under the user's configuration directory. The filename is the name with everything but letters, digits, underscore and hyphen replaced, so two names differing only in punctuation are one file and the second overwrites the first. Refused where there is no network to freeze, and where the name leaves nothing usable behind.

**Example** - keep this moment to come back to

```json
{"id":1,"method":"session.checkpoint","params":{"name":"before the storm"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.checkpoint(name)`

### `session.checkpoints`

List the checkpoints that can be restored.

**Takes** nothing.

**Answers** `checkpoints`. The filenames, sorted, in the form `session.restore` takes as its name - which is the sanitised form rather than the name it was saved under, where the two differ. A machine that has never saved one answers with an empty list rather than an error.

**Example** - see what can be gone back to

```json
{"id":1,"method":"session.checkpoints","params":{}}
```

**Client** `wb.checkpoints()`

### `session.describe`

The cheapest question a client can ask, and the one worth asking first: whether there is a network in this session at all, and whether its clock is moving.

**Takes** nothing.

**Answers** `nodes`, `seed`, `now_ms`, `playing`. Four facts and no more. `now_ms` is simulated time, not wall time, and stands still while `playing` is false. It never fails, so it is also the handshake for a socket that has just connected.

**Example** - check what is loaded before driving it

```json
{"id":1,"method":"session.describe","params":{}}
```

**Client** `wb.describe()`

### `session.journal`

Every command this workbench has been driven with, newest last, and when the process started - so a session picked up cold can be told how the world got here, and whether it has been restarted.

**Takes** nothing.

**Answers** `started_ms`, `count`, `entries`. Each entry is a sequence number, a wall-clock time, the verb, how many nodes there were when it ran, a short rendering of its argument, and the error where it was refused, because a refusal is part of how a session got here. The polls, the interface-only verbs and the workers' own callbacks are left out, and only the last few hundred commands are kept. `started_ms` is when this process started, which is how a driver tells the session it has been talking to all along from one that has been restarted under it.

**Example** - ask how this session got here

```json
{"id":1,"method":"session.journal","params":{}}
```

**Client** `wb.journal()`

### `session.list`

List the workbenches running on this machine, this one included.

**Takes** nothing.

**Answers** `sessions`, `count`

**Example** - which workbenches are up, and what each is holding

```json
{"id":1,"method":"session.list","params":{}}
```

**Client** `meshbench.sessions()`

### `session.restore`

Rebuild a checkpoint and replay to the moment it was taken.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | optional, primary | the checkpoint to restore, by name |
| `path` | string | optional | a checkpoint file kept outside the checkpoints directory |

**Answers** `restored`, `nodes`, `now_ms`, `target_ms`, `replaying`. `replaying` true means the restore is not finished: the session is rebuilt at zero and set playing towards `target_ms`, which takes the run's own time, so poll `sim.state` until `playing` goes false. A checkpoint taken before the clock moved comes back already there, with `replaying` false. Whatever was loaded before is replaced, seed, frequency and physics included, because a checkpoint restored under other settings is a different study wearing the same name.

**Example** - take the session back to a saved moment

```json
{"id":1,"method":"session.restore","params":{"name":"before the storm"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.restore(name)`

### `session.status`

Report what the session is saying, where the run has got to and which long job is still going, cheaply enough for a script to poll and answered whether or not anything is loaded or drawing.

**Takes** nothing.

**Answers** `status`, `nodes`, `playing`, `now_ms`, `firmware_running`, `jobs`, `job`. `jobs` counts only the jobs still running, and `job` is the newest of those: it is absent when nothing is running, which is what a script waiting for a download or a sweep watches for. It carries the job's id, because matching on the wording of a progress line stopped working the moment the wording improved. `status` is the last thing said, replaced while a play is waiting on firmware to come up.

**Example** - poll until the work in hand has finished

```json
{"id":1,"method":"session.status","params":{}}
```

**Client** `wb.status()`

### `ui.keep_above`

Say whether a window popped out of the workbench stays above it, and report the setting either way.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional | set it; omit to read the current setting without changing it |

**Answers** `on`. The setting after the call, which is on where nothing has ever chosen: the fault it answers is a panel lost behind the main window. It is a stored preference rather than an act on a window, so it is answered with no interface attached and takes effect on the windows opened after it. It matters on Linux under Wayland, where staying above changes what the window is: our own title bar, no taskbar entry, and a close button that returns the panel to the main window. Elsewhere the platform does it anyway.

**Example** - keep a popped-out panel in front of the workbench

```json
{"id":1,"method":"ui.keep_above","params":{"on":true}}
```

**Client** `wb.keep_above()`

### `ui.said`

Put a line where the operator is already looking, so a script's own step is visible in the status strip and in the session log beside the verbs it drove.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `text` | string | optional, primary | the line to show; anything that is not a bare string or a single-keyed object says an empty line rather than refusing |

**Answers** `said`. It never refuses, and it does not need an interface: with nothing drawing, the line still goes to the log the session keeps.

**Example** - mark a step of a script on screen

```json
{"id":1,"method":"ui.said","params":"coverage sweep finished"}
```

**Client** `wb.say(text)`

### `ui.scale`

**Refuses when no window is attached.**

Read or set how large the interface draws itself, the one setting a high-density screen needs and then never again.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `scale` | number | optional, primary | the new scale, one being the interface's own size; absent, zero or negative reads the current scale and changes nothing |

**Answers** `scale`. The scale in force after the call, read back from the interface rather than repeated from the request, so an interface that clamps it says so. Refuses when no interface is attached.

**Example** - make everything a quarter larger on a dense screen

```json
{"id":1,"method":"ui.scale","params":{"scale":1.25}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.scale = x`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `ui.state`

**Refuses when no window is attached.**

Ask what is on screen for a caller with no eyes: the view, the panels in their own windows, the map tool, and what the run is doing beside them.

**Takes** nothing.

**Answers** `view`, `popped`, `scale`, `tool`, `nodes`, `playing`, `now_ms`, `jobs`, `running`. The first four come from whatever is drawing, so another interface may answer with other keys; the rest are the session's own. `jobs` counts the jobs still running and `running` is those same jobs as rows with their ids, because a bare count cannot tell a script what it is waiting for. Refuses when no interface is attached, which is what makes session.status the one to poll.

**Example** - check which view a screenshot will catch

```json
{"id":1,"method":"ui.state","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.state()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

## Project

### `project.list`

Name everything that can be opened: what project.save has written, where it writes them, and the networks that shipped with this copy.

**Takes** nothing.

**Answers** `projects`, `dir`, `fixtures`. `projects` is the user's own saved networks, by name, and `dir` is the directory they are read from - always said, whether or not it exists yet, since a caller building a path needs it either way. `fixtures` is the shipped networks, named as the files name them (`fixture-fife-strict`), found on disk beside an install and inside the binary; they are listed rather than copied into the projects directory, so a later release can correct one without overwriting somebody's edit. On a machine that has never run MeshBench `projects` is empty and `fixtures` is not, which is the first-run case and not a fault. A name from `fixtures` is opened by passing it to project.open as it stands; a saved project is opened by joining `dir` and `<name>.json`.

**Example** - see what can be opened

```json
{"id":1,"method":"project.list","params":{}}
```

**Client** `wb.project.list()`

### `project.new`

Throw away what is loaded and start on an empty network, through the same path an open takes, optionally putting the map and the study area on a named place so the first node has somewhere to go.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `place` | string | optional, primary | somewhere to start, looked up the way a study area is, so "Fife" means the same thing here as in the Import panel; empty leaves the map where it was, and a name nothing can be found for is said in the status line rather than refused |

**Answers** `nodes`, `place`. `nodes` is always zero, and `place` appears only where one was named. A run that was going is stopped first, and the firmware behind it with it, since both belonged to the network being discarded. The lookup runs on a worker, so the map and the study area move after this has already answered.

**Example** - an empty network to place nodes on by hand

```json
{"id":1,"method":"project.new","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.project.new(place=None)`

### `project.open`

**Refuses when no window is attached.**

Load a fixture and put the camera on what it holds, on the open rather than at the first play, so nobody is left reading a blank map with a node count beside it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | required, primary | the fixture's path; one that will not load is refused and the session keeps whatever it already had |

**Answers** `opened`, `nodes`, `links`. `links` is zero on every open, because the matrix is cleared here and re-measured as the job `links`: a path loss over real terrain is minutes of work, so the map draws proximity links until it finishes. A run that was going is stopped first and the status line says so: the clock steps whichever engine is live, and this replaces it and stops the firmware processes behind it, so a script that opens a network mid-run plays again itself once it is ready. Nothing else moves the camera on an open, so a script that wants to be somewhere else says so afterwards with map.centre.

**Example** - open a network that ships with the build

```json
{"id":1,"method":"project.open","params":"fixtures/fixture-fife-strict.json"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.project.open(path)`

### `project.save`

Write the nodes as they stand, and the margin they are judged against, into a named file under the user's config directory, so the setup can be opened again.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | what to call it: a name and not a path, since it is joined onto the projects directory and written to, so a separator or a leading dot is refused rather than followed |

**Answers** `saved`, `path`, `nodes`. It writes the scenario's nodes only. A run, its capture and the terrain cache are not in the file, so opening it again gives back the network rather than the session.

**Example** - keep this network to come back to

```json
{"id":1,"method":"project.save","params":{"name":"fife-survey"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.project.save(name)`

## Nodes

### `node.aim`

Turn a node's antenna towards another node, and say what that won it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node whose antenna is turned |
| `at` | string | required | the node to point it at |

**Answers** `node`, `at`, `bearing_deg`, `distance_km`, `gain_dbi`. `bearing_deg` is the true bearing between the two positions the scenario already holds, and `gain_dbi` is this node's pattern read along it - which is the part worth reading, because on an omni it is the figure it was before and a control that reports success while changing nothing is one somebody trusts once. Only the named node turns: what the far end hears back still depends on where its own antenna points. Refused where either node is unknown, where a node is aimed at itself, and where both stand at the same position, which has no bearing between them.

**Example** - point the hilltop repeater at the node it talks to

```json
{"id":1,"method":"node.aim","params":{"at":"Dunfermline","node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.aim(at)`

### `node.antenna`

Report what one node's antenna is and which way it points.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to ask about |

**Answers** `node`, `pattern`, `gain_dbi_peak`, `beamwidth_deg`, `front_to_back_db`, `bearing_deg`, `downtilt_deg`, `polarisation`, `feedline_db`, `peak_dbi`. The same words the verb that sets an antenna takes, so what comes back can be handed straight back in. A node carrying no antenna answers with an empty `pattern` and `peak_dbi` zero rather than as an omni at 0 dBi, which in a table of numbers those two would otherwise share. Both gain figures are the peak along boresight; what a given link is actually worth is the pattern read along the bearing to the far end, and differs in each direction.

**Example** - ask what a node stands under and where it faces

```json
{"id":1,"method":"node.antenna","params":{"node":"West Lomond"}}
```

**Client** `node.antenna`

### `node.boardview`

**Refuses when no window is attached.**

Open a node's board view: what its profile declares, what the firmware left in the chip, where the two differ, and the controls for everything the board has wired.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node |

**Answers** `node`, `board`. `board` is the profile the window is checking against. Refused for a node running a host build, which has no board to check, and refused outright in a headless session, there being no window to open one beside.

**Example** - look at one board in full, and drive what it has

```json
{"id":1,"method":"node.boardview","params":"Deck"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.board_view(node)`

### `node.card`

Report or change what is in one node's card slot: whether a card is fitted, which file it is, and whether to erase it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node |
| `fitted` | bool | optional | put a card in the slot or take it out; unchanged when absent |
| `file` | string | optional | the file behind the card; empty string returns it to the node's own, named after the node and kept beside its flash |
| `wipe` | bool | optional | erase the card, which is what reformatting one is |

**Answers** `node`, `slot`, `fitted`, `file`, `own_file`, `bytes`, `required_by_firmware`, `board_has_slot`, `wiped`. Asking with nothing but a node changes nothing and reports the slot as it stands. `slot` is what the scenario says - fitted, empty or unstated - while `fitted` is whether the node actually has storage, which a firmware that keeps its settings on the card makes true whatever the slot says. `bytes` is nought where the file has not been made yet, which is the normal state before a first run.

**Example** - ask what is in a node's card slot

```json
{"id":1,"method":"node.card","params":"West Lomond"}
```

**Client** `node.card(fitted=|file=|wipe=)`

### `node.energy`

Run a year of sun and battery at one named node, at the duty cycle the run measured for it rather than one typed into a form, for the node window that has a name and no selection to work from.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to study; a name this network has not got, or none at all, is refused - and note that the call selects whatever it names and deselects everything else |

**Answers** `node`. Refused outright unless MESHBENCH_ENERGY is set: the solar model is not trusted yet, and a plausible worst-day figure from an untrusted model is worse than none. The year itself - worst state of charge, the day it falls on, the dead days - goes into the snapshot and onto the status line rather than into this answer. With no run behind it the duty cycle is zero, which sizes the site against a node that never transmits.

**Example** - ask whether one repeater's pack survives December

```json
{"id":1,"method":"node.energy","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.energy()`

### `node.output`

Read what a node's serial port, emulator or radio model has printed.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to read |
| `source` | string | optional | which voice: serial, rom, emulator, radio; serial when absent, and anything else is refused |
| `lines` | number | optional | how many lines of the tail to answer with; 200 when absent or not a positive number, and never more than the 2000 the pane itself holds |

**Answers** `node`, `source`, `lines`, `total`, `path`, `tail`, `note`, `tracing`. `total` is how many lines the file has and `lines` how many the pane was given, so the two differing is a tail rather than a board that stopped talking. `tail` is the shorter answer this call gets, and is empty with a `note` where the source is one this node's backend does not have - a native node has no emulator and no radio log - or where it has not run since the workbench started.

**Example** - read the last lines a board printed

```json
{"id":1,"method":"node.output","params":{"lines":50,"node":"West Lomond","source":"serial"}}
```

**Client** `node.output(source)`

### `node.output_window`

**Refuses when no window is attached.**

Open one node's one log in a window of its own, so a board's screen and several of its logs can be watched at once.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node |
| `source` | string | optional | which log: serial, rom, emulator, radio; serial when absent |

**Answers** `node`, `source`. The answer says the window was opened, not what is in it: the pane fills from the tick that follows. Refused outright in a headless session, there being no window to open one beside.

**Example** - watch the emulator's log beside the board's screen

```json
{"id":1,"method":"node.output_window","params":{"node":"West Lomond","source":"emulator"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.output_window(source)`

### `node.provisioning`

Read the console lines a node is told before a run, so a region defined but never allowed to flood is visible rather than looking like broken radio.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | whose script to read; a name this network has not got is refused |

**Answers** `node`, `commands`. `commands` is the lines that are actually sent, with the commentary dropped; the panel keeps the annotated form, each line with the reason it exists.

**Example** - see what a node will be told at start

```json
{"id":1,"method":"node.provisioning","params":"West Lomond"}
```

**Client** `node.provisioning`

### `node.radio`

Ask a running node what frequency, modulation and transmit power it is actually set to, and say where that disagrees with the figures every path loss for it was computed from.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | the node to ask; absent falls back to the selected node, and with nothing selected either the call is refused |

**Answers** `node`, `assumed`, `reported`, `differences`, `note`. `assumed` and `reported` are both objects of freq_mhz, bandwidth_hz, spreading_factor, coding_rate and tx_dbm. A node answers over a frame or its console and the reply only lands when the engine next steps, so the first call usually returns `reported` null with a `note` saying to ask again. An empty `differences` means the two agree, not that nothing was checked; coding rate is deliberately never compared, because model and firmware write the same setting in different conventions.

**Example** - check the model is pricing this node's real transmit power

```json
{"id":1,"method":"node.radio","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.radio`

### `node.radio_adopt`

Take the radio configuration a node has already reported as the one the model uses for it, so the path losses are computed from what the node transmits with rather than from what the scenario intended.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | the node to believe; absent falls back to the selected node, and a node that is not a connected companion, or has not reported yet, is refused with what to do about it |

**Answers** `node`, `tx_dbm`. `tx_dbm` is the node's own transmit power, now the model's. Frequency, bandwidth and spreading factor are adopted with it, and because path loss is cached per pair the engine is rebuilt and every link measured again on a worker.

**Example** - stop modelling a power this node does not have

```json
{"id":1,"method":"node.radio_adopt","params":"Dunfermline"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.adopt_radio()`

### `node.reflash_failed`

**The workbench's own callback. The socket refuses it.**

Report that a build change did not go through, which reaches the operator after the caller that asked has already been told it was accepted.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `reason` | string | optional, primary | what went wrong, said to the operator as it stands |

**Answers** Answers with nothing: what it changes is the stats and the status line.

**Client** none: the store telling itself a reflash failed

### `node.reflashed`

**The workbench's own callback. The socket refuses it.**

Report that a node's build change went through, refreshing the counters and the node's own window, which reads the node list rather than the stats and so showed the old build for ever.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `message` | string | optional, primary | what to say, whose first word is the node that changed |

**Answers** Answers with nothing: what it changes is the stats, the node list and the status line.

**Client** none: the store telling itself a reflash finished

### `node.set_board`

Say what hardware a node is, which is a change to the physics and not a label: the transmit ceiling, the receive chain's noise figure and the battery the energy model runs on all come off the board.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node changes hardware; absent or unknown is refused |
| `board` | string | optional | the board it is, as the firmware library names one; a board this build has no profile for is refused, and an empty or absent name returns the node to no particular hardware, which is a build for this machine |

**Answers** `node`, `board`. A build pinned for the old board is cleared rather than carried across, because that image is for that hardware and a node keeping the pin would look configured and refuse at start.

**Example** - make a node a Heltec WSL3

```json
{"id":1,"method":"node.set_board","params":{"board":"Heltec_WSL3","node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.board = ...`

### `node.set_firmware`

Change the build a node runs and apply it now, stopping the node, provisioning it and starting it again, because firmware is chosen when a node launches and nothing else would take effect.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which node changes build; absent, blank or unknown is refused |
| `version` | string | required | the build to run, as the firmware library names it; absent or blank is refused |
| `board` | string | optional | the hardware the image is for, because a board image is that image for that board; absent means a build for this machine |
| `role` | string | optional | which MeshCore role the image is; absent leaves the node's role as it was |

**Answers** `node`, `version`, `board`, `role`. The answer says the change was accepted, not that it took: the stop, provision and start run behind it, and a build that will not provision or a node that will not start arrives afterwards as node.reflash_failed.

**Example** - put a host build on one node and restart it into it

```json
{"id":1,"method":"node.set_firmware","params":{"node":"West Lomond","version":"v1.7.1"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.firmware = build`

### `node.set_firmware_only`

Record the build a node will run at its next start without touching the node now, for setting a fleet up before anything is launched.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which node is set; absent, blank or unknown is refused |
| `version` | string | required | the build it will run, as the firmware library names it; absent or blank is refused |
| `board` | string | optional | the hardware the image is for; absent means a build for this machine, and clears any board the node was pinned to |
| `role` | string | optional | which MeshCore role the image is; absent leaves the node's role as it was |

**Answers** `node`, `version`, `board`, `role`. Nothing restarts. A node already running goes on running what it has until something stops it, which is the difference between this and node.set_firmware.

**Example** - choose what a node will run without disturbing it

```json
{"id":1,"method":"node.set_firmware_only","params":{"node":"West Lomond","version":"v1.7.1"}}
```

**Client** `node.set_firmware(build, apply=False)`

### `node.start`

Bring a stopped node's firmware back up, which goes through the whole-mesh attach and so starts every other stopped node with it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node starts; a name this network has not got is refused, and so is a node that is already running |

**Answers** `started`. `started` is the node that was asked for, not everything that came up: the attach behind it skips only the nodes already running.

**Example** - put a stopped node back on the air

```json
{"id":1,"method":"node.start","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.start()`

### `node.stop`

Take one node's firmware down while leaving the node in the scenario, so it reports its final counters on the way out - which are usually the only evidence about a node that was misbehaving.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node stops; a name this network has not got is refused, and so is a node that is not running firmware |

**Answers** `stopped`

**Example** - take one node off the air

```json
{"id":1,"method":"node.stop","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.stop()`

### `node.truerf`

Give one receiver waveform verdicts inside a calculated run, so the pair being studied is decided by the full receive chain while the rest of the network stays on the fast model.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the receiver that takes waveform verdicts; a name no node has is refused rather than ignored |
| `on` | bool | optional | true to hold this node on waveform whatever the run mode; absent means false, which puts it back on the run's own mode |

**Answers** `node`, `true_rf`

**Example** - decide this one receiver honestly, at one node's cost

```json
{"id":1,"method":"node.truerf","params":{"node":"West Lomond","on":true}}
```

**Client** `node.true_rf = bool`

### `node.window`

**Refuses when no window is attached.**

Open one node's own window, the thing people put on a second monitor.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which node |
| `tab` | string | optional | which tab to open on; the window's default when absent |

**Answers** `node`, `tab`. `tab` is the tab the window actually opened on, which is not always the one asked for. Refused outright in a headless session, there being no window to open one beside.

**Example** - put one node on a second monitor

```json
{"id":1,"method":"node.window","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.window(node, tab=None)`

### `node.wipe`

Erase one node's stored state: its flash, its card and its files.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to put back to factory |
| `confirm` | bool | optional | false lists what would go and removes nothing; absent or true erases it |

**Answers** `node`, `wiped`, `removed`, `would_remove`. `wiped` counts what went and `removed` names it, the node's card included where it keeps one elsewhere; the emulator's own sockets are left, being recreated at the next start. With `confirm` false nothing is touched and `would_remove` names what a wipe would take. A node with nothing on disk answers zero rather than refusing. Refused where the node is not an emulated board, where it is still running, and where only part of it could be removed - a partial wipe is an error naming what stayed, because a board that boots back into settings said to be gone is worse than one that was never wiped.

**Example** - see what putting a board back to factory would take

```json
{"id":1,"method":"node.wipe","params":{"confirm":false,"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.wipe()`

### `nodes.add_to_selection`

Add nodes to whatever is already selected, which is the shift-drag, and the way a selection is built up out of several passes over the map.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `names` | array | optional, primary | the nodes to add, as a list, one name, or {"names": [...]}; a name this network has not got refuses the whole call, and no names at all adds nothing and leaves the selection as it was |

**Answers** `added`. `added` counts the nodes matched, not the nodes newly selected: one that was already in the selection is counted again.

**Example** - add one more node to the selection

```json
{"id":1,"method":"nodes.add_to_selection","params":{"names":["Dunfermline"]}}
```

**Client** `wb.nodes.select(*names, add=True)`

### `nodes.allow_flood`

Let a node forward a flood whatever region it was scoped to, which is the difference between a scenario that relays and one that transmits everything, relays nothing and reports no error.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional, primary | true to forward any flood, false to forward only the scoped ones; anything that is not a boolean leaves it true |
| `node` | string | optional | the one node to set, matched on the whole name; absent, every node is set, and a name that matches nothing sets nothing and is not refused |

**Answers** `nodes`, `allow_any_flood`. `nodes` is how many were written to, which is 0 when the name matched nothing.

**Example** - let every node forward a flood whatever its scope

```json
{"id":1,"method":"nodes.allow_flood","params":{"on":true}}
```

**Client** `node.allow_flood = bool`

### `nodes.antenna`

Choose and aim the antenna on one node, on a kind, or on every node.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional | one node; absent means every node the other filters leave |
| `kind` | string | optional | only nodes of this scenario kind |
| `pattern` | string | optional | isotropic, dipole, collinear or yagi |
| `gain_dbi_peak` | number | optional | the headline gain, for a collinear or a yagi |
| `beamwidth_deg` | number | optional | a yagi's horizontal half-power beamwidth |
| `front_to_back_db` | number | optional | how far down a yagi's back is on its front |
| `bearing_deg` | number | optional | compass bearing of boresight, 0 at north |
| `downtilt_deg` | number | optional | degrees the beam is tilted below the horizon |
| `polarisation` | string | optional | vertical, horizontal or circular |
| `feedline_db` | number | optional | cable and connector loss, as a positive number |

**Answers** `nodes`, `pattern`, `gain_dbi_peak`, `beamwidth_deg`, `front_to_back_db`, `bearing_deg`, `downtilt_deg`, `polarisation`, `feedline_db`. What the last matched node now carries, with `nodes` for how many were changed and no node name, because the answer is about a selection rather than one node. Each field named replaces one part of the antenna already there, so a collinear switched to a yagi keeps the gain figure somebody chose. Setting one rebuilds the engine over the changed nodes and re-measures every link: the cached look angles belong to the antenna that used to be there. Refused where a named node is unknown, where a pattern or a polarisation is not one the model prices, and where the filters leave no node at all.

**Example** - stand a yagi on the hill, facing down the Forth

```json
{"id":1,"method":"nodes.antenna","params":{"beamwidth_deg":45,"bearing_deg":208,"front_to_back_db":20,"gain_dbi_peak":12,"node":"West Lomond","pattern":"yagi"}}
```

**Client** `node.set_antenna(...) / wb.nodes.set_antenna(...)`

### `nodes.delete`

Take one node out of the scenario for good, which is how an imported deployment is cut down to what a desktop will actually run.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to remove, matched on the whole name and exactly; an empty one is refused, and so is a name no node has, rather than deleting nothing quietly |

**Answers** `deleted`, `nodes`. `nodes` is how many are left. It rebuilds the engine and empties the link matrix, so every remaining pair is measured again as a job before the run means anything.

**Example** - drop a node from the scenario

```json
{"id":1,"method":"nodes.delete","params":{"node":"Dunfermline"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.nodes.delete(*names) / node.delete()`

### `nodes.delete_many`

Remove a set of nodes in one rebuild, rather than one call each rebuilding the scenario and cancelling the warm the last one started.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `nodes` | array | optional, primary | the names to remove, as a list, one name, or {"nodes": [...]}; one this network has not got refuses the whole call and removes nothing, naming none is accepted and does nothing, and a shape outside that set is refused rather than read as no names |

**Answers** `deleted`, `nodes`. `deleted` is the names that went and `nodes` is how many are left. Every link is dropped and the matrix re-measured behind the answer, because the network is not the one that was measured.

**Example** - take one node out of the network

```json
{"id":1,"method":"nodes.delete_many","params":{"nodes":["Dunfermline"]}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.nodes.delete()`

### `nodes.keep`

Cut a network down to the nodes named and remove everything else, which is how trimming a fixture is actually said.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `nodes` | array | optional, primary | the names to keep, as a list, one name, or {"nodes": [...]}; one this network has not got refuses the whole call and removes nothing, and naming none keeps nothing, which empties the network. A shape outside that set is refused rather than read as no names, because here that reading empties the network and answers success |

**Answers** `deleted`, `nodes`. `deleted` names what was removed rather than what was kept, and `nodes` is how many are left.

**Example** - cut a network down to one node

```json
{"id":1,"method":"nodes.keep","params":{"nodes":["West Lomond"]}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.nodes.keep()`

### `nodes.list`

Read back the whole network as it stands, which is what anything automated does first and the only way to see what a scenario built by a script actually got.

**Takes** nothing.

**Answers** `nodes`, `count`. A row per node under `nodes` and `count` beside them, so a caller need not measure the list to know how big the network is. Each row carries two boards, which are two facts: `board` is what the node is and `firmware_board` what its image was built for, and they come apart the moment a host build is pointed at a T-Deck. There is no limit and no paging, so an imported deployment answers with all of it. What a node has *done* is not here: a row carries no packet counts, because this describes what the network is and the counters change every tick. nodes.stats has them, measured from the engine's own scoreboard.

**Example** - see what is in the scenario

```json
{"id":1,"method":"nodes.list","params":{}}
```

**Client** `wb.nodes  (iterate)`

### `nodes.move`

Put a node at a position and move its physics with its marker, forgetting the losses cached for it so the next window an attached SDR client hears is the one from where it now stands.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which node moves; absent, blank or unknown is refused. Spelt `node`, as every other verb that acts on a node it did not create spells it; `name` is still read, because it is what this one verb asked for and it is in saved scripts |
| `lat` | number | required | degrees north, minus 90 to 90; absent or outside that is refused rather than read as nought, which used to put the node in the Gulf of Guinea and report it as asked for |
| `lon` | number | required | degrees east, minus 180 to 180; absent or outside that is refused |

**Answers** `node`, `name`, `lat`, `lon`. `node` and `name` both carry the node that moved, so a caller reading either spelling back gets an answer.

**Example** - move a node onto the hill it is named after

```json
{"id":1,"method":"nodes.move","params":{"lat":56.25,"lon":-3.29,"node":"West Lomond"}}
```

**Client** `node.move(lat, lon)`

### `nodes.near`

Order the rest of the network by how far it is from one node, which is how an imported deployment is cut down to a neighbourhood a desktop will actually run.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to measure from, matched as nodes elsewhere are matched; a name no node has is refused, and so is an absent one |
| `count` | number | optional | how many of the nearest to return; anything not a positive number returns every other node in the scenario |

**Answers** `node`, `near`. `node` is the name as the scenario spells it. `near` is a row per other node with its great-circle distance in `km`, nearest first, ties broken on the name. The distance is the same geometry the path losses are computed on, so a script's idea of nearest and the simulator's are one idea; it is still only a distance, and says nothing about whether either end can hear the other.

**Example** - find the neighbourhood around a repeater

```json
{"id":1,"method":"nodes.near","params":{"count":5,"node":"West Lomond"}}
```

**Client** `wb.nodes.near()`

### `nodes.place`

Put one node down at a position, which is how a repeater a feed never carried gets into a scenario, and how a mesh is built by hand rather than imported.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | what the node is called, which has to be new: a name the network already holds is refused rather than merged |
| `lat` | number | required | latitude in degrees; the call is refused without it |
| `lon` | number | required | longitude in degrees; the call is refused without it |
| `kind` | string | optional | what the node is - simple-repeater, advanced-repeater, companion, room-server, sdr-observer or emitter; absent places a simple-repeater |
| `board` | string | optional | the hardware it is, which decides the transmit ceiling, the receive chain's noise figure, the battery and the antenna it stands under; a name no board profile has is refused rather than defaulted to a plausible one, and absent leaves it unstated |
| `height_m` | number | optional | antenna height above ground in metres; absent is 10 |
| `tx_dbm` | number | optional | transmit power in decibel-milliwatts; absent is 22 |

**Answers** `placed`, `kind`, `regions`, `board`, `nodes`. `regions` and the firmware build are inherited from the neighbours rather than left empty: a node holding a region its neighbours do not is as silent as one holding none. `nodes` is the whole network's count afterwards. The links are not in the answer, because measuring them is a worker's job and this returns before it.

**Example** - put a repeater on the hill and see what it wins

```json
{"id":1,"method":"nodes.place","params":{"board":"Heltec_v3","height_m":6,"kind":"simple-repeater","lat":56.25,"lon":-3.29,"name":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.nodes.place(name, kind, lat, lon, ...)`

### `nodes.regions`

Say which regions a node holds, which is how a node placed by hand is given what its neighbours already hold: inference reads the real network's traffic and so reaches only the nodes that were seen on it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | the one node to set, matched on the whole name; absent, every node in the scenario is set, and a name that matches nothing sets nothing and is not refused |
| `regions` | array | optional | the regions the node is to hold, as strings; absent or not a list of strings leaves the node holding none, which is a clear rather than a call that did nothing |

**Answers** `nodes`, `regions`. `nodes` is how many were written to, which is 0 when the name matched nothing. What is given replaces what a node held rather than adding to it.

**Example** - give a hand-placed repeater the region its neighbours use

```json
{"id":1,"method":"nodes.regions","params":{"node":"West Lomond","regions":["ioi"]}}
```

**Client** `node.regions = [...]`

### `nodes.search`

Find a node by roughly what it is called and rank the matches, so a name full of emoji, box-drawing characters or Gaelic accents can be reached by something a person can type.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `query` | string | required, primary | roughly the name, with accents and emoji ignored on both sides; one with no letters or digits in it is refused rather than answered with the whole network |
| `limit` | number | optional | how many of the best matches to return; anything not a positive number leaves it at ten |

**Answers** `query`, `matches`, `total`. `total` is how many names scored well enough to be offered and `matches` only the best of them, so the two differ whenever the limit bit. Each match carries a score from 0.2 to 1, where 1 is the same name; a shorter name beats a longer one that matched the same way, and ties break on the name so the same query answers the same way twice. No match at all is an empty list, not an error.

**Example** - find a node whose real name cannot be typed

```json
{"id":1,"method":"nodes.search","params":{"query":"lomond"}}
```

**Client** `wb.nodes.search() / wb.nodes.find()`

### `nodes.select`

Make one node the selection, which is what the verbs that act on "the selected node" - sim.inject among them - go on to find.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | which node; a name this network has not got is not refused here, and clears the selection, as an empty name does |

**Answers** `selected`. `selected` is the name that was asked for, whether or not a node answers to it: this is the click a map sends, and it sets every node's selected flag from that one name.

**Example** - select one node

```json
{"id":1,"method":"nodes.select","params":"West Lomond"}
```

**Client** `wb.nodes.select(name)`

### `nodes.select_many`

Replace the selection with a set of nodes, which is what a box drag on the map amounts to and what a script does before any verb that acts on a selection.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `names` | array | optional, primary | the nodes to select, as a list, one name, or {"names": [...]}; a name this network has not got refuses the whole call, and no names at all clears the selection |

**Answers** `selected`. Every other node is deselected, so this is a replacement rather than an addition; nodes.add_to_selection is the addition.

**Example** - select two nodes at once

```json
{"id":1,"method":"nodes.select_many","params":{"names":["West Lomond","Dunfermline"]}}
```

**Client** `wb.nodes.select(*names)`

### `nodes.stats`

Recompute every node's counters now rather than at the next tick, and answer with the rows themselves, which is how anything outside the window asks whether a node is running.

**Takes** nothing.

**Answers** `nodes`, `stats`. `nodes` is how many rows there are and `stats` is the rows. A session with no engine built has no counters to report and answers with none, which is not a failure.

**Example** - read every node's state and counters

```json
{"id":1,"method":"nodes.stats","params":{}}
```

**Client** `wb.nodes.refresh_stats()`

## Boards

### `board.key`

Type at the board's own keyboard, a character at a time, which is what the hardware sends: it holds the last key pressed and the firmware polls it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which board; refused when absent, when the node is not running, or when its backend has no keyboard |
| `text` | string | required | the characters to send, each one a keypress; refused when absent or empty |

**Answers** `node`, `typed`. `typed` is how many characters were sent, not how many the firmware read: it polls, so typing faster than it polls loses keys.

**Example** - type a word at the board's keyboard

```json
{"id":1,"method":"board.key","params":{"node":"West Lomond","text":"hello"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.device.type(text)`

### `board.matrix`

Publish what every board in the catalogue was last measured to demonstrate, for one firmware release, without booting anything.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `version` | string | optional, primary | the MeshCore release the rows are read for; absent or empty takes the release the matrix defaults to, which tracks the latest rather than whatever a result was first measured on |

**Answers** `version`, `boards`. `boards` is a count: the rows themselves go into the snapshot as the board matrix, where a panel or `ui.state` reads them. Nothing is measured here, so a board with no cached result for that release reads as untested, and one that cannot be emulated at all reads as boot-failed with the reason.

**Example** - read the matrix as it stands

```json
{"id":1,"method":"board.matrix","params":{}}
```

**Client** `wb.boards.matrix(version)`

Planned, not written: no client defines `wb.boards` yet - the board-probe verbs; node.device covers a board that is already placed. Call the verb itself in the meantime.

### `board.press`

Hold one of a board's own buttons down, or let it go, so a long press reaches the firmware as a long press rather than as a tap.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which board; refused when absent, when the node is not running, or when its backend has no buttons |
| `pin` | number | required | the GPIO the button sits on, as the board profile declares it; refused when absent or not a number |
| `down` | bool | optional | true holds it, false releases it; absent counts as a release |

**Answers** `node`, `pin`, `down`. The answer repeats what was asked, which is the acknowledgement that the press reached the board at all. Whether the firmware did anything with it is board.screen's question.

**Example** - hold the PRG button down

```json
{"id":1,"method":"board.press","params":{"down":true,"node":"West Lomond","pin":0}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.device.press(pin, down)  /  .tap(pin)`

### `board.probe`

Boot one board in the emulator for real and measure what it demonstrates, overwriting that board's cached row.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `board` | string | required, primary | the board to boot, by its catalogue name; absent is refused |
| `version` | string | optional | the MeshCore release to probe, which has to be named to be meant; absent takes the release the matrix defaults to |

**Answers** `probing`, `board`, `version`. It answers as soon as the job is started, not when the board has been measured: the boot runs on its own goroutine for as long as the probe budget allows, and the result arrives through `board.probe_finished`, which republishes the matrix. Poll `job.list` for `boardprobe`. A second call while one is running is refused, because a board is probed one at a time.

**Example** - measure one board's capabilities

```json
{"id":1,"method":"board.probe","params":{"board":"Heltec_WSL3"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.boards.probe(board, version)`

Planned, not written: no client defines `wb.boards` yet - the board-probe verbs; node.device covers a board that is already placed. Call the verb itself in the meantime.

### `board.probe_finished`

**The workbench's own callback. The socket refuses it.**

Take a finished probe back onto the store's goroutine: clear the job, republish the matrix over the row the probe has just written, and say how the board did.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `board` | string | required, primary | the board that was probed |
| `version` | string | optional | the release it was probed at; an empty one republishes the matrix for an empty version, which holds no rows |

**Answers** `board`, `passed`, `failed`. `passed` and `failed` are counted off the cached row the probe saved, so capabilities it never reached are in neither total.

**Client** none: a probe worker reporting back

### `board.screen`

Measure what a board's own display is showing as numbers rather than as a picture, so a script can tell whether a press or a keystroke changed anything.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | which board; refused when absent, when the node is not running, or when its backend has no display |

**Answers** `node`, `has_screen`, `width`, `height`, `bpp`, `on`, `lit`, `digest`. A board whose backend models a display but has drawn nothing yet answers with `has_screen` false and nothing else, which is a fact about the board rather than a failure. Otherwise `lit` counts the non-zero bytes of the framebuffer, for how much is on, and `digest` is a hex hash of the whole frame, for whether it is the same frame - which is what a wait for the screen to change compares.

**Example** - check whether the display has changed

```json
{"id":1,"method":"board.screen","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.device.screen()  (numbers, not a picture)`

### `board.screenshot`

Write the board's display to a PNG and return its path.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node whose screen to capture |

**Answers** `node`, `path`, `width`, `height`, `bpp`, `on`. The picture is the frame the firmware drew, at the size the controller holds it, written to screen.png in that node's own work directory and overwritten each time. `on` says whether the panel was lit, which is a separate question from whether there is a frame: a display put to sleep still holds its last one. Refused where the node is not running, is not a board with a display, or has drawn nothing yet.

**Example** - see what the board is showing

```json
{"id":1,"method":"board.screenshot","params":{"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.device.screenshot()  (writes a PNG)`

### `board.touch`

Put a finger on the board's panel at a point, or take it off, in the panel's own pixels rather than the drawn screen's.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | which board; refused when absent, when the node is not running, or when its backend has no touch panel |
| `x` | number | required | the column, from the panel's own left edge; refused when absent or not a number |
| `y` | number | required | the row, from the panel's own top edge; refused when absent or not a number |
| `down` | bool | optional | true touches, false lifts off; absent counts as a lift off |

**Answers** `node`, `x`, `y`, `down`

**Example** - touch the middle of the panel

```json
{"id":1,"method":"board.touch","params":{"down":true,"node":"West Lomond","x":120,"y":80}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.device.touch(x, y, down) / .tap_at(x, y)`

## Simulation

### `sim.faster`

Double how much simulated time one tick covers, trading detail for how long a run takes to watch.

**Takes** nothing.

**Answers** `step_ms`. `step_ms` is the new tick length. Nothing bounds it: enough calls will step past whole transmissions.

**Example** - run twice as coarsely

```json
{"id":1,"method":"sim.faster","params":{}}
```

**Client** `wb.sim.faster()`

### `sim.inject`

Put one packet on the air from a node, to exercise the radio model and the traffic layer without firmware behind it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | which node transmits; absent means the selected one, and a name this network has not got is refused rather than falling through to the first node |

**Answers** `at`. Nothing relays what this originates: relaying is a firmware behaviour, and this packet has no firmware behind it.

**Example** - transmit from one named node

```json
{"id":1,"method":"sim.inject","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.inject(payload=None)`

### `sim.kind`

Choose whether a run carries real MeshCore firmware or only the channel, and report which it is now.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `real` | bool | optional, primary | true to start MeshCore on every node at play; absent leaves the setting alone and only reads it |

**Answers** `real`, `running`. `running` is how many nodes have firmware up right now, which is not changed by setting `real`: it takes effect at the next play.

**Example** - ask what kind of run this is

```json
{"id":1,"method":"sim.kind","params":{}}
```

**Client** `wb.sim.real_firmware = bool`

### `sim.pause`

Stop the clock where it is, leaving firmware running and the engine built.

**Takes** nothing.

**Answers** `playing`

**Example** - hold the run still

```json
{"id":1,"method":"sim.pause","params":{}}
```

**Client** `wb.sim.pause()`

### `sim.play`

Let the clock run, without touching firmware or waiting for the links to be measured.

**Takes** nothing.

**Answers** `playing`. `playing` is always true: this verb sets the clock going rather than reporting whether it could.

**Example** - start the clock

```json
{"id":1,"method":"sim.play","params":{}}
```

**Client** `wb.sim.play()`

### `sim.reset`

Put the clock back to zero and rebuild the engine on the same seed and the same nodes, which is how the arm of a comparison starts.

**Takes** nothing.

**Answers** `seed`, `now_ms`. The scenario survives and the run does not: the send schedule is cleared with the clock, and every link is measured again.

**Example** - start this scenario over

```json
{"id":1,"method":"sim.reset","params":{}}
```

**Client** `wb.sim.reset()`

### `sim.run`

Play until a stated simulated time and stop there, which is how a script gets a run of a known length instead of watching for one.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `for_ms` | number | optional, primary | simulated milliseconds to run for; anything not a positive number leaves it at ten seconds |

**Answers** `running`, `until_ms`, `now_ms`. It returns as soon as the limit is set, not when the run reaches it. Poll `sim.state` until `playing` goes false.

**Example** - run for a simulated minute

```json
{"id":1,"method":"sim.run","params":{"for_ms":60000}}
```

**Client** `wb.sim.run(ms=|seconds=|minutes=)`

### `sim.seed`

Read the seed the run draws its noise and its timing jitter from, or set a new one and rebuild on it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `seed` | number | optional, primary | the new seed; anything not a positive number reads the current one without changing it |

**Answers** `seed`. Setting it rebuilds the engine and re-measures every link, so the run starts again rather than continuing on a new draw.

**Example** - read the seed this run is on

```json
{"id":1,"method":"sim.seed","params":{}}
```

**Client** `wb.sim.seed = n`

### `sim.settle`

Step the engine on a stopped clock so queued serial input reaches the firmware, which is what makes provisioning take effect.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `steps` | number | optional, primary | how many ticks to step; anything not a positive number leaves it at sixty |

**Answers** `now_ms`, `steps`. Refuses with no engine built. It steps synchronously, so it answers only once the steps have been taken.

**Example** - let a just-provisioned mesh read what it was sent

```json
{"id":1,"method":"sim.settle","params":{"steps":60}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.sim.settle(steps=...)`

### `sim.slower`

Halve how much simulated time one tick covers.

**Takes** nothing.

**Answers** `step_ms`. `step_ms` is the new tick length, which reaches zero after enough calls and stops the clock advancing at all.

**Example** - run twice as finely

```json
{"id":1,"method":"sim.slower","params":{}}
```

**Client** `wb.sim.slower()`

### `sim.speed`

Set how much simulated time one tick covers, which is the honest form of a speed control.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `step_ms` | number | optional | simulated milliseconds per tick, which is what the engine actually paces on |
| `factor` | number | optional | a multiple of ten milliseconds per tick, read only when `step_ms` is absent, for scripts written against the old socket |

**Answers** `step_ms`. `step_ms` is what it settled on. Neither parameter positive leaves the tick alone and reports it.

**Example** - ten simulated milliseconds a tick

```json
{"id":1,"method":"sim.speed","params":{"step_ms":10}}
```

**Client** `wb.sim.step_ms = n  /  wb.sim.faster(x)`

### `sim.start`

What a play button presses, which is four different things: it pauses a run already playing, declines while the links are still being measured, brings MeshCore up on every node without playing if this is a firmware run and nothing is up yet, and otherwise starts the clock.

**Takes** nothing.

**Answers** `playing`, `warming`, `starting_firmware`, `started_firmware`. `playing` is the only key always present. `warming` appears when it declined because the link matrix is not measured yet, and `starting_firmware` when it brought the mesh up instead of playing, which is the case where a second call is needed.

**Example** - press play

```json
{"id":1,"method":"sim.start","params":{}}
```

**Client** `wb.sim.start()`

### `sim.state`

Report where the run has got to, cheaply enough to poll and safely enough to call before anything is loaded.

**Takes** nothing.

**Answers** `playing`, `now_ms`, `until_ms`, `events`, `step_ms`, `seed`, `warming`, `links_measured`, `ground`, `reproducible`, `not_reproducible_why`. `until_ms` is zero unless `sim.run` set a limit. `events` is the count since the engine was built, not since the last call. The link measurement has two states: `warming` while it runs, and `links_measured` once every pair has been walked. Neither true is a warm that failed or was cancelled, which finishes its own job row - so a wait for the workbench to go idle returns having waited for nothing, and reading that as a measurement is how a study came to be believed over ground nobody walked. `ground` is what the studies here are standing on, in the shape `terrain.ground` returns. `reproducible` says whether running this scenario again on the same seed would put the same traffic at the same instants: false wherever a node runs in an emulator, because that node's firmware is stepped by the emulator's clock rather than by the run's, and `not_reproducible_why` is the sentence saying which node and what follows from it. A script comparing two runs, or quoting a timing against another run's, has to read it.

**Example** - ask whether the run has finished

```json
{"id":1,"method":"sim.state","params":{}}
```

**Client** `wb.sim.state()`

### `sim.step`

Advance the engine by one tick while the clock is stopped, which is how a paused run is inspected between packets.

**Takes** nothing.

**Answers** `now_ms`. `now_ms` is the simulated clock after the tick. One tick is whatever `sim.speed` last set, not one millisecond.

**Example** - move on one tick

```json
{"id":1,"method":"sim.step","params":{}}
```

**Client** `wb.sim.step()`

### `sim.toggle`

Play if paused and pause if playing, for a control that is one key.

**Takes** nothing.

**Answers** `playing`. `playing` is the state it has just moved to, not the one it was in.

**Example** - flip the clock

```json
{"id":1,"method":"sim.toggle","params":{}}
```

**Client** `wb.sim.toggle()`

### `sim.unverified_wiring`

Allow boards whose wiring nobody has watched boot to run anyway, which is the only way a newly imported board is ever verified.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional, primary | true to lift the gate; absent reads the setting without changing it |

**Answers** `on`. The setting is saved to preferences, so it outlives the session. Turning it on names the boards it has just trusted.

**Example** - ask whether the gate is lifted

```json
{"id":1,"method":"sim.unverified_wiring","params":{}}
```

**Client** `wb.sim.unverified_wiring = bool`

## Firmware

### `firmware.build`

Compile a MeshCore checkout and put what comes out in the library, so one script can compare a stock build against a locally changed one without shelling out to a second binary.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `source` | string | required, primary | the top of a MeshCore checkout, the directory holding src/ and examples/; refused when absent |
| `from` | string | optional | another name for `source`, read only when that is absent |
| `role` | string | optional | build only this role; absent builds both `simple_repeater` and `companion_radio` from the one tree |
| `label` | string | optional | what to call the result in the library and what a node then pins; absent names it after the checkout's git ref |

**Answers** `building`, `source`, `job`. It answers as soon as the build has started, because a role takes a minute or two. Watch the job named in `job`; what came out lands in the library on its own, and a failure reaches the status line with the role and the reason rather than being returned here. The toolchain's own output is discarded.

**Example** - build both roles from a working tree

```json
{"id":1,"method":"firmware.build","params":{"source":"/home/you/src/MeshCore"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.build()`

### `firmware.build_failed`

**The workbench's own callback. The socket refuses it.**

Put the reason a build stopped where somebody will see it, the compiler's own output having gone nowhere.

**Takes** nothing.

**Answers** Handed the failing role and the error as one string. It answers nothing: the sentence goes to the status line.

**Client** none: the build worker telling the store it failed

### `firmware.built`

**The workbench's own callback. The socket refuses it.**

Say what a finished build produced, so a build nothing has named is not a build no picker offers.

**Takes** nothing.

**Answers** `built`. The builder hands it a role-to-result map, not named parameters, and anything else is refused. `built` is those roles and versions as one sorted list of strings.

**Client** none: the build worker telling the store it finished

### `firmware.delete`

Remove one build from the cache, to reclaim the disk or to prove a download works by taking away what it produced.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | required, primary | the build's path, as the library and `firmware.details` give it; refused when absent, and refused when it points anywhere but inside the firmware cache |

**Answers** `deleted`. The build's settings sidecar goes with it, so the next build imported under the same name does not inherit somebody else's answers. Nothing is said about the nodes pinned to it, which keep the name and have nothing to run until they are pinned again.

**Example** - reclaim the disk a build was using

```json
{"id":1,"method":"firmware.delete","params":{"path":"/home/you/.cache/meshbench/firmware/native/repeater-v1.16.0"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.delete(build)`

### `firmware.details`

Report everything known about one build: where it is, what it is, and what has been decided about it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `version` | string | required, primary | the build's version or imported label |
| `role` | string | optional | which role, when one label carries more than one |
| `board` | string | optional | which board; absent means a build for this machine |

**Answers** `role`, `version`, `board`, `native`, `on_disk`, `path`, `settings_path`, `bytes`, `modified`, `in_use`, `kind`, `bootable`, `flash_mb`, `coproc_at_reset`, `card_required`, `notes`. Answered from the library, so a published build nobody has fetched is described too: `on_disk` is then false and `path` and `bytes` are empty. `in_use` counts the nodes pinned to it, `kind`, `bootable` and `flash_mb` are read from a board image and say nothing about a native build, `settings_path` is where its settings would be written whether any exist or not, and `modified` is absent for a build not on this machine. A label naming more than one build is refused rather than guessed at.

**Example** - find out where a build lives and what is running it

```json
{"id":1,"method":"firmware.details","params":{"version":"repeater-v1.16.0"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.details(build)`

### `firmware.download`

Fetch a published build now rather than at the moment a node first needs it, which is what somebody about to work offline wants.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `role` | string | required, primary | the role to fetch, such as `simple_repeater`; refused when absent |
| `version` | string | required | the published release tag; refused when absent |
| `board` | string | optional | the board image to fetch; absent means the native build for this machine |

**Answers** `downloading`, `role`, `version`. It answers as soon as the fetch has been started, not when the file lands. Progress arrives on a job called `fw-<version>-<role>`, counted in kilobytes, and a failure is reported there rather than here; the installed list and the library are re-read either way.

**Example** - fetch a repeater build before working without a network

```json
{"id":1,"method":"firmware.download","params":{"role":"simple_repeater","version":"repeater-v1.16.0"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.download(role, version, board=None)`

### `firmware.failed`

**The workbench's own callback. The socket refuses it.**

Report that bringing the mesh up failed, and cancel a play that was waiting on it rather than advancing a clock over a mesh that is not there.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `reason` | string | optional, primary | what went wrong, said to the operator as it stands |

**Answers** Answers with nothing: it is a report, and what it changes is the status line and whether a waiting play is still waiting.

**Client** none: the firmware starter reporting a failure

### `firmware.import`

Put somebody's own build into the library, which is how a change is tested against a release before it is one.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | required, primary | the file to import; refused when absent, and an ESP32 board's application-only .bin is refused too, because a board starts from the whole flash image |
| `role` | string | required | the role it is imported as; refused when absent |
| `board` | string | optional | the board it is for; absent means a build for this machine |
| `label` | string | optional | what the library will know it by and what a node pins; absent, it is stamped with a timestamp so a second import does not replace the first in place |
| `version` | string | optional | an older name for `label`, read only when `label` is absent, because scripts written against it are already out in the world |

**Answers** `version`, `role`, `board`, `path`, `bytes`. `version` is the label the build was stored under, which is the timestamp when none was given and is what `firmware.set` then has to be handed.

**Example** - test a local change against a published build

```json
{"id":1,"method":"firmware.import","params":{"board":"Heltec_v3","label":"repeater-my-fix","path":"/home/you/MeshCore/.pio/build/Heltec_v3/firmware.factory.bin","role":"simple_repeater"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.import_(path, role, board=None, label="")`

### `firmware.installed`

Read what is actually in the firmware cache on this machine, which is the only thing that decides what a node can start today.

**Takes** nothing.

**Answers** `cache`, `installed`. `installed` is a row per build on disk - `version`, `role`, `board`, `native`, `bytes`, `path` - and is empty on a machine that has downloaded nothing. It says nothing about what is published: `firmware.library` is the list that holds both.

**Example** - see what this machine can run offline

```json
{"id":1,"method":"firmware.installed","params":{}}
```

**Client** `wb.firmware.installed`

### `firmware.library`

List every build there is, on disk and published together, so a build nobody has fetched can still be offered and one imported from a branch still appears.

**Takes** nothing.

**Answers** `builds`, `count`. The catalogue is asked over the network in the background and its answer lands seconds later, so the first call in a session answers from disk alone and is worth making again. Each row in `builds` carries `role`, `version`, `board`, `bytes`, `on_disk`, `path`, `in_use` and `unavailable`.

**Example** - list what could be run, fetched or not

```json
{"id":1,"method":"firmware.library","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.library`

### `firmware.needed`

Ask what a mesh that will not start is missing, by role rather than by node, and what is already installed to fill each gap.

**Takes** nothing.

**Answers** `roles`. `roles` is a list of `{role, nodes, choices}`: how many nodes running that role have no build this machine holds, and the versions installed for it, which may be none. An empty list means every node that runs firmware has one it can start.

**Example** - find out what the run is short of

```json
{"id":1,"method":"firmware.needed","params":{}}
```

**Client** `wb.firmware.needed()`

### `firmware.published`

**The workbench's own callback. The socket refuses it.**

Take the catalogue's answer when it lands and rebuild the library on it, because a fetch nobody re-reads leaves the published builds invisible until something else asks.

**Takes** nothing.

**Answers** `published`, `builds`. Handed the fetch's own list, not named parameters, and anything else is refused. `published` is how many builds the catalogue offered and `builds` how many rows the library holds once they are merged with what is on disk.

**Client** none: the catalogue fetch landing its answer; wb.firmware.scan() asks for one

### `firmware.rescan`

Ask the catalogue what is published again, which is how a build nobody has downloaded becomes offerable.

**Takes** nothing.

**Answers** `scanning`, `count`. `scanning` says a fetch is in flight, and `count` is how many rows the library holds now, which are still the ones from before it lands: read `firmware.library` again a few seconds later. A fetch already running is left alone rather than started twice.

**Example** - look for a build published since this session started

```json
{"id":1,"method":"firmware.rescan","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.scan()`

### `firmware.set`

Pin a build to nodes, which is what decides what each one starts and is the step a run that will not start is usually missing.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `version` | string | required, primary | the version or imported label to pin; refused when absent, but not checked against the library, so a name nothing answers to is only found out at the next start |
| `node` | string | optional | pin this one node by name; absent means every node the role filter leaves |
| `role` | string | optional | only nodes running under this role, pinned or implied by their kind; absent means all of them |
| `board` | string | optional | the board the image is for, so a fleet of emulated nodes can be pinned in one call; absent leaves each node's board as it is, and an explicit empty string moves them back to a build for this machine. Unlike node.set_firmware, absent does not mean native here: clearing three hundred boards is not what a caller who only named a version asked for |

**Answers** `version`, `nodes`, `considered`, `board`. `nodes` is how many were pinned and `considered` how many exist, which counts the ones that never run firmware. With a `role` and no `node` it pins every node running that role, but marks every node in the fleet list as running the version whatever its role, so a call per role leaves the list reading as the last one: pass `node` to pin exactly one. `board` comes back only when it was passed, so a caller can tell a board left alone from one set to a host build.

**Example** - pin one node to the build it will start

```json
{"id":1,"method":"firmware.set","params":{"node":"West Lomond","version":"repeater-v1.16.0"}}
```

**Client** `wb.firmware.use(version, role=|node=)`

### `firmware.start`

Bring MeshCore up on every node that runs one, without starting the clock, so a mesh can be watched settling before any traffic.

**Takes** nothing.

**Answers** `starting`. The answer says the attempt began, not that anything is up: each node attaches on its own goroutine and the outcome arrives later as firmware.started or firmware.failed. Poll firmware.state to see how far it has got.

**Example** - bring the mesh up before pressing play

```json
{"id":1,"method":"firmware.start","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.start()`

### `firmware.started`

**The workbench's own callback. The socket refuses it.**

Carry the count of running firmware back into the world when an attach finishes, and tell whoever pressed play that pressing it again will now start the run.

**Takes** nothing.

**Answers** `running`, `playing`. `playing` appears only where a play was waiting on the mesh coming up, and is false: this reports that the mesh is up, and the next press of play is what starts the run.

**Client** none: the firmware starter reporting back

### `firmware.state`

Ask how far along bringing the mesh up is, which is what every wait for firmware is built on.

**Takes** nothing.

**Answers** `running`, `nodes`, `total`, `starting`. `running` counts the processes that are up and `nodes` the nodes that run firmware at all, so a wait compares those two and never `total`, which counts every node in the scenario including the SDR observers and emitters that never boot one. `starting` is how many attaches are still in flight.

**Example** - ask whether the mesh is up yet

```json
{"id":1,"method":"firmware.state","params":{}}
```

**Client** `wb.firmware.state()  /  wb.firmware.wait_started()`

### `firmware.update`

Rename a build, move it to another board or role, or change how it is run.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `version` | string | required, primary | the build to change, by its current version or label |
| `role` | string | optional | its current role, when one label carries more than one |
| `board` | string | optional | its current board |
| `label` | string | optional | rename it to this; unchanged when absent |
| `new_role` | string | optional | run it as this role instead; unchanged when absent |
| `new_board` | string | optional | move it to this board instead; unchanged when absent |
| `card_required` | bool | optional | this firmware will not get far without storage in the board's slot, so every node running it is given a card whatever it would otherwise have had |
| `coproc_at_reset` | bool | optional | start this build's coprocessors enabled, which the part does not do - for a firmware that traps inside its own exception vector and cannot be seen past |
| `notes` | string | optional | what the next person should know about this build |

**Answers** `role`, `version`, `board`, `path`, `renamed`, `repinned`, `settings`. The build's identity after the change comes back, with `renamed` saying whether the file actually moved and `repinned` how many nodes were pointed at the new name, which happens on their behalf so none is left naming a build nothing answers to. `settings` holds `coproc_at_reset`, `card_required` and `notes` as they now stand. Refused for a build not on this machine, and refused while a node is running it.

**Example** - record what the next person should know about a build

```json
{"id":1,"method":"firmware.update","params":{"notes":"the arm this study compares against","version":"repeater-v1.16.0"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.update(build, label=|new_role=|coproc_at_reset=|notes=)`

### `firmware.window`

**Refuses when no window is attached.**

Open one build's own window: what it is, where it lives, and how it is run.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `version` | string | required, primary | the build's version or imported label |
| `role` | string | optional | which role, when one label carries more than one |
| `board` | string | optional | which board; absent means a build for this machine |

**Answers** `role`, `version`, `board`. The build it opened on, once. Refused where there is no user interface, and refused for a build the library does not hold or a label naming more than one, so nobody is left closing an empty window.

**Example** - open a build to change what it is run as

```json
{"id":1,"method":"firmware.window","params":{"version":"repeater-v1.16.0"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.window(build)`

### `firmware.wipe`

Clear every node's persistent files, which is what belongs between the arms of a comparison: a node keeps its preferences between runs exactly as hardware does, so one that has run before loads its old settings, never reaches a changed default, and returns the same numbers as the arm before it.

**Takes** nothing.

**Answers** `wiped`, `root`, `cards`. `wiped` counts the node directories removed under `root`, and `cards` the storage images a node was given somewhere else, which are wiped too or the claim would be a lie. A storage directory that does not exist yet is not a failure: it answers `wiped` 0 and nothing else.

**Example** - put every node back to factory before the next arm of a study

```json
{"id":1,"method":"firmware.wipe","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.firmware.wipe()`

## Console and fleet

### `console.cli`

Run one meshcore-cli line at a companion node, which has no text console of its own, connecting to it first if nothing has yet.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to run the line at; refused when absent |
| `command` | string | required | the line, in meshcore-cli's own vocabulary; refused when absent or blank, and `?` lists what this build answers |

**Answers** `node`, `reply`, `failed`. `failed` is present and true where the line ran and the answer was no: an unknown command, a command meshcore-cli has that this build does not, or a connect that could not be made. That is not an error, because a console refused at the status bar leaves nothing where it was typed. `reply` is what the console prints either way, and the node's own answer to it arrives later in the scrollback.

**Example** - list the meshcore-cli commands this build answers

```json
{"id":1,"method":"console.cli","params":{"command":"?","node":"West Lomond"}}
```

**Client** `node.companion.cli(line)`

### `console.read`

Read back what a node's firmware has said, which is where the reply to anything typed at it arrives.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node whose scrollback to read; refused when it is absent, unknown, or runs no firmware |

**Answers** `node`, `lines`, `tail`. `lines` is how many the node has said since its console was attached and `tail` is the last two hundred of them, so the two disagree on a node that has been running a while.

**Example** - read what a node has been saying

```json
{"id":1,"method":"console.read","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.console.read()  /  node.console.tail`

### `console.type`

Run a line at a node's own firmware console, which answers what the node says rather than what it sent, and is the question asked when a node is not behaving as its configuration claims.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required | the node whose console to type at; refused when it is absent, unknown, or runs no firmware |
| `command` | string | required | the line to type, sent with a carriage return and newline after it; refused when it is absent or empty |

**Answers** `node`, `sent`, `note`. The node's reply is not in this answer: it lands in the scrollback when the engine next steps, and is read with `console.read`. On a paused clock this steps the engine sixty times so the reply arrives anyway, but while a sweep owns the clock the replies come back empty.

**Example** - ask a node what it believes it is called

```json
{"id":1,"method":"console.type","params":{"command":"get name","node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.console.send(line)`

### `fleet.replies`

**The workbench's own callback. The socket refuses it.**

Read what each node said once the engine has run far enough for it to have answered, and put the rows on the snapshot, which is where a fleet command's real answer arrives.

**Takes** nothing.

**Answers** `replies`. `replies` is how many rows were collected, and the rows themselves go on the snapshot. A node that said nothing reads as "-", and a companion says so in words: it speaks the app protocol rather than the repeater console, so the command reached it and meant nothing.

**Client** none: the reply collector, called only by its own goroutine

### `fleet.send`

Type one repeater command at every node at once, or at a filtered subset, and keep each node's reply apart from the others because the answer worth having is which node disagreed.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `command` | string | required, primary | the line to type, exactly as it would be typed at one node's console; blank or whitespace is refused |
| `node` | string | optional | send to this node alone, matched on the whole name; absent, every node with firmware up is a target |
| `kind` | string | optional | send only to nodes of this kind; absent, kind is not filtered on |

**Answers** `command`, `sent_to`, `replies`, `warning`. The `replies` in this answer are not the replies: they are empty for every node the command reached, and carry a reason only where the send itself failed. A node answers on its own next loop, so the real replies land on the snapshot a second of simulated time later. `warning` appears only for a command that changes what the nodes are, which makes anything already measured a different mesh. It is refused when no node is running firmware.

**Example** - make every repeater advertise itself

```json
{"id":1,"method":"fleet.send","params":{"command":"advert"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.fleet.send(command, kind=|node=)`

Planned, not written: no client defines `wb.fleet` yet - one command to every node of a kind. Call the verb itself in the meantime.

## Companion

### `companion.add_channel`

Ask a node what one of its channel slots holds, despite the name: nothing is added, and the slot's name and key come back later as a frame rather than in this answer.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |
| `index` | number | optional | which channel slot to ask about; absent asks about slot 0 |

**Answers** `asked_for_channel`. It answers as soon as the question is queued. What the slot holds lands in the node's decoded state, which `companion.state` counts.

**Example** - ask what is in the second channel slot

```json
{"id":1,"method":"companion.add_channel","params":{"index":1,"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.add_channel(index)`

### `companion.advert`

Make a node announce itself, which is how the rest of the mesh comes to hold it as a contact.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it, or when it has not answered anything since it was connected |
| `flood` | bool | optional | true sends a flood advert, which repeaters carry onward; false sends one that is not flooded; absent floods |

**Answers** `advert`, `flood`

**Example** - announce to the neighbours only

```json
{"id":1,"method":"companion.advert","params":{"flood":false,"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.advert(flood=False)`

### `companion.configure`

Change a node's own settings the way a phone would, sending only the fields that were asked for, and reporting which ones went.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |
| `name` | string | optional | the name the node advertises itself under; absent or empty leaves it alone |
| `lat` | number | optional | latitude in degrees; absent leaves the advertised position alone, and giving it is what makes `lon` be read at all |
| `lon` | number | optional | longitude in degrees, read only when `lat` was given; absent alongside a `lat` sends zero |
| `tx_dbm` | number | optional | transmit power in dBm; absent leaves it alone |
| `freq_khz` | number | optional | centre frequency in kHz, refused outside 150000 to 2500000; absent with another modem field given keeps what the node last said it holds |
| `bw_khz` | number | optional | bandwidth in kHz, refused outside 7 to 500; absent with another modem field given keeps what the node last said it holds |
| `sf` | number | optional | spreading factor, refused outside 5 to 12; absent with another modem field given keeps what the node last said it holds |
| `cr` | number | optional | coding rate denominator, refused outside 5 to 8; absent with another modem field given keeps what the node last said it holds |
| `path_hash` | number | optional | bytes of path hash each hop adds, 1 to 3, refused outside that; absent leaves it alone |

**Answers** `set`. `set` names what was sent, in the order it went. A call that asks for nothing at all is refused rather than answered with an empty list. The four modem fields go as one command, so any one of them is refused until the node has said what its radio is.

**Example** - turn a node's transmit power down

```json
{"id":1,"method":"companion.configure","params":{"node":"West Lomond","tx_dbm":17}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.configure(...)`

### `companion.connect`

Claim a node's serial port for the companion protocol and make the same opening a phone makes, so a client's view of the node can be shown or driven.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node to attach to; refused when it is absent, runs no firmware, is already connected, or its port is being served to an attached outside client |

**Answers** `connected`. A listener that is serving the port but has nobody on it is taken back rather than refused. Everything the node says in reply arrives later as frames, so read it with `companion.state`.

**Example** - attach to a node the way a phone would

```json
{"id":1,"method":"companion.connect","params":{"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.connect()`

### `companion.disconnect`

Give a node's port back, which is what lets its text console be used again.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |

**Answers** `disconnected`

**Example** - hand the port back to the console

```json
{"id":1,"method":"companion.disconnect","params":{"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.disconnect()`

### `companion.raw`

Put bytes of the caller's own choosing on a node's companion port, for when what the firmware makes of a frame is the thing in question.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |
| `bytes` | array | required | the payload, one number per byte; anything in the array that is not a number is dropped, and an empty or absent array is refused |

**Answers** `sent_bytes`. `sent_bytes` counts what was framed and written, which is the numbers that survived, not the length of the array as it was passed.

**Example** - hand the firmware three bytes and see what it makes of them

```json
{"id":1,"method":"companion.raw","params":{"bytes":[1,2,3],"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.raw(bytes)`

### `companion.read`

Mark which channel a client is looking at so its unread count clears, which is bookkeeping in this session and puts nothing on the wire.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |
| `channel` | number | optional | the channel slot being looked at; absent means slot 0 |

**Answers** `node`, `channel`

**Example** - say the public channel is on screen

```json
{"id":1,"method":"companion.read","params":{"channel":0,"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.messages(channel=)`

### `companion.refresh`

Ask a node again for everything a client draws, emptying the held contact list first so a contact the node has forgotten does not linger in the view.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |

**Answers** `node`. It answers as soon as the questions are queued, so the contact count read straight after is zero. The answers arrive later as frames.

**Example** - rebuild the view from what the node says now

```json
{"id":1,"method":"companion.refresh","params":{"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.refresh()`

### `companion.scope`

Set the region a node sends under, by the one route a companion build has for it, and ask the node back rather than assume the write landed.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |
| `scope` | string | optional | the region name, canonicalised before it is sent and paired with the key derived from it; absent or empty clears the scope, so the node sends unscoped |

**Answers** `node`, `scope`. `scope` is the name as it was asked for, canonicalised, not what the node now holds: the node's own answer arrives later as a frame.

**Example** - put a node on a named region

```json
{"id":1,"method":"companion.scope","params":{"node":"West Lomond","scope":"scotland"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.scope = name`

### `companion.send`

Send a channel message from a node as a phone's composer would, and keep a copy of it, because the node transmits without saying so and a client that draws only what arrives would show an empty conversation.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it, or when it has not answered anything since it was connected |
| `text` | string | required | the message; refused when it is absent or only whitespace |
| `channel` | number | optional | which channel slot to send on; absent sends on slot 0 |
| `path_hash` | number | optional | bytes of path hash each hop adds, 1 to 3, written to the node before the message goes and refused outside that range; absent leaves the node's own setting alone |

**Answers** `sent`, `channel`

**Example** - put a message on the public channel

```json
{"id":1,"method":"companion.send","params":{"node":"West Lomond","text":"anybody hearing this"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion.send(text, channel=, path_hash=)`

### `companion.state`

Read what a client attached to this node would be showing, which is what the node has actually said rather than what the scenario believes about it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the connected node; refused when nothing is connected to it |

**Answers** `node`, `contacts`, `messages`, `channels`, `recent`, `name`, `freq_khz`. `contacts`, `messages` and `channels` are counts rather than the things themselves, and `recent` is the session's own note lines. `name` and `freq_khz` appear only once the node has answered a device query, so their absence means it has not.

**Example** - ask what the client sees

```json
{"id":1,"method":"companion.state","params":{"node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.companion  (properties)`

## Serving to real clients

### `bench.drop`

Take a served node's port back, or with no node named cut every attached client loose while leaving the listeners open.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional | the node to stop serving, whether or not a client is on it; absent drops the clients instead and stops serving nothing |

**Answers** `dropped`. `dropped` counts what was closed, and zero is a normal answer: named a node nothing was serving, or asked to drop clients when none were attached.

**Example** - stop serving one node

```json
{"id":1,"method":"bench.drop","params":{"node":"West Lomond"}}
```

**Client** `node.unserve()`

### `bench.refresh`

Look again at which endpoints are open and which have a client on them, because a client attaching or leaving tells this session nothing.

**Takes** nothing.

**Answers** It answers with nothing at all. The endpoints go into the published state, which is where a panel reads them.

**Example** - see whether a client has attached

```json
{"id":1,"method":"bench.refresh","params":{}}
```

**Client** `wb.endpoints`

### `bench.serve`

Open a real endpoint onto one simulated node, so an unmodified companion client on this machine or another can talk to it as if it were hardware.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional | the node to serve; absent takes the first companion in the scenario, because an endpoint is what is wanted before a node has been chosen |
| `kind` | string | optional | "serial" for a pseudo-terminal a serial client opens; anything else, absent included, listens on TCP on every interface and a port the operating system picks the first time this node is served |

**Answers** `node`, `addr`. Serving takes the port off the workbench, so a companion session on that node is released rather than shared. A second call for the same node replaces the listener rather than adding one, and keeps the address: the port is drawn once per node and asked for again on every serve after, including after a drop, because a client already pointed at the first address cannot be told about a second. A port taken by something else in the meantime moves the endpoint and says so.

**Example** - put a node on a TCP port for a real client

```json
{"id":1,"method":"bench.serve","params":{"kind":"tcp","node":"West Lomond"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.serve(kind='tcp'|'serial')`

### `bench.stray`

Hand a companion a frame it was never going to be sent, so what the decoder does with rubbish can be watched rather than assumed.

**Takes** nothing.

**Answers** `at`. It injects at the first companion in the scenario, and names which that was. Refused where there is no run, or where the scenario holds no companion at all.

**Example** - put a bad frame at a companion

```json
{"id":1,"method":"bench.stray","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.endpoints.stray()`

Planned, not written: no client defines `wb.endpoints` yet - the TCP and serial endpoints a node is served on. Call the verb itself in the meantime.

### `sdr.serve`

Offer what one node's antenna hears as an rtl_tcp source, so real SDR software can be pointed at the simulated spectrum rather than at a drawing of it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node whose antenna is served; refused when it is absent, not in the scenario, or not in the engine |

**Answers** `node`, `addr`, `rate_hz`. `rate_hz` is the node's own receiver bandwidth, one sample per hertz, and 250 kHz where the scenario states none. It is what the stream is rendered at, not what a client is held to: the client's own rate setting is followed. Serving a node already served replaces the listener and keeps the address, and so does stopping and serving again, so SDR software pointed at it once stays pointed at it; a port taken by something else in the meantime moves the endpoint and says so. The IQ is signal only, with the noise floor added at the server, so a paused run streams a bare floor rather than stopping.

**Example** - point SDR software at a node's antenna

```json
{"id":1,"method":"sdr.serve","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.serve_sdr()`

### `sdr.stop`

Close a node's rtl_tcp listener and stop rendering its IQ, which is work a run keeps doing for as long as it is served.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the served node; refused when it is absent or not being served, so a stop is never mistaken for having worked |

**Answers** `stopped`. Any client on the line is cut rather than told, the same way unplugging a dongle would.

**Example** - stop serving a node's antenna

```json
{"id":1,"method":"sdr.stop","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `node.unserve_sdr()`

## Events, packets and capture

### `capture.file`

Write every receiver's view of every frame to a pcapng file as the run happens, which is how the bytes are kept on a session with nobody at the screen.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | optional, primary | where to write it; absent writes meshbench-capture.pcapng in the temporary directory, and whatever is at the path already is replaced |

**Answers** `path`. It replaces any capture already running, file or stream, so the frames go to one place at a time. Refused where no network is loaded.

**Example** - keep the run's frames for later

```json
{"id":1,"method":"capture.file","params":{"path":"/tmp/lomond.pcapng"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.capture.start(path)`

Planned, not written: no client defines `wb.capture` yet - frame capture to a pcapng, and the Wireshark launch beside it. Call the verb itself in the meantime.

### `capture.stop`

Close whichever capture is running, file or stream, and say how much of the run it caught.

**Takes** nothing.

**Answers** `path`, `frames`. `path` is the file that was written, or the address that was being streamed to. Both come back empty with `frames` zero where nothing was capturing, which is not an error; the only refusal is no network loaded.

**Example** - finish the capture and count it

```json
{"id":1,"method":"capture.stop","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.capture.stop()`

Planned, not written: no client defines `wb.capture` yet - frame capture to a pcapng, and the Wireshark launch beside it. Call the verb itself in the meantime.

### `capture.wireshark`

Stream the same frames as datagrams to 127.0.0.1:5555 and open Wireshark on loopback filtered to that port, with both dissectors loaded in the order that makes them read.

**Takes** nothing.

**Answers** `addr`, `how`, `dissector_error`, `dissector_warning`, `launched`, `launch_error`. The stream is started before anything is launched and stays up whatever happens next, so `launched` false is a window that did not open rather than a capture that did not start: `how` is then the command to run by hand. `dissector_error` says the Lua that registers the port was not found beside the binary, and `dissector_warning` that only the MeshCore half is missing. The capture belongs to the engine as it stands, so it is started once per session and not once per run: an engine rebuilt, or a workbench restarted, is capturing nothing until this is called again.

**Example** - watch the mesh in Wireshark

```json
{"id":1,"method":"capture.wireshark","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.capture.wireshark()`

Planned, not written: no client defines `wb.capture` yet - frame capture to a pcapng, and the Wireshark launch beside it. Call the verb itself in the meantime.

### `events.dump`

Write the event log to disk as NDJSON, one event per line, because a run's log is appended to and read back a line at a time and a single JSON array can be neither streamed nor tailed.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | optional, primary | where to write; absent it goes to meshbench-events.ndjson in the temporary directory, and an existing file is overwritten rather than appended to |

**Answers** `path`, `written`, `total`. `written` is how many lines the file got and `total` how many the run has produced. They differ on any long run, because the store keeps a bounded tail rather than the whole log, and the difference is not the file being truncated by a bug. An event whose signal-to-noise ratio has no finite value is written with `snr_db` null, JSON having no way to say infinity.

**Example** - keep a run's log for something else to read

```json
{"id":1,"method":"events.dump","params":{"path":"/tmp/run-events.ndjson"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.events.dump(path)`

### `events.recent`

Read the end of the event log, which is what a caller polls a run with rather than asking for the whole thing every second.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `limit` | number | optional, primary | how many of the most recent events to return; anything not a positive number leaves it at 50, and asking for more than the store keeps returns what there is |

**Answers** `events`, `total`, `shown`. `shown` is how many rows came back and `total` how many the run has produced since the engine was built, which is much larger: the store keeps a bounded tail, not the whole log, so `total` cannot be used to page backwards. An event whose signal-to-noise ratio has no finite value comes back with `snr_db` null.

**Example** - poll what has just happened

```json
{"id":1,"method":"events.recent","params":{"limit":20}}
```

**Client** `wb.events.recent(limit=)`

### `packet.close`

Put the open packet away, which is also what stops the per-tick rebuild that keeps an open one live.

**Takes** nothing.

**Answers** Nothing comes back, whether or not a packet was open: the answer is null and the call cannot fail.

**Example** - stop following a packet

```json
{"id":1,"method":"packet.close","params":{}}
```

**Client** `wb.packets.close()`

Planned, not written: no client defines `wb.packets` yet - opening one captured frame. Call the verb itself in the meantime.

### `packet.open`

Dissect one transmission and gather what every node did with it, and leave it as the packet the view keeps following while its message is still spreading.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `id` | number | required, primary | the transmission's id; absent or zero is refused |
| `seek` | number | optional | step this many ids at a time until a transmission that is still in the ledger is found, up to fifty tries, which is what the previous and next arrows send; absent means take the given id or nothing |

**Answers** `id`, `origin`, `heard`, `missed`, `transmissions`, `reached`. `id` is the transmission actually opened, which is not the one asked for when `seek` walked to a neighbouring one. `transmissions` counts every time the message was put on the air, so a relayed flood is told from a single advert without reading the header, and `reached` is how many distinct nodes heard any of them. Refused where no engine is built, and where nothing of that id is left in the event log.

**Example** - open the first transmission of a run

```json
{"id":1,"method":"packet.open","params":{"id":1}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.packets.open(id, seek=)`

Planned, not written: no client defines `wb.packets` yet - opening one captured frame. Call the verb itself in the meantime.

### `waterfall.capture`

Take one 200 ms window of what a node's receiver hears and turn it into a spectrogram, of the instant it was asked for rather than of whenever a worker got round to it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | the node to listen at; absent falls back to whichever node is selected, and a name this network has not got is refused rather than read as none |

**Answers** `captured`. `captured` is false without an error whenever there was nothing to draw: no engine, no node selected, or nothing on the air in that instant. Which of the three it was is said on the status line rather than returned.

**Example** - look at what one node hears during a flood

```json
{"id":1,"method":"waterfall.capture","params":"West Lomond"}
```

**Client** `wb.capture.waterfall(node)`

Planned, not written: no client defines `wb.capture` yet - frame capture to a pcapng, and the Wireshark launch beside it. Call the verb itself in the meantime.

## Links, budgets and profiles

### `budget.for_selection`

Break the selected node's strongest measured link into the decibels it is made of, both ways, and cut the terrain through the same pair so the picture and the margins cannot tell different stories.

**Takes** nothing.

**Answers** `budgets`. `budgets` is 2 when there was a link to break down and 0 otherwise - nothing selected, no engine built, or no link measured yet - which is a state rather than an error. The two budgets are the two directions and their totals differ: each end's antenna gain is evaluated on the bearing towards the other, so a beam is a different antenna each way round. Both are best cases, with no multipath, no body loss and no oscillator error in them. The breakdown itself goes into the snapshot rather than into this answer, and the cut-through arrives later still.

**Example** - see what the selected node's best link is made of

```json
{"id":1,"method":"budget.for_selection","params":{}}
```

**Client** `wb.links.budget()`

Planned, not written: no client defines `wb.links` yet - the link matrix, one pair, and a terrain profile through it. Call the verb itself in the meantime.

### `link.pair`

Answer why two particular places do or do not hear each other, without the engine or a warm link matrix, which are exactly what is missing at the moment somebody points at two masts and asks.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `a` | object | required | one end: a node's name as a bare string or as {node}, or a place as {lat, lon} with an optional height_m that defaults to 2 m head height; anything else is refused, as is a name this network has not got |
| `b` | object | required | the other end, in the same two forms; refused when it labels the same place as a, since a link needs two |

**Answers** `from`, `to`, `ground`. It answers with the two labels as soon as the worker starts, and with the `ground` between them in the shape `terrain.ground` returns. Said rather than refused, unlike the rasters: this verb exists to answer before a warm has happened, and a cut-through with nothing under it is visibly flat. The cut-through and both margins arrive later through the internal `link.pair_set`, and there are two margins because there are two answers: each end's gain is evaluated on the bearing towards the other, so A to B and B to A can differ by tens of decibels on a beam. Both are best cases - bare earth, the calibrated excess loss, a default noise floor and no multipath - which is what the profile's assumption line says. A clicked place with no scenario loaded is priced at 868 MHz, and says so.

**Example** - ask why two repeaters do or do not hear each other

```json
{"id":1,"method":"link.pair","params":{"a":"West Lomond","b":"Dunfermline"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.links.pair(a, b)`

Planned, not written: no client defines `wb.links` yet - the link matrix, one pair, and a terrain profile through it. Call the verb itself in the meantime.

### `link.pair_set`

**The workbench's own callback. The socket refuses it.**

Take the finished cut-through and its two budgets into the snapshot in one go, so the panel's picture and its margins are always of the same pair and the same model.

**Takes** nothing.

**Answers** `from`, `to`, `km`, `edges`. Answers nothing at all when the analysis could not run, having cleared the profile: the reason has already been said on the status line by the worker.

**Client** none: the pair worker publishing its answer

### `link.profile`

Cut the terrain through whatever two nodes are selected, for a script or a capture that has no panel to press, where the interface reaches the same worker through budget.for_selection.

**Takes** nothing.

**Answers** `from`, `to`. Refuses unless two nodes are selected, and takes the first and the last of them when more are. It answers with the two ends as soon as the worker starts; the profile lands later through the internal `link.profile_set` and carries no margins at all - both directions read 0 dB, because this draws the ground and `budget.for_selection` is what prices the link.

**Example** - cut through between the two selected nodes

```json
{"id":1,"method":"link.profile","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.links.profile(a, b)`

Planned, not written: no client defines `wb.links` yet - the link matrix, one pair, and a terrain profile through it. Call the verb itself in the meantime.

### `link.profile_set`

**The workbench's own callback. The socket refuses it.**

Hold the finished cut-through for the panel to draw, or clear it where the analysis could not run.

**Takes** nothing.

**Answers** `from`, `to`, `km`, `edges`. Answers nothing at all when it is handed no profile, which is how a failed analysis takes the old picture off the panel rather than leaving one of the wrong pair there.

**Client** none: the profile worker publishing its answer

### `links.recompute`

Measure every pair's path loss again over the real terrain, which is what a moved node, a changed radio or a new calibration needs before any margin on screen means anything.

**Takes** nothing.

**Answers** `warming`. It answers the instant the work is queued, never with the links: a full matrix is tens of thousands of terrain profiles and minutes of them, so it runs as the job `links` and the links land on the world when it finishes. Watch job.list for it.

**Example** - re-measure after moving a node

```json
{"id":1,"method":"links.recompute","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.links.recompute()`

Planned, not written: no client defines `wb.links` yet - the link matrix, one pair, and a terrain profile through it. Call the verb itself in the meantime.

### `links.set`

**The workbench's own callback. The socket refuses it.**

Take a finished link matrix onto the world and rebuild the link budget for whatever node is selected, since a budget is about a link and cannot exist before the links do.

**Takes** nothing.

**Answers** `links`. It carries a []state.Link and refuses anything else rather than emptying the matrix, which is what a caller from outside the process would otherwise do by accident.

**Client** none: the warm publishing its matrix

### `study.margin`

Read or set how far outside the study boundary a node still counts, which decides what an imported network keeps.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `km` | number | optional, primary | kilometres beyond the boundary; a negative number is ignored and only reads the current value |

**Answers** `km`

**Example** - keep nodes up to twenty kilometres outside the boundary

```json
{"id":1,"method":"study.margin","params":{"km":20}}
```

**Client** `wb.study.margin_km = n`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

## Coverage and planning

### `coverage.clear`

Take the raster off the map without recomputing anything, for when it is covering the ground somebody wants to look at.

**Takes** nothing.

**Answers** Answers nothing at all: there is no state left to report.

**Example** - put the map back

```json
{"id":1,"method":"coverage.clear","params":{}}
```

**Client** `wb.study.clear_coverage()`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `coverage.combined`

**The workbench's own callback. The socket refuses it.**

Turn a finished stack of rasters into the three numbers the planning question was actually about, and take the job off the status line.

**Takes** nothing.

**Answers** `mode`, `gap_cells`, `known_cells`, `redundancy`, `single_point_of_failure`. `known_cells` is the cells any raster had an answer for, so `gap_cells` is out of what was known rather than out of the whole grid. It refuses when the combine produced nothing.

**Client** none: the raster worker publishing the network-wide answer

### `coverage.compute`

Raster what one node reaches over its own 60 km study square, which is the question somebody has just asked by clicking a mast.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | the node to cover from; absent falls back to whichever node is selected, and a name this network has not got is refused rather than read as none |

**Answers** `nodes`, `started`. It answers as soon as the job starts, with `nodes` at 1 for the single station. The raster lands later through the internal `coverage.set`, and a failure through `coverage.failed`. Every cell is judged in both directions and they differ: the antenna's gain is evaluated on the bearing and look angle to that cell, and a node imported with position uncertainty carries that uncertainty into the cell as slack. The margins are a best case, with no multipath and no body loss in them, and cells with no cached elevation are counted rather than coloured.

**Example** - see the ground one repeater serves

```json
{"id":1,"method":"coverage.compute","params":"West Lomond"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.study.coverage(node)`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `coverage.failed`

**The workbench's own callback. The socket refuses it.**

Say on the status line why a raster job gave up, because a job that ends with nothing drawn is otherwise indistinguishable from one still running.

**Takes** nothing.

**Answers** Answers nothing.

**Client** none: the raster worker reporting a failure

### `coverage.map`

Raster where the network works rather than what one mast reaches, by pricing every repeater and room server over one shared grid and keeping the best two-way server in each cell.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `cells` | number | optional, primary | cells on the long edge; absent uses the saved coverage.resolution, and a value outside 64 to 4096 is refused rather than clamped |
| `station` | string | optional | cover from this one node instead of the whole network, or "selected" for whatever the map has under the cursor; absent covers every repeater and room server, a name this network has not got is refused, and "selected" with nothing selected is refused too |
| `south` | number | optional | the viewport's southern border in degrees, -90 to 90; all four borders or none, since three and a typo is refused rather than quietly rastered over other ground |
| `north` | number | optional | the northern border, -90 to 90, and above south |
| `west` | number | optional | the western border, -180 to 180 |
| `east` | number | optional | the eastern border, -180 to 180, and right of west |

**Answers** `nodes`, `started`, `ground`. It answers as soon as the job starts, `nodes` being how many stations went into it and `ground` being what it stood on, in the shape `terrain.ground` returns. A raster over bare earth nobody chose is refused outright rather than drawn: free space closes every link a hill would have blocked, and the picture is read as where the network works. An operator who has answered the terrain question - either way - gets the raster and the note that goes with it, because an offline run over cached ground is what refusing downloads is for. The raster lands later through the internal `coverage.set`, the network-wide summary through `coverage.combined`, and a failure through `coverage.failed` where almost none of the ground has cached elevation. With no viewport given it covers the study boundary, and with no boundary the network's own box plus 15 km. Each cell keeps both directions and they differ: gain is evaluated per station on the bearing and look angle to that cell, a station imported with position uncertainty carries it into the cell as slack, and the margins are a best case, with no multipath and no body loss in them.

**Example** - raster where the whole network works, finer than the default

```json
{"id":1,"method":"coverage.map","params":{"cells":480}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.study.coverage_map()`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `coverage.resolution`

Read or set how sharp every shared-grid raster is, which is the one knob that trades minutes for detail: the cost scales with the square of it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `cells` | number | optional, primary | cells on the raster's long edge, 64 to 4096; absent reads the current setting without changing it, and a value outside that range or one that is not a number is refused rather than clamped, since a caller who asked for 30,000 cells and silently got 240 has been told a picture is sharp when it is not |

**Answers** `cells`. `cells` is always the setting as it stands after the call. Setting it writes the preferences rather than the scenario, because a resolution is a machine-and-patience choice and not a network's, so it outlives whichever scenario was open at the time.

**Example** - ask how sharp the rasters are

```json
{"id":1,"method":"coverage.resolution","params":{}}
```

**Client** `wb.study.coverage_cells = n`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `coverage.set`

**The workbench's own callback. The socket refuses it.**

Take a finished raster into the snapshot and say how much of it is ignorance rather than absence, since a raster computed with no elevation looks exactly like a statement about radio.

**Takes** nothing.

**Answers** `node`. Answers nothing at all when it is handed a nil raster, which is how the map is cleared.

**Client** none: the raster worker publishing its answer

### `coverage.start`

Ask one of the network-wide planning questions by name - where nobody is reached, how many servers a covered cell has, and how much of the map one mast is carrying alone.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `mode` | string | optional, primary | one of best, best-server, gaps, redundancy or node; absent is best, an unknown one is refused, and node hands the call to coverage.compute for the selected node and refuses when nothing is selected |

**Answers** `mode`, `nodes`, `started`, `ground`. It answers as soon as the job starts, with `ground` saying what it stood on in the shape `terrain.ground` returns; over bare earth nobody chose it is refused rather than answered, because a gap found over free space is somewhere a hill would have made pointless. The answer itself - gap cells, servers per covered cell, cells depending on one - arrives later through the internal `coverage.combined`, and a failure through `coverage.failed`. `nodes` is every node in the scenario, companions included, because this rasterises the lot over one shared grid rather than the infrastructure alone that `coverage.map` picks out. Mode node answers with `coverage.compute`'s keys instead of these. Each raster is a best case in both directions, with gain evaluated towards each cell and any position uncertainty carried into it.

**Example** - find the ground nobody reaches

```json
{"id":1,"method":"coverage.start","params":{"mode":"gaps"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.study.coverage(mode=)`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `energy.for_selection`

Run the same year of sun and battery at whichever node is selected, for the panel that has a selection and no name.

**Takes** nothing.

**Answers** `node`. Refused outright unless MESHBENCH_ENERGY is set, and refused again with nothing selected. The year goes into the snapshot and onto the status line rather than into this answer, and the duty cycle it is run at is the one the run measured, which is zero where no run has happened yet.

**Example** - size the selected site's panel and pack

```json
{"id":1,"method":"energy.for_selection","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.study.energy()`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `plan.failed`

**The workbench's own callback. The socket refuses it.**

Put a route search's error in the status line, which is the only place it can go: the search runs on a worker, long after the verb that started it answered.

**Takes** nothing.

**Answers** Nothing. It exists to say something to the operator.

**Client** none: the planner reporting a failure

### `plan.routes`

Search for the paths a message could take between the first and last selected nodes, on a worker, and answer with only the pair it has gone off to search between.

**Takes** nothing.

**Answers** `from`, `to`. It takes its ends from the selection rather than from parameters, and refuses where fewer than two nodes are selected. The routes themselves arrive later through an internal callback and land on the world; nothing here waits for them, and a search that fails says so in the status line rather than in this answer.

**Example** - why a message gets from one end to the other

```json
{"id":1,"method":"plan.routes","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.study.plan(a, b)`

Planned, not written: no client defines `wb.study` yet - coverage, planning and the study margin. Call the verb itself in the meantime.

### `plan.set`

**The workbench's own callback. The socket refuses it.**

Take a finished route search onto the world, which is the only place the paths become something the map can draw.

**Takes** nothing.

**Answers** `routes`. Anything that is not a []state.Route leaves the world holding no routes rather than being refused, so a caller from outside the process would silently erase them - which is why the socket is not allowed to reach it.

**Client** none: the planner publishing its answer

## Boundary, import and feeds

### `boundary.accept`

Take one of the places the search offered into the study area, which unions rather than replaces: Scotland and Ireland is two accepts and then one prune, not two calls where the second wins.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | which of the offered places to take, matched without regard to case and on any part of the offered name, so "Scotland" takes "Alba / Scotland"; an empty one is refused, and one that matches nothing is refused with the list of what was offered |

**Answers** `accepted`, `areas`. `accepted` is the gazetteer's own name for what was taken, which is not always what was asked for, and `areas` how many the study area now holds. Accepting the same place twice is answered rather than refused, and does not stack it. This changes what is measured, never what is loaded: the nodes outside stay until boundary.prune.

**Example** - add a searched place to the study area

```json
{"id":1,"method":"boundary.accept","params":{"name":"Fife"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.boundary.accept(name)`

### `boundary.list`

Say which areas the study is made of, which is how a caller outside the window finds out whether an accept or a load took.

**Takes** nothing.

**Answers** `areas`, `names`. `areas` is a row per area with its name and how many rings and points it is drawn from, and `names` the same names on their own, ready for boundary.remove. The geometry itself is not returned: a national boundary is megabytes of coordinates. Both are empty when no study area has been set, which is not an error.

**Example** - check what the study area holds

```json
{"id":1,"method":"boundary.list","params":{}}
```

**Client** `wb.boundary.list()`

### `boundary.load`

Take a study area from GeoJSON rather than from the gazetteer, which is the only way to study a catchment, a valley or a polygon somebody drew this morning, and the only way to set one offline.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | optional, primary | a file to read the GeoJSON from; a file over 64 MB is refused, and giving neither this nor geojson is refused, as is giving both |
| `geojson` | string | optional | the document itself, for a caller that generated the polygon and should not have to write it to disk first |
| `name` | string | optional | what to call the area; for a single polygon it wins outright, for several it only fills in the ones the file left unnamed, and absent it falls back to the file's own name and then to a number |
| `name_field` | string | optional | the feature property to read each name from; absent it is "name", which is what almost every file calls it |

**Answers** `loaded`, `areas`, `polygons`. `polygons` is what the document held and `loaded` names only those actually added, so it is shorter when an area of that name was already in the study and empty when all of them were. `areas` is the size of the whole study area afterwards. A GeoJSON coordinate is longitude then latitude, which is the opposite way round to everything else here.

**Example** - study a polygon the gazetteer has no name for

```json
{"id":1,"method":"boundary.load","params":{"geojson":"{\"type\":\"Polygon\",\"coordinates\":[[[-3.5,56.0],[-3.2,56.0],[-3.2,56.3],[-3.5,56.3],[-3.5,56.0]]]}","name":"Lomond hills"}}
```

**Client** `wb.boundary.load() / wb.boundary.use()`

### `boundary.prune`

Delete the nodes outside the study area, keeping a margin because a node just outside the border still relays to and interferes with one just inside it and dropping it makes the mesh behave better than reality.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `margin_km` | number | optional, primary | how far outside a node may be and still be kept; absent or negative leaves it at the session's own margin |

**Answers** `removed`, `nodes`. `nodes` is how many are left. Removing none is answered with zero and touches nothing; removing any rebuilds the engine and empties the link matrix while every remaining pair is measured again. It is refused when no study area has been accepted, rather than treated as an area that contains nothing.

**Example** - cut an imported network down to the study area

```json
{"id":1,"method":"boundary.prune","params":{"margin_km":15}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.boundary.prune(margin_km=)`

### `boundary.remove`

Take one area back out of a study area built from several, so a wrong accept costs one call rather than starting the search over.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the area to drop, matched whole and without regard to case; an empty one is refused, and one that names no accepted area is refused with the list of what there is |

**Answers** `removed`, `areas`. `areas` is how many are left. Like accepting, this changes what is measured and not what is loaded: nodes pruned away on the old area do not come back.

**Example** - drop an area from the study without starting again

```json
{"id":1,"method":"boundary.remove","params":{"name":"Fife"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.boundary.remove(name)`

### `boundary.set`

Look a place up in the gazetteer and offer what it matched, which is the search half of choosing a study area and changes nothing on its own.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `query` | string | required, primary | the place to search for; blank or whitespace is refused rather than answered with everything |

**Answers** `found`, `names`. `found` is a row per match with its name and kind, and `names` the same names on their own, for handing straight to boundary.accept. Nothing joins the study area until one is accepted, and the names are the gazetteer's own: a search for Scotland comes back as "Alba / Scotland". It needs the network and gives the geocoder thirty seconds.

**Example** - find a council area to study

```json
{"id":1,"method":"boundary.set","params":{"query":"Fife"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.boundary.search(query)`

### `feed.failed`

**The workbench's own callback. The socket refuses it.**

Say the live feed came back with nothing and why, kept separate from the import's own failure because a deployment can publish nodes and not receptions.

**Takes** nothing.

**Answers** Nothing: it says so and returns nil.

**Client** none: the feed reporting a failure

### `feed.pull`

Fetch the last hour of a deployment's real receptions and put them beside this scenario's links, which is what turns a simulation into something with a measured answer to compare against.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `url` | string | required, primary | the deployment to pull from, under this key or as a bare string; an empty one is refused, and an object whose one key is something else is read as the url it holds, the way the old socket's callers write it |

**Answers** `url`. It returns as soon as the pull is accepted, not when the receptions land: they arrive later, and the residuals against this scenario's links are computed with them rather than behind a second call. A pull that comes back after feed.stop is thrown away. The fetch has ninety seconds.

**Example** - follow what a real deployment is hearing

```json
{"id":1,"method":"feed.pull","params":{"url":"https://map.example.net"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.feed.pull(url)`

Planned, not written: no client defines `wb.feed` yet - the live reception feed, which wb.live half covers and does not name. Call the verb itself in the meantime.

### `feed.set`

**The workbench's own callback. The socket refuses it.**

Take the receptions a pull came back with and compute the residuals against this scenario's links in the same step, so observed and predicted are never one button apart.

**Takes** nothing.

**Answers** `receptions`. `receptions` is everything pulled; how many of them matched a link in this scenario is a smaller number, and it goes on the snapshot rather than in this answer.

**Client** none: the feed publishing receptions

### `feed.stop`

Stop following a deployment's live traffic, which means not starting the next pull and throwing away the one still in the air rather than closing a connection.

**Takes** nothing.

**Answers** `stopped`. `stopped` is false when no feed was running, which is an answer rather than a refusal. Receptions already pulled stay where they are.

**Example** - stop following the live traffic

```json
{"id":1,"method":"feed.stop","params":{}}
```

**Client** `wb.feed.stop()`

Planned, not written: no client defines `wb.feed` yet - the live reception feed, which wb.live half covers and does not name. Call the verb itself in the meantime.

### `import.commit`

Make the fetched nodes the scenario, either in place of what is loaded or alongside it, and start measuring the links again.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `strategy` | string | optional, primary | "replace-all" for the imported network on its own, "add" to keep what is already loaded and add the names it has not got; absent it is replace-all, and anything else is refused - "replace" is not a strategy name, and a caller who writes it gets an error rather than a network with the demonstration nodes still in it |

**Answers** `nodes`, `strategy`. `nodes` is the size of the scenario afterwards, not how many arrived. It returns before the links are measured: every pair is a path loss over real terrain, so that runs as a job and the link matrix is empty until it finishes.

**Example** - make the imported deployment the whole scenario

```json
{"id":1,"method":"import.commit","params":{"strategy":"replace-all"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.import_.commit(strategy=)`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

### `import.describe`

Count what a deployment would bring in, without setting it as the import source or changing a node, so a URL can be weighed before it is committed to.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `url` | string | required, primary | the deployment to read, under this key or as a bare string; a call with no url in it is refused outright rather than starting a ninety second read of the empty string and answering as though it had been accepted. Whether the url is reachable is still the read's business, and a failure there arrives on the snapshot a moment later |

**Answers** `url`. It returns at once with the URL it started on. The counts arrive later on the snapshot, as records, importable, no position and placed loosely: the last of those is the nodes whose published position is too loose to trust to a decibel, which are kept and marked rather than dropped. An accepted study area narrows it, which is read here rather than in the worker. The read has ninety seconds.

**Example** - count what a deployment holds before importing it

```json
{"id":1,"method":"import.describe","params":{"url":"https://map.example.net"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.import_.describe(url)`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

### `import.failed`

**The workbench's own callback. The socket refuses it.**

Say a read failed and end the traffic job with it, so a scripted import fails at once instead of waiting out its whole timeout on a job that will never finish.

**Takes** nothing.

**Answers** Nothing: it marks the job failed and returns nil.

**Client** none: the fetch reporting a failure

### `import.fetch`

Read a deployment's nodes and say what committing them would change, before anything in the scenario changes.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `url` | string | optional, primary | the deployment to read, remembered as the source; absent, the source already set is used, and the fetch is refused when there is none |

**Answers** `records`, `nodes`, `skipped_no_position`, `uncertain`. `records` is what the deployment published and `nodes` what survived: a node with no position is dropped, and one outside an accepted study area and its margin is dropped too. `uncertain` counts the ones placed more loosely than a kilometre, which are kept and marked rather than dropped, because a node imported at plus or minus 5 km cannot be given a confident answer and the mark is what carries that. Nothing is committed until import.commit.

**Example** - see what a deployment holds before taking it

```json
{"id":1,"method":"import.fetch","params":{"url":"https://map.example.net"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.import_.fetch(url)`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

### `import.set`

**The workbench's own callback. The socket refuses it.**

Put the finished description on the snapshot and say what it found, which is how import.describe answers at all.

**Takes** nothing.

**Answers** Nothing: it writes the snapshot and returns nil.

**Client** none: the fetch publishing its preview

### `import.set_source`

Name the CoreScope deployment every later import and inference verb reads from, so they need not be told twice.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `url` | string | required, primary | the deployment's base URL; an empty one is refused, and a trailing slash is trimmed rather than passed on |

**Answers** `url`. The URL that comes back is the trimmed one, which is what the fetch will actually ask.

**Example** - point the import at a deployment

```json
{"id":1,"method":"import.set_source","params":{"url":"https://map.example.net"}}
```

**Client** `wb.import_.source = url`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

### `infer.apply`

Write the inferred regions onto the nodes, which is the step that gets forgotten and the one that decides whether anything relays: without it a mesh has regions inferred and not applied, which transmits everything, relays nothing and reports no error.

**Takes** nothing.

**Answers** `applied`. `applied` is how many nodes were written to, and 0 is the answer worth reading: the inference ran and nothing was written back. It matches on the public key a node kept from the feed and falls back to the name, so it only reaches nodes that were seen on the real network. It is refused outright when nothing has been inferred yet.

**Example** - apply what the traffic proved about which node relays what

```json
{"id":1,"method":"infer.apply","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.import_.apply_inference()`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

### `infer.progress`

**The workbench's own callback. The socket refuses it.**

Carry the running packet count from the reading goroutine into the traffic job, so a long read shows movement rather than a bar that has stopped.

**Takes** nothing.

**Answers** Nothing: it updates the job and returns nil.

**Client** none: the traffic reader saying how far it has got

### `infer.result`

**The workbench's own callback. The socket refuses it.**

Turn the packets the reader collected into the per-node region inference and end the traffic job.

**Takes** nothing.

**Answers** `packets`, `nodes`, `regions`. `regions` is a map of region name to how many nodes hold it. The inference is held for infer.apply and reaches no node until that is called.

**Client** none: the traffic reader handing its packets back; wb.import_.inference reads the answer

### `infer.run`

Read a window of the deployment's own traffic and work out which regions each node holds, which is the only honest source for what a real mesh forwards.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `hours` | number | optional, primary | how far back to read, in hours; anything not positive leaves it at 168, a week |

**Answers** `reading`, `hours`. It returns as soon as the read is started, not when it finishes: the packets come back later through the reader's own callback, which ends the `infer` job and reports how many nodes were seen, and a failed read ends the same job. It is refused when no import source has been set. Nothing reaches the nodes until infer.apply.

**Example** - read a week of traffic to see what each node relays

```json
{"id":1,"method":"infer.run","params":{"hours":168}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.import_.infer(hours=)`

Planned, not written: no client defines `wb.import_` yet - bringing a real deployment in; wb.live carries the feed half of it. Call the verb itself in the meantime.

## Experiments and sweeps

### `experiment.base`

Set the two timings every arm shares, how long a cell runs and when its burst is fired, and deliberately nothing else: the firmware versions belong to the arms.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `run_for_ms` | number | optional | how long each cell runs, in simulated milliseconds; zero or less is ignored and the current length kept |
| `send_at_ms` | number | optional | the simulated instant the burst is fired, the same in every arm; zero or less is ignored |

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`. The same summary experiment.define answers with, so the arms and senders it reports are whatever they already were.

**Example** - a two minute cell with the burst thirty seconds in

```json
{"id":1,"method":"experiment.base","params":{"run_for_ms":120000,"send_at_ms":30000}}
```

**Client** `wb.experiment.base(...)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.compare`

Put two arms' means beside each other with the difference between them, absolute and as a percentage of the first.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `arm_a` | string | required, primary | the arm the difference is measured from, by the label experiment.define or experiment.vary gave it; a label with no results under it is refused |
| `arm_b` | string | required | the arm measured against it, which has to be named rather than passed bare because the bare value is already spent on arm_a; a label with no results under it is refused |

**Answers** `a`, `b`, `delta`, `note`. `delta` carries tx, rx, delivered, redundant and collisions, each with a `_pct` twin where the first arm's figure is not zero. It says nothing about whether the difference is larger than the seed spread: that judgement is the warning experiment.results carries.

**Example** - what listening before talking cost, or won

```json
{"id":1,"method":"experiment.compare","params":{"arm_a":"cad off","arm_b":"cad on"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.experiment.compare(a, b)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.define`

State a whole matrix in one call - the arms, the seeds, the senders and the burst's timing - which is how a script sets up a sweep it did not build in the panel.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `arms` | array | optional | one object per arm, carrying `label`, `repeater_version` and `companion_version`; an arm with no label takes its repeater version as one, and an absent or empty list leaves the arms alone |
| `seeds` | array | optional | the seeds each arm is repeated over, as numbers; anything that is not a number is dropped, and an absent or empty list leaves the seeds alone |
| `senders` | array | optional | the nodes that originate the burst, by name; unlike the others an empty list is obeyed and clears them, which leaves an experiment experiment.start will refuse |
| `run_for_ms` | number | optional | how long each cell runs, in simulated milliseconds; zero or less is ignored and the current length kept |
| `send_at_ms` | number | optional | the simulated instant the burst is fired, which is the same in every arm; zero or less is ignored |
| `spread_ms` | number | optional | milliseconds to stagger the senders over; zero fires them all at once, which is the sharpest test of contention and the least like anything real, and a negative value is ignored |
| `bytes` | number | optional | pad the message to at least this size, since airtime scales with payload and airtime is what collides; it is a floor rather than the width, because every cell of the matrix floods the same number of bytes whatever its label and seed are, and zero leaves that common width to the widest cell; a negative value is ignored |
| `scope` | string | optional | the region every sender originates under; empty sends unscoped, which is carried by a different set of repeaters and so measures a different network |

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`. Counts of what is now defined rather than the definition itself, except `arm_labels`, which names every arm: a count cannot tell a cross that produced the six arms wanted from one that produced six others.

**Example** - a flood from one node, ninety seconds a cell, over two seeds

```json
{"id":1,"method":"experiment.define","params":{"run_for_ms":90000,"seeds":[1,2],"send_at_ms":30000,"senders":["West Lomond"]}}
```

**Client** `wb.experiment.define(...)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.export`

Write the whole experiment to a file, its definition beside every cell's raw numbers and the per-arm summary, so a sweep outlives the session that ran it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | optional, primary | where to write it; empty writes meshbench-experiment.json in the system temporary directory, and a path that cannot be written is refused |

**Answers** `path`, `bytes`. JSON, holding the arms, seeds, senders and timings alongside `results`, which is every cell including the per-second histogram and the at-risk shares, and `summary`, which is the means. It will write an empty one before anything has run.

**Example** - keep the sweep, wherever the machine puts temporary files

```json
{"id":1,"method":"experiment.export","params":{}}
```

**Client** `wb.experiment.export(path)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.finished`

**The workbench's own callback. The socket refuses it.**

Close a sweep out on the store's goroutine: retire its progress row and say in one line how many cells ran and whether the numbers are yet a result.

**Takes** nothing.

**Answers** `runs`, `warning`. `warning` is empty where nothing is wrong with the sweep, and is the same sentence experiment.results carries otherwise.

**Client** none: the sweep runner reporting it finished

### `experiment.results`

Read the sweep back as one row per finished cell and one summary per arm, and publish the same numbers to the panels so a client and a window cannot disagree about what was measured.

**Takes** nothing.

**Answers** `runs`, `arms`, `warning`. `runs` and `arms` are both lists, and an empty `runs` is the normal answer before anything has started. `warning` is present only where the numbers do not mean what they look like: nothing run, one seed, one arm, a cell that failed, or seeds that agree so exactly that a difference between arms has nothing to be called larger than.

**Example** - read the table so far

```json
{"id":1,"method":"experiment.results","params":{}}
```

**Client** `wb.experiment.results(arm=)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.seeds`

Replace the seeds every arm is repeated over, which is the only thing that gives a difference between arms something to be called larger than.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `seeds` | array | required | the seeds, as numbers; anything that is not a number is dropped, and a list left holding none is refused rather than emptying the seeds |

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`

**Example** - four draws of each arm

```json
{"id":1,"method":"experiment.seeds","params":{"seeds":[1,2,3,4]}}
```

**Client** `wb.experiment.seeds = [...]`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.senders`

Choose which nodes originate the burst, which decides more than it looks like: with one originator every seed can return the same numbers, and then the seed bounds nothing.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `senders` | array | optional | the node names; an absent list leaves the senders alone, an empty one clears them, and entries that are not strings are dropped |

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`

**Example** - two originators, so the seeds have something to disagree about

```json
{"id":1,"method":"experiment.senders","params":{"senders":["West Lomond","Dunfermline"]}}
```

**Client** `wb.experiment.senders = [...]`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.start`

Put every arm through every seed on a worker, each cell in its own engine with node storage of its own, and answer as soon as the worker is away rather than when it is done.

**Takes** nothing.

**Answers** `running`, `runs`, `reproducible`, `not_reproducible_why`. `runs` is how many cells were queued. Poll experiment.state until `running` goes false. It refuses where a sweep is already running, where no network is loaded, where no sender has been named, and where the last sweep's cell in flight has not yet let go of the results table. `reproducible` is false where the network carries a node running in an emulator, whose firmware is stepped by the emulator's clock rather than by the run's: the cells then differ for reasons the matrix does not record, so the arms measure nothing against each other, and `not_reproducible_why` says which node, empty when there is none. The sweep still runs - a single arm on emulated firmware is a legitimate thing to watch - but a script comparing arms should stop here.

**Example** - start the matrix that is defined

```json
{"id":1,"method":"experiment.start","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.experiment.start()`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.state`

Ask the runner where it has got to without disturbing it, which is what a script polls between starting a sweep and reading it.

**Takes** nothing.

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`, `running`, `done`, `status`, `log`. Everything experiment.define answers with, plus `running`, `done` as the number of cells finished, `status` naming the cell in flight, and `log`, the last twelve lines, which is absent until something has been logged.

**Example** - poll a sweep in progress

```json
{"id":1,"method":"experiment.state","params":{}}
```

**Client** `wb.experiment.state()`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.stop`

Ask a sweep to stop and say whether it actually has, since the cell in flight finishes before the worker leaves and waiting for it here would deadlock the worker.

**Takes** nothing.

**Answers** `stopped`, `done`, `total`, `settled`. `stopped` is whether there was anything running to stop, so false is a normal answer. `settled` is the one a script wants: false means the worker is still inside a cell, and the next experiment.start will be refused until it is not.

**Example** - abandon a sweep part way through

```json
{"id":1,"method":"experiment.stop","params":{}}
```

**Client** `wb.experiment.stop()`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `experiment.vary`

Cross the arms already defined with one parameter's values, so three path hash modes against two firmware versions is six arms rather than the two the second call would leave.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `parameter` | string | required, primary | what to vary: path_hash_mode, rep_path_hash, loop_detect, cad, repeater_version, companion_version, spread_ms, or `set:` followed by any firmware setting the CLI takes; anything else is refused, with the list |
| `values` | array | required | the values, as strings, one arm per value; a list holding no strings is refused, and a value the parameter cannot take is refused with what it can |

**Answers** `arms`, `seeds`, `senders`, `runs`, `run_for_ms`, `send_at_ms`, `spread_ms`, `bytes`, `scope`, `arm_labels`. It crosses onto the arms that are there rather than replacing them, so calling it three times gives the full product. It also discards the last sweep's results, because a finished sweep's arms answered a different question from the one now being asked.

**Example** - one arm that listens before talking and one that does not

```json
{"id":1,"method":"experiment.vary","params":{"parameter":"cad","values":["off","on"]}}
```

**Client** `wb.experiment.vary(parameter, values)`

Planned, not written: no client defines `wb.experiment` yet - the whole experiment chain, which is the largest thing still only reachable by verb. Call the verb itself in the meantime.

### `sweep.run`

Push a rising offered load through one node until the network stops carrying what it is given, which is the point a delivery figure taken at one load cannot show.

**Takes** nothing.

**Answers** `arms`, `seeds`. The shape of the plan it has just started, not a result. The plan is fixed and takes no parameters: four message rates from one every two seconds down to one every 250 ms, over six seeds. Those two dozen short simulations run on a worker and the matrix arrives later through an internal callback, with progress under the job id `sweep`. The node swept is the first selected one, or the scenario's first companion where nothing is selected. It refuses where no engine has been built or no node is loaded.

**Example** - find where the selected node's mesh saturates

```json
{"id":1,"method":"sweep.run","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.sweep.run()`

Planned, not written: no client defines `wb.sweep` yet - the offered-load sweep. Call the verb itself in the meantime.

### `sweep.set`

**The workbench's own callback. The socket refuses it.**

Take the finished offered-load matrix onto the world, on the one goroutine allowed to apply it.

**Takes** nothing.

**Answers** Nothing. It carries a *state.Matrix, which is a Go value nothing outside the process can spell, so anything else is refused rather than applied as an empty matrix.

**Client** none: the sweep runner publishing its matrix

## Validation

### `validate.calibrate`

Set the excess path loss term from what the comparison measured, or from a stated figure, and rebuild every link on it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `db` | number | optional, primary | the excess loss to apply, in decibels, which wins over the measurement; absent uses the measured residual, a negative is refused because excess loss is a loss, and a db that cannot be read as a number is refused rather than falling back to the residual nobody asked for |

**Answers** `db`, `links`. Refuses when nothing has been measured and no db was given: defaulting to 0 dB there is not the absence of calibration but the most optimistic model there is. The figure applied is a total on top of nothing, not a delta, so repeated fetch-then-calibrate rounds converge. It rebuilds the engine and re-measures every link, so `links` is 0 at the moment it answers and fills in as the matrix warms.

**Example** - apply a stated excess loss rather than a measured one

```json
{"id":1,"method":"validate.calibrate","params":{"db":12}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.validate.calibrate(db=None)`

Planned, not written: no client defines `wb.validate` yet - predicted against actually heard. Call the verb itself in the meantime.

### `validate.compare`

**The workbench's own callback. The socket refuses it.**

Put what the model predicted beside what was actually heard, and turn the difference into a total excess loss worth applying.

**Takes** nothing.

**Answers** `matched`, `unmatched`, `median_db`, `iqr_db`, `suggested_excess_loss_db`. A positive `median_db` means the model predicted a stronger signal than was heard, which is the simulator being kinder than the air. `suggested_excess_loss_db` is a total and not a delta: the links these residuals were measured against already carried the current term, so it is that term plus the median. Saturated observations are counted but do not vote. It refuses when nothing matched, and says how much of that was names outside the scenario against pairs with no measured link, because the two want different fixes.

**Client** none: the observation fetch handing back what was heard

### `validate.failed`

**The workbench's own callback. The socket refuses it.**

Take the validation job off the status line and put the reason there instead, so a fetch that matched nothing says which of the several nothings it was.

**Takes** nothing.

**Answers** Answers nothing.

**Client** none: the observation fetch reporting a failure

### `validate.fetch`

Pull what a real deployment actually heard and hand it to the comparison, which is where a number for the excess loss term comes from rather than a guess.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `url` | string | optional, primary | the CoreScope base URL to ask; absent falls back to the source set with import.set_source, and is refused when there is no source either way |
| `hours` | number | optional | how far back to ask for, from a minute to a year; absent is a day, and a value outside that range or not a number is refused rather than quietly made a day |

**Answers** `fetching`, `hours`. It answers the moment the fetch starts, not with anything fetched. The comparison lands later through the internal `validate.compare`, and every way it can come to nothing - neither endpoint answering, observations carrying keys from another network, no SNR in the window - through `validate.failed` on the status line. This is the one verb here that goes to the network.

**Example** - fetch a day of real receptions to compare against

```json
{"id":1,"method":"validate.fetch","params":{"hours":24,"url":"https://corescope.example/"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.validate.fetch(url, hours=)`

Planned, not written: no client defines `wb.validate` yet - predicted against actually heard. Call the verb itself in the meantime.

### `validate.uncalibrate`

Put the excess path loss back to the default, which is a stated guess rather than a measurement, and rebuild every link on it.

**Takes** nothing.

**Answers** `db`. The snapshot stops calling itself calibrated, which is the point: the default is a reasonable figure for typical clutter and not this network's. It re-measures the whole link matrix, so the margins move over the seconds after it answers.

**Example** - drop a calibration and go back to the stated default

```json
{"id":1,"method":"validate.uncalibrate","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.validate.uncalibrate()`

Planned, not written: no client defines `wb.validate` yet - predicted against actually heard. Call the verb itself in the meantime.

## The radio model

### `environ.failed`

**The workbench's own callback. The socket refuses it.**

Close the pull's progress job and say why it came to nothing, so a failed download leaves a reason rather than a bar that stops moving.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `reason` | string | optional, primary | what went wrong, as the error itself put it; a stop by the operator never arrives here, because that is not a failure and reporting it as one teaches somebody to distrust the button they pressed |

**Client** none: the building fetch reporting a failure

### `environ.fetch`

Pull building footprints for the loaded network from a public database, cache them permanently and switch them on, which is the impatient path to testing buildings without preparing a region offline with tools/envgen first.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `source` | string | optional, primary | merged for Microsoft footprints enriched with what OpenStreetMap explicitly tags, or osm or microsoft for one of them alone; absent pulls osm, and a name that is none of the three fails inside the job rather than at the call |

**Answers** `source`, `started`. `started` is always true: the pull is minutes of somebody else's bandwidth, so it runs as a job the jobs strip can stop, and this answers before any of it. How it ended arrives in the journal, and a pull that lands switches the tiles on through rf.environment, so there is one way buildings come into force. A patch set larger than a public server may fairly be asked for, and ground with no buildings on it, both fail loudly rather than quietly returning nothing.

**Example** - get the buildings around every node, from both databases

```json
{"id":1,"method":"environ.fetch","params":"merged"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.rf.fetch_environment(source)`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `environ.fetched`

**The workbench's own callback. The socket refuses it.**

Close the pull's progress job and say what landed, which is how a download that finished silently on a worker becomes something the operator can see happened.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `note` | string | optional, primary | what was written, counted in buildings and tiles, or that the pull was already cached and nothing crossed the network |

**Client** none: the building fetch reporting success

### `environ.list`

Name every building pull already on this disk, so moving between environments is a choice from what is cached rather than another download.

**Takes** nothing.

**Answers** `dirs`, `current`. `dirs` is absolute paths, sorted, each one ready to hand straight to rf.environment, and an empty list is the honest answer where nothing has been pulled yet rather than a failure: neither a missing cache directory nor an empty one is an error here. `current` is the directory in force now, empty for bare earth.

**Example** - see which environments are already downloaded

```json
{"id":1,"method":"environ.list","params":{}}
```

**Client** `wb.rf.environments`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `radio.preset`

Put the nodes on one of the community's agreed modem settings, which decides sensitivity and airtime and therefore every number downstream of them, or list the settings there are to choose from.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `preset` | string | optional, primary | the preset's label, exactly as it is listed; a label no preset has is refused, and absent lists the labels instead of changing anything |
| `node` | string | optional | one node to move, named rather than passed bare; absent moves every node in the network |

**Answers** `preset`, `nodes`. `nodes` is how many were moved, and zero is the answer where `node` named nobody. Called with no preset at all it answers instead with `presets`, the labels available. The frequency is part of a preset, so a change rebuilds the engine and remeasures every link on a worker.

**Example** - put the whole network on the settings its neighbours use

```json
{"id":1,"method":"radio.preset","params":"EU/UK (Narrow)"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.radio.presets  /  wb.radio.apply(preset, node=)`

Planned, not written: no client defines `wb.radio` yet - the preset list and applying one. Call the verb itself in the meantime.

### `rf.environment`

Point the session at a directory of environment tiles so both RF modes price buildings into the path budget, or take it away again and go back to bare earth; asked with nothing it reports the tiles in force instead of setting any.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `dir` | string | optional, primary | the tile directory, as tools/envgen or environ.fetch wrote it; named and empty is refused unless on is false, because a switch with nothing to switch on would silently leave the model bare. No parameters at all is the question rather than the switch, and reports what is loaded |
| `on` | bool | optional | false drops the environment and returns the model to bare earth; absent or true expects a dir |

**Answers** `environment`. `environment` is the directory in force after the call, and empty means bare earth, so the reply is the same shape whether the tiles were set or asked for. Every path loss already cached was priced without buildings, so a live engine drops its link cache and the links are measured again.

**Example** - charge the paths for the buildings they cross

```json
{"id":1,"method":"rf.environment","params":{"dir":"/var/lib/meshbench/environment/fife"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.rf.environment = dir`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `rf.excess_loss`

Add a flat calibration loss to every path, which is where the clutter, body loss and multipath the bare-earth model has no term for get paid for, and read back what the model is running with.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `db` | number | optional, primary | decibels of loss on top of the modelled path, fitted from observations the study has already validated against; a negative figure would add signal and is refused, and absent changes nothing and only reports, leaving the default zero that makes every margin a best case |

**Answers** `db`, `links`. `links` is how many links the world is holding as this returns. Path loss is cached per pair for the life of an engine, so setting `db` over a loaded network rebuilds the engine and measures every pair again on a worker: the links are empty until that finishes, and this answers before it does.

**Example** - charge the 8 dB a validation run found the model was short

```json
{"id":1,"method":"rf.excess_loss","params":{"db":8}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.rf.excess_loss_db = n`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `rf.mode`

Choose which physics decides reception, and stamp the choice into the world so every snapshot, saved run and export says which of the two models produced it; asked with nothing it reports the mode in force instead of setting one.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `mode` | string | optional, primary | calculated for link budgets against demodulator floors, which is the fast model, or waveform for the full receive chain of demodulation, FEC and CRC; absent altogether reports the mode in force and changes nothing, and any other value - the empty string included - is refused rather than read as the absent case |

**Answers** `mode`. `mode` is the mode in force after the call, so the reply is the same shape whether the mode was set or asked for. A caller that needs to know which physics produced a number can ask without setting one, which used to be impossible: the only reader was the snapshot, and a socket client has not got one.

**Example** - let the receive chain decide, rather than a link budget

```json
{"id":1,"method":"rf.mode","params":"waveform"}
```

**Client** `wb.rf.mode = 'calculated'|'waveform'`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `rf.realism`

Price in the imperfections the channel otherwise leaves out - crystal error, a delayed echo, fading, receiver implementation loss and front-end clipping - because with all five at zero the model is kinder than the air and every margin it reports is a best case.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `osc_ppm` | number | optional | worst-case crystal error in parts per million, each node offset deterministically within it; absent leaves the current value alone, and zero is a pair of perfect oscillators |
| `multipath_db` | number | optional | how far below the direct ray one delayed reflection arrives, in decibels; absent leaves it alone, and zero is a single clean path with nothing to cancel against |
| `fading_hz` | number | optional | how fast that echo's phase rotates over simulated time, so a marginal link breathes; absent leaves it alone, and zero holds the interference pattern still |
| `impl_loss_db` | number | optional | the receiver's shortfall from theory, applied as extra receiver noise; absent leaves it alone, and zero credits the receiver with its datasheet floor and nothing worse |
| `saturation_dbm` | number | optional | the level above which the front end clips, harmonics and all; absent leaves it alone, and zero models a receiver that never overloads however close the transmitter is |

**Answers** `realism`. `realism` is the whole switch set after the call, including the switches this call did not name, so one knob can move without restating the rest. The effects act on the waveform paths, so a calculated run stores them and shows no change until the physics is switched.

**Example** - charge the receiver a realistic 2 dB and let the crystals disagree

```json
{"id":1,"method":"rf.realism","params":{"impl_loss_db":2,"osc_ppm":10}}
```

**Client** `wb.rf.realism(...)`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

### `rf.toggle`

Flip to whichever RF physics is not running, for a control that is one button rather than a choice of two.

**Takes** nothing.

**Answers** `mode`. `mode` is the physics now in force, not the one it left.

**Example** - swap the physics for the other one

```json
{"id":1,"method":"rf.toggle","params":{}}
```

**Client** `wb.rf.toggle()`

Planned, not written: no client defines `wb.rf` yet - the radio model: which physics, what realism, how much excess loss. Call the verb itself in the meantime.

## Provisioning, schedule and assertions

### `assert.add`

State what has to be true for a run to have passed, so a scripted run ends with a verdict rather than a table somebody has to read.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `kind` | string | required, primary | what is measured: `delivered` (or `deliveries`, or `unique_deliveries`) counts the nodes that heard anything, `sent` (or `transmissions`) counts transmissions; absent is refused, and a kind this build does not understand is accepted here and fails at the check rather than passing |
| `node` | string | optional | restrict a transmission count to one node; has to be named, and absent counts every node |
| `at_least` | number | optional | the floor the count has to reach; absent is zero, which any count clears |
| `at_most` | number | optional | the ceiling, which a transmission count uses in place of at_least whenever it is above zero |
| `max_pct` | number | optional | recorded on the assertion and carried in the fixture, and read by no kind this build checks |
| `within_ms` | number | optional | recorded on the assertion and carried in the fixture, and read by no kind this build checks |

**Answers** `assertions`. `assertions` is how many the scenario now carries, not the one just added.

**Example** - the flood has to reach at least two nodes

```json
{"id":1,"method":"assert.add","params":{"at_least":2,"kind":"delivered"}}
```

**Client** `wb.assertions.add(kind, ...)`

### `assert.check`

Measure every assertion against the run so far and report each one separately, which is what a scripted run exits on.

**Takes** nothing.

**Answers** `passed`, `total`, `results`. `results` is one row per assertion, carrying `kind`, `node`, `pass`, `got` and `want`. A scenario carrying no assertions is refused rather than answered with a pass, and an assertion whose kind this build does not understand fails with `got` reading "not measured": a green run that checked nothing is the failure this guards against.

**Example** - did this run pass

```json
{"id":1,"method":"assert.check","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.assertions.check()  ->  Report`

### `provisioning.apply`

Type the current settings into the nodes that are already running, which is the difference between changing what a future run does and changing what this one is doing.

**Takes** nothing.

**Answers** `nodes`. `nodes` counts the ones that had firmware up and were typed at; a node not running is passed over rather than refused, so a count below the size of the network is normal on a mesh that is still coming up. The commands are queued at each node's serial input, so the clock has to move before the firmware acts on them. A node whose console will not take a line ends the whole call with an error naming it, leaving the nodes after it untouched.

**Example** - push changed settings into a mesh that is up

```json
{"id":1,"method":"provisioning.apply","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.provisioning.apply()`

Planned, not written: no client defines `wb.provisioning` yet - the settings a node is provisioned with, at the network level. Call the verb itself in the meantime.

### `provisioning.get`

Read the switches every node is told at boot, which is what decides whether a mesh comes up named, positioned and in the same conversation as the rest.

**Takes** nothing.

**Answers** `set_name`, `set_position`, `set_clock`, `region_from_area`, `default_scope`, `advert_hops`, `advert_minutes`, `stagger_ms`, `flood_max_advert`, `path_hash_mode`, `comp_path_hash_mode`, `loop_detect`, `cad`, `extra`. The settings themselves, unwrapped. A zero or an empty string is not a value sent to the firmware but a firmware default left alone, and `path_hash_mode` and `comp_path_hash_mode` say the same thing with -1.

**Example** - read what the next start will send

```json
{"id":1,"method":"provisioning.get","params":{}}
```

**Client** `wb.provisioning.settings`

Planned, not written: no client defines `wb.provisioning` yet - the settings a node is provisioned with, at the network level. Call the verb itself in the meantime.

### `provisioning.set`

Change what a future run tells its nodes, a switch at a time, leaving every setting not named where it was.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `set_name` | bool | optional | send each node its scenario name; absent leaves it alone, and off leaves a node reporting as its board type |
| `set_position` | bool | optional | send each node its latitude and longitude; absent leaves it alone, and off leaves a node advertising no position |
| `set_clock` | bool | optional | set every node to the run's own epoch; absent leaves it alone, and off leaves clocks that reject traffic as replays |
| `region_from_area` | bool | optional | define a transport region named after the study area; absent leaves it alone |
| `default_scope` | bool | optional | make that region the one nodes originate under; absent leaves it alone, and off leaves them relaying but never originating |
| `advert_hops` | number | optional | how far an advert may flood; absent leaves it alone and zero says nothing to the firmware |
| `advert_minutes` | number | optional | how often a node says it is there; absent leaves it alone, zero means never, and anything outside 60 to 240 is clamped into that range because the firmware refuses the rest |
| `stagger_ms` | number | optional | milliseconds between node starts; absent leaves it alone |
| `flood_max_advert` | number | optional | how far an advert is relayed; absent leaves it alone and zero leaves the firmware's own limit |
| `path_hash_mode` | number | optional | the repeaters' path-hash mode; absent leaves it alone and negative says nothing to the firmware |
| `comp_path_hash_mode` | number | optional | the companions' own, which is a different question from the repeaters'; absent leaves it alone and negative falls back to path_hash_mode |
| `loop_detect` | string | optional | the firmware's loop-detect setting, sent only to nodes that transmit; absent or empty leaves it alone |
| `cad` | string | optional | the firmware's CAD mode, sent only to nodes that transmit; absent or empty leaves it alone |
| `extra` | string | optional | further console lines, one per line, sent after the rest and unchecked; absent or empty leaves it alone |

**Answers** `set_name`, `set_position`, `set_clock`, `region_from_area`, `default_scope`, `advert_hops`, `advert_minutes`, `stagger_ms`, `flood_max_advert`, `path_hash_mode`, `comp_path_hash_mode`, `loop_detect`, `cad`, `extra`. Every setting comes back, the same shape as `provisioning.get`, so one call both changes and reads. Nothing reaches a node that is already running: this is what the next start sends, and `provisioning.apply` is what changes a mesh that is up.

**Example** - make the mesh advertise, and let adverts cross a country

```json
{"id":1,"method":"provisioning.set","params":{"advert_minutes":60,"flood_max_advert":32}}
```

**Client** `wb.provisioning.set(...)`

Planned, not written: no client defines `wb.provisioning` yet - the settings a node is provisioned with, at the network level. Call the verb itself in the meantime.

### `run.save`

File the counters as they stand as a named run, so two configurations measured an hour apart can still be put side by side.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | optional, primary | what to file it under; empty files it as "run", and the time is appended either way so two saves of one name both survive |

**Answers** `path`. It writes into the user's cache directory, not the working directory, and takes its numbers from the renderer's snapshot rather than the world, so what is recorded is what was on screen when it was asked for.

**Example** - keep the run to compare the next one against

```json
{"id":1,"method":"run.save","params":"baseline"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.save_run(path)`

### `schedule.add`

Have a node originate traffic on its own at a stated moment of simulated time, so a run reproduces without anybody watching it for the moment to press send.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | required, primary | the node that sends; absent is refused, and so is a name no node in this network carries |
| `at_ms` | number | optional | the simulated instant of the first send; absent is zero, which is the start of the run |
| `every_ms` | number | optional | repeat interval in simulated milliseconds; absent or zero sends once |
| `command` | string | optional | what the node is told to do, in its own console's words; has to be named rather than passed bare, and absent leaves it empty |

**Answers** `sends`. `sends` is how many the schedule now holds, not the one just added.

**Example** - one flood thirty seconds into the run

```json
{"id":1,"method":"schedule.add","params":{"at_ms":30000,"node":"West Lomond"}}
```

**Client** `wb.schedule.add(node, command, at=, every=)`

### `schedule.clear`

Empty the schedule, which is what editing it amounts to: nothing removes a single send, so a schedule is rebuilt rather than amended.

**Takes** nothing.

**Answers** `cleared`

**Example** - start the schedule again

```json
{"id":1,"method":"schedule.clear","params":{}}
```

**Client** `wb.schedule.clear()`

## Machine resources

### `gpu.set`

Choose whether the link matrix is measured on the GPU or on the processor, which is a choice about how long it takes and nothing else: every kernel has a CPU twin tested against it, so a machine turning this off loses time, not answers.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional, primary | true to use a device where there is one worth opening, false to keep the work on the cores; absent changes nothing and only reports, and a value given here is remembered across launches in place of the default |

**Answers** `enabled`, `present`, `device`, `backend`, `why`, `last_warm`. `enabled` is the choice and `present` is the hardware, which are different things: a device that opens but disagrees with the CPU twin on a small problem is reported absent with `why` saying so, rather than trusted with the network. `device` and `backend` appear only where one passed. `last_warm` is what the previous measurement actually did - used, pairs, ms, and why not - because a run that quietly fell back to the cores must not read as one that did not.

**Example** - keep the measurement on the processor and the device free

```json
{"id":1,"method":"gpu.set","params":false}
```

**Client** `wb.gpu.enabled = bool`

Planned, not written: no client defines `wb.gpu` yet - whether the link matrix is measured on the GPU. Call the verb itself in the meantime.

### `gpu.state`

Read what hardware this machine has, whether it passed the check against the CPU twin, and what the last link measurement actually ran on.

**Takes** nothing.

**Answers** `enabled`, `present`, `device`, `backend`, `why`, `last_warm`. the same answer gpu.set gives, without changing anything. The machine is asked for a device exactly once per session, so calling this repeatedly costs nothing.

**Example** - ask what the last warm ran on

```json
{"id":1,"method":"gpu.state","params":{}}
```

**Client** `wb.gpu`

### `job.cancel`

Stop a long operation where whoever started it left a way to, and say so by name where it did not, rather than leaving somebody watching a bar that carries on.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `id` | string | required, primary | the job's id, as job.list reports it; an id no running job carries is refused, and so is one whose job cannot be interrupted safely - job.list's `cancellable` says which before it is pressed |

**Answers** `stopping`. The row stays on the list with "stopping" put in front of what it says, because the work ends when its context notices, which is not this instant, and a row that vanished on the press would be claiming otherwise.

**Example** - stop a terrain download

```json
{"id":1,"method":"job.cancel","params":{"id":"tiles"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.jobs[id].cancel()`

### `job.done`

**The workbench's own callback. The socket refuses it.**

Take a job off the list, because a progress bar that never goes away is a worse lie than no progress bar.

**Takes** nothing.

**Answers** Nothing, and an id no job carries is not an error: the work finishing twice is commoner than the row being missing.

**Client** none: a worker retiring its own progress row

### `job.list`

List the long operations in flight, with what each one is and how far it has got.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `all` | bool | optional, primary | include jobs that have already finished |

**Answers** `jobs`, `running`. Each row carries the job's id, what it is, `done` against `total` in whatever the job counts in, whether it has finished or failed, and `cancellable`, which is whether `job.cancel` would be refused: a terrain download can be stopped and a link measurement cannot. Finished rows stay in the list, so a caller polling at the wrong moment still learns how what it was waiting for turned out. `running` counts only what is still in flight, whatever `all` said.

**Example** - see what is running, and how the last things ended

```json
{"id":1,"method":"job.list","params":{"all":true}}
```

**Client** `wb.jobs()`

### `job.progress`

**The workbench's own callback. The socket refuses it.**

Create or move on one row of the job list, carrying forward the cancel function of the row it replaces.

**Takes** nothing.

**Answers** Nothing. It carries a state.Job, a Go value holding a closure nothing outside the process can spell, so anything else is refused.

**Client** none: a worker reporting progress; read wb.jobs instead

### `resource.fetch`

Download one runtime resource as a job that can be stopped, which is how a headless machine gets the SoftDevice or the emulator toolchain an emulated board is blocked on.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the row's name as resource.list gives it; absent is refused, and so is a name the chosen provider does not hold |
| `kind` | string | optional | which provider owns it - softdevice, toolchain, terrain, basemap or buildings - named rather than bare; absent means softdevice, so a row of another kind is not found without it |
| `version` | string | optional | the release to fetch, named rather than bare; the row's own version overrides it wherever the row carries one |

**Answers** `fetching`, `version`. It answers as soon as the download is started, not when it has arrived. Progress is the job `resource:<name>`, which `job.list` shows and `job.cancel` stops, and how it ended is said aloud rather than returned here. A resource that fills itself as the map is used is refused: there is nothing to ask for out of context.

**Example** - fetch the SoftDevice an emulated nRF52 boots

```json
{"id":1,"method":"resource.fetch","params":{"name":"s140"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.resources.fetch(kind, name, version)`

Planned, not written: no client defines `wb.resources` yet - what has been downloaded, and its licence. Call the verb itself in the meantime.

### `resource.fetched`

**The workbench's own callback. The socket refuses it.**

Take a finished download back onto the store's goroutine: say once that it is cached and where its terms are, then relist what is on disk.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the resource that arrived |
| `version` | string | optional | the release that arrived, for the sentence it says |

**Answers** `name`

**Client** none: the downloader reporting it finished

### `resource.licence`

Read the terms a cached resource arrived under, and leave them open in the snapshot for the interface to show.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the row's name; absent or unknown to the chosen provider is refused |
| `kind` | string | optional | which provider owns it, named rather than bare; absent means softdevice |
| `version` | string | optional | the release whose terms to read, named rather than bare; the row's own version overrides it where it has one |

**Answers** `name`, `version`, `text`. The whole licence text, and the same text is put into the snapshot until `resource.licence.hide` clears it. Refused where the resource is not cached: the terms are a file fetched beside it, not a string this build carries.

**Example** - read the terms the SoftDevice came under

```json
{"id":1,"method":"resource.licence","params":{"name":"s140"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.resources.licence(kind, name, version)`

Planned, not written: no client defines `wb.resources` yet - what has been downloaded, and its licence. Call the verb itself in the meantime.

### `resource.licence.hide`

Clear the terms resource.licence left open, so the interface stops showing them.

**Takes** nothing.

**Answers** `hidden`. `hidden` is always true. Calling it with nothing open is not an error, and it is left out of the session journal because it changes a window rather than the world.

**Example** - put the terms away

```json
{"id":1,"method":"resource.licence.hide","params":{}}
```

**Client** none: closing a box only a window has

### `resource.list`

Say what this machine already holds of everything downloaded at runtime, what it has cost the disk, and what could still be fetched.

**Takes** nothing.

**Answers** `rows`, `resources`. `rows` is a count, kept for the callers that were already reading it; `resources` is the rows themselves, each carrying its kind, name, version, state, size and path, why it is in the state it is in, and whether it can be fetched, carries terms, or fills itself as the map is used. A provider whose directory cannot be read contributes one row saying so rather than emptying the list.

**Example** - see what this machine already holds

```json
{"id":1,"method":"resource.list","params":{}}
```

**Client** `wb.resources`

### `resource.remove`

Delete one cached resource from the disk, through whichever provider owns it, and relist what is left.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the row's name; absent is refused, and so is a name the chosen provider does not hold |
| `kind` | string | optional | which provider owns it, named rather than bare; absent means softdevice, so removing terrain or a toolchain needs it |
| `version` | string | optional | the release to remove, named rather than bare; used only where the row carries no version of its own |

**Answers** `removed`. Nothing is confirmed here: this deletes, and the asking belongs to whatever called it. Removing 7 GB of terrain and removing a SoftDevice are the same call with a different kind.

**Example** - give the terrain cache's disk back

```json
{"id":1,"method":"resource.remove","params":{"kind":"terrain","name":"terrain tiles"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.resources.remove(kind, name, version)`

Planned, not written: no client defines `wb.resources` yet - what has been downloaded, and its licence. Call the verb itself in the meantime.

### `setup.check`

What this machine has, what it is missing, and what each one costs.

**Takes** nothing.

**Answers** `groups`, `ready`, `needed`, `undecided`, `blocked`, `missing`. Four groups in the order the problems are met - what this build is, what a node would run, what a link is measured over, and what an emulated board needs on top - each row saying its state, what it is for, what it would cost, where it lives and what to do about it, with the verb and parameters that would do it where the application can act. The five counts beside them are those same rows tallied by state, so a script can ask whether anything is wrong without walking the list. Read-only and offline: a check that started a download would be the mistake it exists to report.

**Example** - ask whether this machine is ready

```json
{"id":1,"method":"setup.check","params":{}}
```

**Client** `wb.setup.check()`

Planned, not written: no client defines `wb.setup` yet - the first-run readiness check. Call the verb itself in the meantime.

### `terrain.allow`

Allow or refuse terrain downloads on this machine, and remember it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional, primary | true to download terrain when a study needs it, false to use only what is cached |

**Answers** `on`, `warming`. `warming` is true where the answer changed the setting and a measurement was therefore started again: a matrix walked under the old answer is a matrix of different ground. Turning downloads off does that as well as turning them on, because links measured over terrain this machine may no longer fetch claim a precision the next region opened will not have. Downloads are on unless somebody turned them off; there is no third state and nothing waits to be asked.

**Example** - let this machine fetch the ground a study needs

```json
{"id":1,"method":"terrain.allow","params":true}
```

**Client** `wb.terrain.allow(on=True)`

Planned, not written: no client defines `wb.terrain` yet - the elevation cache, its consent and its prefetch. Call the verb itself in the meantime.

### `terrain.cache`

Say how much memory decoded terrain may occupy, in the unit people think in, and read back where the tiles are kept and whether this machine is allowed to fetch more.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `gb` | number | optional, primary | the ceiling in gigabytes, four thousand-odd tiles to the gigabyte; anything under 0.25 is ignored rather than refused, and absent only reports |

**Answers** `gb`, `dir`, `downloads`. `dir` is where the tiles live on disk, which is permanent: nothing here expires a cached tile. `downloads` is whether terrain may be fetched at all, answered here because this is the verb the interface asks at startup and the switch has to draw its own position on the first frame.

**Example** - ask where the terrain is and how much is held

```json
{"id":1,"method":"terrain.cache","params":{}}
```

**Client** `wb.terrain.cache_gb = n`

Planned, not written: no client defines `wb.terrain` yet - the elevation cache, its consent and its prefetch. Call the verb itself in the meantime.

### `terrain.cache_dir`

Move the terrain cache to another disk, files and all, so a permanent cache that has outgrown where it started does not have to be downloaded again somewhere else.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | optional, primary | where the cache should live, created if it is not there already; absent only reports where it lives now, while a path that cannot be written into, one that contains or sits inside the current cache, or a second move while one is still running, is refused rather than queued |

**Answers** `moving`, `to`. the move runs on a worker with a progress job, so this answers `moving` true before a byte has gone. Asked with no path it answers `dir` instead. A move that fails leaves the cache where it was and says so in the journal.

**Example** - put the tiles on the disk with room for them

```json
{"id":1,"method":"terrain.cache_dir","params":"/srv/terrain"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.terrain.cache_dir = path`

Planned, not written: no client defines `wb.terrain` yet - the elevation cache, its consent and its prefetch. Call the verb itself in the meantime.

### `terrain.cache_moved`

**The workbench's own callback. The socket refuses it.**

Point the tile store and the settings at the directory a finished move has filled, and say whether the next launch will look there too or download it all again.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `dir` | string | required | the directory the files were moved to; the callback is refused without it |
| `files` | number | optional | how many files moved, for the message; absent counts as none |

**Answers** `dir`

**Client** none: the cache mover reporting it finished

### `terrain.ground`

Report what elevation data the current network actually has under it, and whether having none of it was chosen.

**Takes** nothing.

**Answers** `state`, `chosen`, `note`, `tiles_sampled`, `tiles_cached`. `state` is `terrain` where every sampled tile under the network is cached, `partial` where some are, and `bare-earth` where none are. Bare earth is not a missing decoration: the propagation model prices a profile with no elevation as free space, which is the most optimistic answer it has and more optimistic than the best case the rest of the model is documented as. `chosen` says somebody answered the terrain question, either way, so an offline run the operator asked for can be told apart from a fetch that never happened because nobody was asked. `note` is the sentence a study carries in its own result, empty only when the ground is all here. The tile counts are a bounded sample of the study's box rather than a census, so they are there to be compared with each other and not quoted as a size.

**Example** - check what the studies here are standing on before believing one

```json
{"id":1,"method":"terrain.ground","params":{}}
```

**Client** `wb.terrain.ground()`

Planned, not written: no client defines `wb.terrain` yet - the elevation cache, its consent and its prefetch. Call the verb itself in the meantime.

### `terrain.prefetch`

Download the ground under the loaded network before anything needs it, so the minutes of network time a first measurement would spend invisibly are a visible, priced, stoppable job instead.

**Takes** nothing.

**Answers** `tiles`, `to_fetch`, `bytes_rough`. `tiles` is the area's whole tile count and `to_fetch` how many of them are missing, so `to_fetch` zero means the ground is already cached and nothing was started. Otherwise the download runs as a cancellable job and this returns before it, with `bytes_rough` an average tile size times a count rather than a measurement. Refused where no network is loaded, where the machine has no tile store, and where terrain downloads are switched off, which terrain.allow is what turns on.

**Example** - fetch the ground this study stands on

```json
{"id":1,"method":"terrain.prefetch","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.terrain.prefetch()`

Planned, not written: no client defines `wb.terrain` yet - the elevation cache, its consent and its prefetch. Call the verb itself in the meantime.

### `terrain.shade`

Hillshade the relief under one view, which is a tile fetch and a pass over every cell in it, so it runs as a job that says out loud that it is happening.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `view` | array | required, primary | the borders to shade, as the four numbers south, north, west, east; anything that is not four numbers, and four that are all zero, is refused |

**Answers** `shading`. `shading: true` means the job started, not that anything has been drawn. The shading lands later through the internal `terrain.shade_set`, or `terrain.shade_failed` where the view has no elevation to shade.

**Example** - shade the relief across Fife

```json
{"id":1,"method":"terrain.shade","params":[56,56.5,-3.6,-3]}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.shade()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `terrain.shade_failed`

**The workbench's own callback. The socket refuses it.**

Drop the hillshade and say the view has no elevation cached, so a layer that switched on and drew nothing carries its reason.

**Takes** nothing.

**Answers** Answers nothing.

**Client** none: the hillshade worker reporting a failure

### `terrain.shade_set`

**The workbench's own callback. The socket refuses it.**

Hold the finished hillshade, and warn where most of the view was blank ground rather than flat ground.

**Takes** nothing.

**Answers** Answers nothing: the shading goes into the snapshot the renderer reads.

**Client** none: the hillshade worker publishing its raster

### `update.allow`

Allow or refuse update checks on this machine, and remember it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `on` | bool | optional, primary | true to ask the release page once a day whether a newer release exists, false never to ask |

**Answers** `on`, `asked`, `checking`. `asked` is whether this machine had already answered before the call, never asked being the third state a fresh install is in and the one that spends nothing. Off is the default and stays the default: nothing in the simulation depends on knowing whether a release exists, so a machine with no network, or an operator who does not want the question, loses nothing by never answering it. `checking` is true where the grant started a check straight away, which it does because nothing is watching for permission to arrive and a switch that promises something and does it tomorrow is a switch nobody believes. Allowing a check never downloads anything.

**Example** - let this machine find out when a newer release is published

```json
{"id":1,"method":"update.allow","params":true}
```

**Client** `wb.update.allow(on=True)`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

### `update.check`

Ask the release feed whether a newer release exists.

**Takes** nothing.

**Answers** `checking`, `build`. Starts the check and returns immediately: asking is a network call, and one made on the store's own goroutine would stop the simulation for as long as a socket takes to time out. It announces itself in the jobs strip and the answer lands in `update.status`, which is what a script polls. The routine question is answered by the redirect the releases page already serves - a 302 naming the newest tag, no API call - because GitHub allows an unauthenticated caller 60 requests an hour per address, and an address is a household, an office or an ISP doing carrier-grade NAT; the API is asked once a newer release is found, for the assets and their sizes. A check that could not reach either is an error with a reason, never a report that this build is current: a rate limit, a captive portal and an up-to-date build are three answers and only one of them is about this build. A working copy is refused rather than checked, and says so: a build with no release stamped in it is not behind anything, it is unreleased. Calling this by hand is its own consent for that one check - the preference `update.allow` sets governs the check nobody asked for, the one on a timer.

**Example** - find out now whether a newer release has been published

```json
{"id":1,"method":"update.check","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.update.check()`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

### `update.checked`

**The workbench's own callback. The socket refuses it.**

Take a finished check back onto the store's goroutine: publish what the release feed said and say it once.

**Takes** nothing.

**Answers** Internal: the check reporting back to itself from the worker it runs on. The result it carries is the whole of what `update.status` then answers.

**Client** none: the release check reporting back from its own worker

### `update.download`

Download the release the last check found, beside this build rather than over it.

**Takes** nothing.

**Answers** `downloading`, `bytes`, `release`, `into`. Refuses unless a check has already found a release this machine could take, and says which of the four reasons applies - nothing has asked yet, the last check could not reach the feed, there is a newer release but nothing published for this platform or this bundle, or this build is already the newest. The fetch runs in the background with its size announced in the jobs strip before it is spent, and it is verified before it is offered: the release publishes SHA256SUMS beside its artefacts, and a file whose digest does not match is deleted rather than kept. That digest comes from the same release as the file, so what it proves is that the download arrived intact; what says the release is ours is the connection to github.com, which is why an asset served from anywhere else is refused outright. Nothing is replaced: the download lands in the update cache and the answer says how to swap it by hand, because a run holds unsaved state and replacing a binary underneath one is a way to lose somebody's work.

**Example** - fetch the release the last check found

```json
{"id":1,"method":"update.download","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.update.download()`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

### `update.notes`

Open the release page for what the last check found.

**Takes** nothing.

**Answers** `opened`. The notes are linked rather than embedded: they are prose, and prose outgrows any panel it is put in. Refuses when no check has named a release page yet.

**Example** - read what changed in the release being offered

```json
{"id":1,"method":"update.notes","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.update.notes()`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

### `update.reveal`

Open the folder a downloaded release landed in.

**Takes** nothing.

**Answers** `opened`. Refuses when nothing has been downloaded, because there is then no folder to open. Hands the path to whatever this desktop uses - xdg-open, open, or the Windows file protocol handler - and says so when the machine has none rather than failing silently.

**Example** - look at the release that was just downloaded

```json
{"id":1,"method":"update.reveal","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.update.reveal()`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

### `update.staged`

**The workbench's own callback. The socket refuses it.**

Take a finished download back onto the store's goroutine: record where it landed and say what to do with it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `path` | string | required, primary | where the verified download landed |

**Answers** `staged`. Internal: the download reporting back from the worker it runs on. What it says is the whole instruction for this platform's bundle, including that a client pinned to the old release will be refused by the new workbench.

**Client** none: the update download reporting back from its own worker

### `update.status`

What the last update check found, without asking anything.

**Takes** nothing.

**Answers** `build`, `latest`, `tag`, `newer`, `available`, `notes`, `published`, `checked`, `asset`, `bytes`, `artefact`, `why`, `staged`, `feed`, `error`, `allowed`, `asked`. Read-only and offline: every field is empty until something has asked, because nothing here is filled at startup. `newer` is whether a higher release exists at all and `available` is whether one exists that this machine could actually take - they differ where a release is published for platforms this is not one of, or where the package manager owns this copy, and `why` is the reason in words. `artefact` names which bundle this build came out of, because what can honestly be done with a download afterwards differs per bundle. `staged` is where a verified download landed, always beside this build and never on top of it. `error` is why the last check could not answer, which is a different thing from there being nothing newer. `feed` is only set when somebody pointed the check at something other than the published release feed.

**Example** - ask whether this build is the newest one, without touching the network

```json
{"id":1,"method":"update.status","params":{}}
```

**Client** `wb.update.status()`

Planned, not written: no client defines `wb.update` yet - whether a newer release exists and getting it onto the disk. Call the verb itself in the meantime.

## The window

### `layout.reset`

**Refuses when no window is attached.**

Put the view on screen back to the arrangement it is declared with, which is the way back from a layout somebody has docked into an unusable shape.

**Takes** nothing.

**Answers** `reset`. The view on screen only: the other views keep whatever was done to them. Popped-out windows are not pulled back in. Refuses when no interface is attached.

**Example** - start this view's layout again

```json
{"id":1,"method":"layout.reset","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.layouts.reset()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.basemap`

Choose which map is drawn under the simulation and remember the choice, or read the one in force.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `id` | string | optional, primary | the basemap: carto-dark, carto-light or esri-topo. An absent or empty id reads the current choice and changes nothing; the session does not draw the map, so an id it does not recognise is stored rather than refused |

**Answers** `id`. Empty means nobody has chosen and the map's own default stands. The choice is written to the settings file, so it survives a restart; where that write fails the session still uses it and says so, rather than promising a next launch it cannot keep.

**Example** - put topography under the nodes for a siting review

```json
{"id":1,"method":"map.basemap","params":{"id":"esri-topo"}}
```

**Client** `wb.ui.map.basemap = id`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.centre`

**Refuses when no window is attached.**

Point the map at a node, or at a latitude and longitude.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `node` | string | optional, primary | centre on this node instead of giving coordinates |
| `lat` | number | optional | degrees north |
| `lon` | number | optional | degrees east |
| `zoom` | number | optional | zoom level; unchanged when absent |

**Answers** `lat`, `lon`, `zoom`. The position the camera was sent to, which for a node is that node's own, and `zoom` is zero where none was asked for rather than the zoom the map is on. The camera moves on the next frame, so the answer is what was asked for, not what is drawn yet. Refuses a node it cannot find, refuses a call that leaves both coordinates at zero, and refuses when no interface is attached.

**Example** - frame one repeater before a screenshot

```json
{"id":1,"method":"map.centre","params":{"node":"West Lomond","zoom":12}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.centre(node=|lat=, lon=, zoom=)`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.filter`

**Refuses when no window is attached.**

Dim everything on the map that does not match some text, which is how one network's worth of nodes is read as a handful.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `query` | string | optional, primary | the text to match; empty or absent clears the filter and everything is drawn again |

**Answers** `query`. The filter dims what does not match rather than removing it, so nothing is hidden and the count of nodes does not change. Refuses when no interface is attached.

**Example** - pick the repeaters out of a busy map

```json
{"id":1,"method":"map.filter","params":{"query":"repeater"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.filter = query`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.fit`

**Refuses when no window is attached.**

Zoom the map so every node is on it.

**Takes** nothing.

**Answers** `nodes`. `nodes` is how many the camera was framed around, so zero means an empty network and a camera that has not moved. Refuses when no interface is attached.

**Example** - get the whole network back on screen

```json
{"id":1,"method":"map.fit","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.fit()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.layer`

**Refuses when no window is attached.**

Turn one of the map's layers on or off by the name the map shows, so coverage, terrain and the antenna pattern can be reached by a script, a capture or a test rather than only by clicking.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the layer: basemap, boundaries, links, nodes, labels, traffic, coverage, terrain, regions, antenna or measure. An unknown or missing name is refused with the list |
| `on` | bool | optional | false to stop drawing it; absent means on |

**Answers** `layers`. `layers` is every layer against whether it is drawn, the one just changed among them, so nothing has to ask again. Turning one on is a request for work as well as a redraw: coverage and terrain are computed when they are first drawn. Refuses when no interface is attached.

**Example** - put the coverage raster under the nodes

```json
{"id":1,"method":"map.layer","params":{"name":"coverage","on":true}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.layers[name] = bool`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.layers`

**Refuses when no window is attached.**

Read every layer the map knows and whether it is being drawn, which is the list `map.layer` takes its names from.

**Takes** nothing.

**Answers** `layers`. One object of layer name against true or false, not a list of the ones that are on. Refuses when no interface is attached.

**Example** - check whether terrain is being drawn

```json
{"id":1,"method":"map.layers","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.layers`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `map.zoom`

**Refuses when no window is attached.**

Zoom the map in or out from wherever it is now, for a caller that wants a step rather than a stated scale.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `factor` | number | optional, primary | what to multiply the current scale by, so above one is closer and below one is further out; anything not a positive number leaves it at two |

**Answers** `factor`. The factor applied, not the zoom level reached: this multiplies whatever the map is on, and nothing here knows what that was. A caller that needs a known scale gives `map.centre` a zoom instead. Refuses when no interface is attached.

**Example** - pull back to twice the ground

```json
{"id":1,"method":"map.zoom","params":{"factor":0.5}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.zoom(factor)`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `panel.close`

**Refuses when no window is attached.**

Take a panel out of the layout, giving the room to what is left.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel to close; an unknown or missing name is refused with the list of the ones there are |

**Answers** `panel`. Whether the panel was open is not asked, only whether it exists: closing one that is not in the layout answers the same way and does nothing, which is what the caller wanted anyway. Refuses when no interface is attached.

**Example** - clear the rail before a screenshot

```json
{"id":1,"method":"panel.close","params":{"name":"Inspector"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.panel(name).close()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `panel.dock`

**Refuses when no window is attached.**

Bring a popped-out panel back into the main window, which is the other half of panel.pop_out and the only way back for a window a compositor has hidden.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel to dock; an unknown or missing name is refused with the list of the ones there are |

**Answers** `panel`, `where`. `where` is always "layout". A panel that was never popped out is docked where it belongs rather than refused. Refuses when no interface is attached.

**Example** - put the map back in the main window

```json
{"id":1,"method":"panel.dock","params":{"name":"Map"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.panel(name).dock()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `panel.open`

**Refuses when no window is attached.**

Put a panel in the layout, switching to the view it belongs to on the way, which is how the panels no view starts with are reached at all.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel, spelled as panels.list spells it; an unknown or missing name is refused with the list of the ones there are |

**Answers** `panel`. It answers once the panel has been asked for, not once it is drawn: the layout is the frame loop's, and the change lands on the next frame. A panel already in the layout is left where it is. Refuses when no interface is attached.

**Example** - bring up the waterfall from a script

```json
{"id":1,"method":"panel.open","params":"Waterfall"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.panel(name).open()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `panel.pop_out`

**Refuses when no window is attached.**

Send a panel out into a window of its own, so a second monitor can hold what the layout has no room for.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel to pop out; an unknown or missing name is refused with the list of the ones there are |

**Answers** `panel`, `where`. `where` is always "window", the verb having only one destination: it is there so an answer read on its own says what happened. Refuses when no interface is attached.

**Example** - give the map a monitor to itself

```json
{"id":1,"method":"panel.pop_out","params":{"name":"Map"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.panel(name).pop_out()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `panels.list`

**Refuses when no window is attached.**

Name every panel the interface has registered.

**Takes** nothing.

**Answers** `panels`, `count`. Every panel that exists, sorted, not the ones on screen: a panel nothing has opened is still named here, which is what makes this the list to choose a `panel.open` from. Refuses when no interface is attached.

**Example** - find out what there is to open

```json
{"id":1,"method":"panels.list","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.panels`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `tool.set`

**Refuses when no window is attached.**

Choose what a click on the map does, so a script can place or measure without a hand on the mouse.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | select, move, place, link or measure; anything else is refused with that same list |

**Answers** `tool`

**Example** - make the next two clicks a distance

```json
{"id":1,"method":"tool.set","params":"measure"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.map.tool = name`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `view.delete`

**Refuses when no window is attached.**

Forget a saved arrangement, deleting the file behind it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the saved view to remove; one that is not there is refused |

**Answers** `deleted`. The layout on screen is not touched: this removes the copy on disk, and nothing is asked first. Refuses when no interface is attached.

**Example** - drop a layout that is no longer used

```json
{"id":1,"method":"view.delete","params":{"name":"coverage-review"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.layouts.delete(name)`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `view.list`

**Refuses when no window is attached.**

Name the saved arrangements there are to load, which is the only way to find out what a machine has kept.

**Takes** nothing.

**Answers** `views`. The names alone, sorted, without the .json the files carry. Null where nothing has been saved, and null too where the platform cannot say where config lives, which reads the same from here. Refuses when no interface is attached.

**Example** - see which layouts this machine has kept

```json
{"id":1,"method":"view.list","params":{}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.layouts`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `view.load`

**Refuses when no window is attached.**

Put a saved arrangement back, windows and all.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the saved view, as view.list names it; one that is not there is refused |

**Answers** `loaded`. It answers once the arrangement is asked for, the layouts and the view landing on the next frame and the windows opening after that. A file written before docking existed carries no layouts and loads as the declared presets rather than failing. Refuses when no interface is attached.

**Example** - go back to the layout a study is read in

```json
{"id":1,"method":"view.load","params":"coverage-review"}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.layouts.load(name)`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `view.save`

**Refuses when no window is attached.**

Keep the arrangement on screen under a name, so a layout built for one kind of work survives the next launch and the next machine.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | what to call it; an empty or missing name is refused, and a name already used is overwritten without asking |

**Answers** `saved`. It saves every view's arrangement, not only the one on screen, along with which view is showing and which panels are in windows of their own, to a file under the user's config directory. Refuses when no interface is attached.

**Example** - keep the layout a coverage study is read in

```json
{"id":1,"method":"view.save","params":{"name":"coverage-review"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.layouts.save(name)`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `window.close`

**Refuses when no window is attached.**

Close a panel's own window, which returns the panel to the main layout rather than losing it.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel whose window is closed; a panel that is not in a window of its own is refused |

**Answers** `closed`. Refuses when no interface is attached, and refuses where the interface has no window manager behind it.

**Example** - close the window a capture no longer needs

```json
{"id":1,"method":"window.close","params":{"name":"Waterfall"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.window(name).close()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `window.open`

**Refuses when no window is attached.**

Open a panel in a window of its own, the same act as panel.pop_out under the name a caller thinking in windows reaches for.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `name` | string | required, primary | the panel to give a window; an unknown or missing name is refused with the list of the ones there are |

**Answers** `window`. The window is asked for here and opened by the frame loop, so the answer says it was accepted rather than that a window is up. Refuses when no interface is attached.

**Example** - watch the waterfall beside the map

```json
{"id":1,"method":"window.open","params":{"name":"Waterfall"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.window(name).open()`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.

### `workspace.set`

**Refuses when no window is attached.**

Show one of the workbench's top-level views.

**Takes**

| parameter | type | | what |
|---|---|---|---|
| `view` | string | required, primary | the view's name, as panels.list and the view bar spell it |

**Answers** `view`. Refuses when no interface is attached, and refuses a name that is not a view with the list of the ones that are.

**Example** - go to the view that asks why one packet failed

```json
{"id":1,"method":"workspace.set","params":{"view":"Debug"}}
```

Not made by the test suite: this call needs more than the two-node headless session the runnable examples go to.

**Client** `wb.ui.view = name`

Planned, not written: no client defines `wb.ui` yet - the window: panels, views, layouts and the map camera. Call the verb itself in the meantime.
