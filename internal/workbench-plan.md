# Workbench plan — HopReach parity on real firmware

The organising idea, stated once: HopReach *models* MeshCore's behaviour with a
faithful port of its formulas. This workbench does not model it — every node is
a real MeshCore build, so every HopReach feature lands as "the same question,
answered by the firmware itself". Settings are not simulator state we happen to
display; they are `set` commands delivered to a real CLI, persisted in the
node's own preference file, and read back with `get`. If the firmware would
reject it on a hilltop, it gets rejected here.

## Shape of the UI

Today everything is one window with one shared radio config and no menus. The
target is a menu bar, floating windows, and per-node state:

```
File     Scenario  Simulation  Planning   Windows    Help
 ├ open   ├ add node ├ run/pause ├ line of sight ├ Nodes
 ├ save   ├ import…  ├ step      ├ connect two   ├ Radio settings
 ├ export ├ radio…   ├ reset     ├ cover an area ├ Fleet commands
 └ quit   └ firmware…├ send…     ├ coverage      ├ Timeline
                     └ stress…   └ adjust site   ├ Scoreboard
                                                 ├ Console
                                                 └ Replay
```

- The map stays the main view. Everything else becomes a **Windows**-menu
  floating window (imgui windows, dockable later), so panels can be opened,
  moved, and closed instead of everything being always-on. The bottom tab bar
  goes away.
- The right inspector stays, but shrinks to what is per-node: position, height,
  antenna, firmware, radio, and a "console" button.

## Work items, in order

### 1. Radio settings + preset table (the HopReach table)
- `internal/scenario/presets.go`: the 20-entry preset table (Australia … EU/UK
  Narrow … USA/Canada), same source as HopReach's — the letsmesh forum list —
  baked in so it works offline. `UK/EU (Narrow) 869.618 62.5 SF8 CR4/8` etc.
- A **Radio settings** window: preset dropdown + the full table view with
  label / freq / bw / SF / CR columns; apply to scenario default, to selection,
  or to all nodes.
- Per-node radio overrides in `scenario.Node` (already has `Radio`), surfaced in
  the inspector. The engine currently assumes one shared PHY config —
  `engine.Config` grows per-node params, and the channel refuses to couple nodes
  on different frequencies (that *is* the physics: they simply never decode).
- Applying a preset to firmware-running nodes issues the firmware's own
  `set freq/bw/sf/cr` over the console bridge, then reads back with `get` to
  confirm — the preference file is the truth, not our struct.

### 2. Fleet commands (mass CLI)
- A **Fleet commands** window: target picker (all / repeaters / selection /
  filter match), a command box with history, and a result table
  (node, command, reply, ok/error) — every reply is the firmware's own text.
- Common operations as buttons that emit real CLI lines, not private APIs:
  set region prefix, `set flood.max N`, `set flood.max.unscoped`, `set
  allow.read.only on|off`, `set repeat on|off`, advert interval, TX power.
  Each button shows the exact CLI line it will send before sending.
- Sequencing: commands go out one tick apart per node so replies attribute
  cleanly; a progress row until all replies are in.
- This is also HopReach's "per-repeater action list with copy-pasteable CLI
  lines" — the search tools below emit into this window.

### 3. Menus and windows refactor
- Menu bar as above; each panel becomes `imgui` window with open/closed state
  remembered in a small json settings file next to the tile cache.
- Keyboard: space run/pause, `.` step, F fit, 1-5 tools.

### 4. Planning tools on the map (code exists, needs UI)
- **Line of sight chain**: click points, hops drawn green/orange/red by margin
  with dB — `internal/pathview` already computes this for one link; generalise
  to a chain.
- **Connect two repeaters**: `planning.Bridge` is written and tested — wire to
  a tool: pick A, pick B, get up to 3 routes drawn, accept one to place the
  proposed sites as real (firmware-runnable) nodes.
- **Cover an area**: `planning.CoverArea` likewise — draw polygon, N sites,
  before/after %, placements in build order.
- **Adjust-a-site / companion pin**: already have node dragging and companion
  placement; add "who hears a handheld here" (one-shot reverse coverage).
- **Coverage raster**: `internal/coverage` exists headless; render to a texture
  overlaid on the map with opacity slider, per selected node or whole network.

### 5. Simulation workflows (HopReach sim parity)
- **Scheduled sends**: a send list (node, time, payload, repeat) instead of the
  single "send from X" button; injected via the companion path or a real
  companion node's CLI.
- **Replay scrub-bar animation**: timeline already records everything; add
  play-through-time on the map — expanding rings for transmissions, coloured
  arrows for receptions, scrub bar shared with the event table.
- **Stress test**: ramp scheduled load until delivery ratio knees; report the
  knee. Headless command + a window.
- **Settings search**: grid-search over preset/flood parameters ranked by
  delivery ratio, ending in a Fleet-commands action list. Long-running; runs on
  the engine headless with progress.
- **Replay a real packet**: `internal/replay` + CoreScope packet fetch;
  observed hops vs simulated hops, dashed-amber/blue difference view.

### 6. Data in/out
- CoreScope: pagination fixed (was 50-node truncation). Add reach data
  (`/api/nodes/:key/reach`) as a link-decider option: terrain / observed /
  blend, like HopReach.
- Export/import scenario json (exists) + KML export of plans; share = save file.

## HopReach GUI patterns to copy directly

Named, because "take hints from HopReach" deserves specifics:

- **The "Repeaters & settings" modal** — one editable table over every node:
  radio preset, tx power, txdelay/rxdelay factors, flood.max, regions, loop
  detect, node type — with a bulk-apply row at the top that fills the whole
  column, and per-run result columns (duty, received) appearing after a run.
  This becomes the workbench's **Nodes & settings** window; on firmware nodes
  each edit is a real `set` command, and the result columns come from the
  engine's ledger.
- **Staged edits** — the table stages changes and commits on Apply; closing
  without applying discards. Same here.
- **The packet inspector** — click one sent message, see exactly its own
  propagation drawn on the map: hops green where clean, red where collided,
  every repeater it reached marked. Answers "did this message get through and
  to whom" without reading a log.
- **Per-repeater detail** — click a repeater, get its own view: what it heard,
  what it relayed, its duty, its neighbours. Here that becomes a **per-node
  window** (imgui floating window per node, as many open as you like): live
  console on one tab, stats/graph on another, settings on a third. Open from
  the map (double-click), the node table, or the Windows menu.
- **Long-list row cap with "Show all N"** (their LONG_LIST_ROW_CAP=200) —
  already adopted in the node list; apply to the timeline too.
- **Colour discipline** — proposed vs real never share a palette (blue-purple
  vs orange-green); collided red, clean green, half-duplex its own colour.
  Already partly true in the timeline; keep it everywhere.

## Interaction basics owed before anything fancy

- **Place once, then back to select.** Every placement tool returns to the
  select tool after one click; accidental node scatter was the current
  behaviour. Holding shift keeps the tool armed for laying a chain.
- **Delete.** Right-click → delete, and the Delete key on the selection. There
  is currently no way to remove a node at all.
- **Right-click everywhere.** The reason to be a desktop app and not a web
  page: context menus. Right-click a node → open its window, console, start
  link from here, coverage from here, delete. Right-click empty map → place
  each node type at that spot, fit view. Right-click a timeline row →
  inspect that packet.
- **USB companions.** `internal/companion` already carries the PTY and TCP
  transports; the workbench never exposes them. A companion node running the
  real `companion_radio` build gets a "connect a phone" action: open a PTY
  (or TCP port) speaking the companion frame protocol into the firmware's
  serial interface, so a real MeshCore phone app or meshcore-cli attaches to a
  simulated node exactly as it would to a USB device. The path exists —
  companion frames are the firmware's own serial protocol — it needs the
  bridge's console channel split from the companion frame channel.

## Rasters

`internal/coverage` computes per-station rasters and `Combine` merges them; the
CLI writes PNGs — but the workbench never draws them. Coverage becomes a map
overlay: compute for the selected node (or all repeaters) off-thread with a
progress row, render to a texture, draw under the nodes with an opacity slider,
HopReach-style orange-to-green for real sites and blue-to-purple for proposed
ones. Cache keyed on (node, height, radio, extent) so a pan is not a recompute.

## Requested additions (2026-08-10)

### Companions over IP
`internal/companion` has TCP and PTY transports; nothing exposes them. Each
companion node gets a **configurable port** (ten companions means ten ports, so
the port is per-node state shown in its window and the nodes table, not one
global setting). "Connect" starts the listener wired into that node's real
`companion_radio` serial frame interface; a phone app or meshcore-cli then
attaches to the simulated node exactly as to a USB device. Status shows
listening / attached, per node.

### Packet dissection, CoreScope-style
A packet inspector that takes any sent/received frame and dissects it the way
CoreScope does: header bits (route type, payload type, version), path hashes
with names resolved where known, payload broken down per type, raw hex
alongside. Reached from the timeline (click a row), from a node window's
activity tab, and from the SDR view later. The dissection logic must be shared
with the Lua dissector's field set so the two never disagree.

### Repeater window, HopReach layout
The per-node window's stats tab adopts HopReach's per-repeater panel:
**received / relayed** counts with the unique-vs-redundant split, duty cycle,
and a **neighbours list** — who this node actually hears and is heard by, with
last-heard SNR each way — plus the links drawn on the map when the window is
focused. The neighbour data comes from the ledger, not from a model.

### Wireshark path, finished
`internal/capture` writes valid pcapng and `tools/dissector/meshcoresim.lua`
exists — but the workbench never writes a capture. Needed: continuous capture
to a rolling pcapng per run (toggle in the Simulation menu), an "open in
Wireshark" action, and the dissector extended with **filterable fields** —
route type, payload type, node names, SNR, collision outcome — so Wireshark's
own filter bar becomes the deep-analysis surface. The pseudo-header must carry
the engine's verdict (delivered / collided / below-floor) so filters can reach
what only the simulator knows.

## GPU

Measured before deciding (elite, 12 cores, 400 nodes, 79,800 pairs):

| terrain | link matrix warm |
|---|---|
| flat | 0.16 s |
| real DEM, all pairs | 5.2 s |

The link matrix does not justify a GPU port: it warms in the background with a
progress figure, off the critical path, and the freeze it used to cause was the
lazy fill on the frame thread — an architecture bug, not a compute shortage.

Where the GPU *is* justified: **coverage rasters at scale**. A combined raster
over hundreds of stations is millions of cells, each a terrain profile — CPU
minutes of user-facing wait, embarrassingly parallel, same kernel everywhere.
`internal/gpu` already holds the pattern to follow (dechirp WGSL + CPU twin +
equivalence test, per the project rule that every GPU kernel has a tested CPU
double). That is the next GPU kernel, when coverage-at-scale lands.

## Order of attack

1. Presets + **Nodes & settings** table window (the HopReach modal, on real CLI)
2. **Fleet commands** window (mass CLI: regions, flood.max, deny, tx…)
3. Menu bar + Windows registry + **per-node windows** (console/stats/settings)
4. **Coverage rasters** drawn on the map
5. Planning UI (LOS chain, connect-two, cover-area — engines already written)
6. Sim workflows (scheduled sends, animated replay, stress, settings search)
7. Reach-data link decider + replay-a-real-packet

## Explicitly not doing

- HopReach's calibrated-positions pipeline and nightly rasters (server-side,
  different tool).
- Phone layout. This is a desktop workbench.
- Fetching the preset list at runtime.
