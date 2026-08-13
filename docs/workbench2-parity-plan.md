# Workbench 2: the plan to parity

Every item on this plan has landed and been verified in the running
application: provisioning at attach with the old workbench's startup knobs,
stepping after commands (paused consoles answer), fleet reply collection (all
309 firmware nodes on the national fixture answer "ver"), tool gating, the
layers (the hillshade now draws relief visible on the dark basemap), and the
polish items - the fleet's kind filter is a dropdown, the waterfall speaks on
every outcome, Import lives in one menu.

Written after reading workbench 1's source against workbench 2's, with every
fault Alex has reported. The one-line diagnosis, which is also the reason this
list exists: **workbench 1 is full of load-bearing behaviour that never made it
into a spec — provisioning at attach, stepping after commands, reply
collection, tool gating — and workbench 2 reimplemented the panels without the
behaviour.** The fix is to port the behaviour, journey by journey, and prove
each one in the running application before calling it done.

Rules for the whole plan:

- Every item is verified in the running app with a capture before it is
  ticked. A passing test alone does not close an item — that standard is how
  the silent-mesh bug survived a 165-control audit.
- One item, one commit. The artifact page updates as each lands.
- Workbench 1's source is the reference. Where its behaviour and its looks
  disagree with workbench 2, the behaviour wins; looks can differ.

## P0.1 — The mesh must talk when you press play

**Symptom:** 311 nodes, 242 s, zero packets, nothing heard. Workbench 1 fine.

**Root cause, found:** wb1's `attachFirmware` runs `applyStartupConfig()` the
moment firmware is up — name, clock, position, **regions**, flood caps — then
steps the engine so the nodes read it. A repeater with no regions relays
nothing and reports no error. wb2 provisioned only inside experiment arms;
plain play brought up MeshCore everywhere and told it nothing.

**State:** provisioning-on-start is written and landed. **Not yet proven** —
my verification script pressed play twice (the second press pauses), so the
trace it produced is worthless, and it also showed the run playing before the
second press, which the two-press design should not allow. Both need running
down.

Steps:
1. Re-verify end to end with a single play: mesh up → provisioned → run
   started → events climbing, heard counts non-zero, trails on the map,
   nobody typing anything. Capture it.
2. Diff wb1's `startupCommands` against wb2's `ProvisioningFor`: wb1 also
   sends `set path.hash.mode`, `set loop.detect`, `set cad`,
   `set flood.max.advert`, truncates names at 32 runes on a rune boundary,
   and sets `time <scenarioEpoch>` rather than `time 0`. Port what's missing.
3. wb1 steps the engine 60 ticks after provisioning so commands are read even
   before play. Do the same, so a paused-but-provisioned mesh answers its
   console.
4. Chase the "playing before the second press" anomaly in the same trace.

(Stagger: already on — wb2 uses `engine.New`, which defaults
`StaggerBoot: true`. An assertion pins it so it cannot regress silently.)

**Acceptance:** screenshot of the map with traffic trails and a Nodes panel
with non-zero heard, from nothing but play.

## P0.2 — Right-click → "Open in its own window"

Reported broken repeatedly; the menu opens (screenshot shows it) and choosing
the entry does nothing. The dispatcher fix landed earlier, so the remaining
break is likely in one of: the popped-out map wiring its own `OnMenu`
differently from the docked one, `node.window` failing silently (its error
goes to a status line the popped-out window doesn't show), or the click that
chooses the entry also landing on the map underneath.

**Acceptance:** in the running app, right-click a node on the *popped-out*
map, choose the entry, node window appears. Captured.

## P0.3 — The map tools, rebuilt on wb1's semantics

wb2's toolbar says `select move place link measure` but nothing reads the
chosen tool: a drag on a node moves it whatever tool is active (the "link mode
moves repeaters" report), place and link do nothing at all.

wb1's model, which is the one to port:
- **select** — click selects; ctrl-click a second node makes the pair a link
  question (the Inspector's "why this link?").
- **move** — the only tool that drags nodes.
- **place** — one tool per kind in wb1 (repeater / companion / observer /
  emitter). wb2 has a single "place": give it a kind picker, then a click
  places that node at the clicked lat/lon.
- **measure** — already works, but through the Measure *layer*; unify so the
  tool and the layer are the same state, not two.
- The gate: `dragNode` only when the tool is move. That single change closes
  the reported bug.

**Acceptance:** each tool exercised in the running app; a node placed, a link
questioned, a node immovable in link mode. Captured.

## P0.4 — Layers that lie

Per Alex, with wb1's behaviour as reference:
- **Coverage** — wanted, does not work. wb1 computes from the selected node
  and draws an overlay. Wire the toggle → compute → `covOp` path, and say
  when there is no selection rather than doing nothing.
- **Terrain** — does not work. The verb exists; find where it fails silently
  (likely needs warmed tiles and says nothing while cold).
- **Weak links** — does not work and is not wanted. Remove the layer.
- **Traffic** — should come alive once P0.1 lands; verify then, not before.
- **Regions** — needs a key when on. Extend the existing map key (it already
  does kinds) with the region colours while the layer is active.

**Acceptance:** every remaining layer toggles something visible, captured; the
removed one is gone.

## P1.1 — Fleet commands show their replies

wb1's loop, which is the model: mark each node's console, type the command at
every target, **step the engine one simulated second** so the firmware reads
and answers, then collect `linesSince(mark)` per node into a results table —
one row per node, in the firmware's own words. It also has: a target picker
(all / repeaters / room servers / companions / selection / filter) with a live
node count, a warning + double-send confirm when a command invalidates the
run (radio changes), quick-command groups that fill the box rather than
sending, and history.

wb2 fires a verb and shows nothing. Port the loop into `fleet.send` and give
the panel the results table. The picker and confirm come with it; quick
commands and history follow if time allows.

**Acceptance:** `ver` to repeaters shows a table of replies. Captured.

## P1.2 — The firmware manager, rebuilt on wb1's model

Alex: "entirely different to how it worked in workbench 1". wb1's model:

One searchable, sortable table of every build — published and on-disk, merged
so imported branch builds appear:
- columns: role · version · board (green "native" / orange board name, with a
  key) · on disk (size, or a **download** button, or "downloading...") ·
  in use by N nodes · **use for role** · **delete** (confirmed, because the
  failure arrives at play).
- filters: search box (all words must match), on-disk only, boards only /
  native only (mutually exclusive).
- board images apply per-node with a cost warning (every node becomes its own
  real-time emulator); host builds apply per role.
- default sort: what the scenario runs now, then what is on disk, then what
  can run.

Replace wb2's form-style Firmware panel (type a role, type a version, type a
path) with this table. The form is the thing the audit kept flagging as
"requires typing what is on screen".

**Acceptance:** download, use-for-role, delete, and the filters all exercised
in the running app. Captured.

## P1.3 — Window behaviour

- **Pop-outs always on top.** Gio has no public always-on-top option, so this
  needs investigation: X11/Wayland hints from the window handle if reachable,
  otherwise this comes back with an honest "not possible in Gio today" plus
  the nearest alternative rather than a silent skip.
- **Duplicate actions.** Companion bench / Planning / Import / Live feed each
  show the same actions twice (panel's own + generic control strip). Keep the
  panel's own, drop the strip copy. (Stated assumption — say if you want the
  opposite.)

## P2 — Polish, after the above

- The density button named "default" is not the default (Comfortable is the
  zero value). Needs a decision on which name changes.
- `waterfall.capture` answers `captured: false` silently.
- "Import a live network" is in two menus.
- Menu duplicates: "Open a saved network" wording vs File→Import.

## What was reviewed to write this

wb1 files read line by line against wb2: `config.go` (startupCommands,
applyStartupConfig, regionToken, 32-rune truncation), `windows.go`
(attachFirmware → provision → status), `traffic.go` (engine build, StaggerBoot,
autoWarm), `fleet.go` (mark/type/step/collect loop, invalidating-command
confirm, targets), `fwmanager.go`/`fwlist.go` (the table model, row merging,
default sort), `workbench.go` (tool semantics), `abtest.go` (stagger in
experiments). Plus every fault report from tonight, each mapped to an item
above.
