# The master plan — UX, workflows, and what moves where

2026-08-10. Follows `adr-gap-review.md`. The diagnosis first, because every
change below follows from it.

## Diagnosis

The workbench accreted. Each feature landed in whatever container was nearest
— the Load panel now holds provider import, boundary files, saved networks
*and* projects; the right sidebar holds a node list, an inspector and a
firmware picker; seven bottom tabs compete with eight floating windows and
three menus; region configuration lives in a window called "Configuration"
while region *inference* lives in another window entirely. Nothing is wrong
individually. Together they mean an operator cannot predict where anything is,
which reads as "too complicated" — the complaint is correct and the cause is
placement, not features.

Multi-monitor was broken by two real bugs (a detach button that placed the
window 40 px *inside* the main window — now fixed — and imgui's auto-merge
default), and remains fragile because detaching is a per-window afterthought
rather than a first-class layout act.

## The organising principle

**One question, one place.** The UI reorganises around the five questions an
operator actually asks, which are the five workflows the UX spec named:

1. *What is this network?* — *build/import*
2. *Where should things go?* — *plan*
3. *What is it doing?* — *run/observe*
4. *Why did that happen?* — *debug*
5. *Is it still correct?* — *compare/validate*

Everything visible belongs to exactly one of these. Anything that serves two
gets a door in both, but lives in one.

---

## Phase 1 — Workspaces (the structural fix)

ImGui docking (`ConfigFlagsDockingEnable` — the backend already enables it,
nothing uses it) with **four saved workspace presets** as the ADR-0001/UX-spec
always specified:

| workspace | docked layout |
|---|---|
| **Plan** | map maximised · boundary + planning right · coverage controls bottom |
| **Run** | map · run controls + schedule right · timeline (time graph) bottom |
| **Debug RF** | map small · waterfall + symbol large · budget + cut-through right · packet timeline bottom |
| **Firmware** | consoles grid · fleet commands right · reception ledger bottom |

- Workspace switcher in the menu bar, position remembered per workspace in the
  config file. `Ctrl-1..4`.
- **Every panel becomes dockable and detachable.** The bottom tab bar and the
  fixed right sidebar are dissolved into dockable panels with the same names.
  Nothing is always-on except the map and the status/seed strip.
- **Detach is a titlebar affordance on every panel** (the ⇱ button), not a
  per-window special case buried in a node window. It places the window fully
  outside the main viewport (bug fixed) and, on X11, that makes it a real OS
  window on whichever monitor you drag it to. The Wayland limitation stays
  documented in the button's tooltip.

Cost: the layout plumbing is the work; each existing draw function is already
window-shaped. This is the highest-leverage change in the plan.

## Phase 2 — Import becomes a tool, not a panel

The user's own example, and the right one. A new **Import window** (Scenario →
Import…, and a toolbar door):

- **Sources listed by the provider registry** (finally implementing ADR-0016's
  interface): CoreScope, Beacon, MQTT-when-live, file, saved network. Health
  shown per source before anything is fetched.
- **Preview-then-commit.** Fetch shows *what would arrive* — count, bounding
  box drawn on the map, names — before anything touches the scenario. The
  boundary filter applies at preview, visibly.
- **Merge is the default posture, with a strategy**: add-only-new (by public
  key, then name) / replace-matching / replace-all. Today's silent
  append-or-replace checkbox becomes an explicit, previewed choice. This is
  what "merge onto existing networks" needs and never had.
- **Inference folds in here.** After a CoreScope import, the same window offers
  "read the traffic too" with the tick-boxes (regions / default scope /
  flood.max floor). Import-then-infer is one workflow in one window, instead of
  a Load panel plus a separate Infer window whose connection nobody could see.
- The Load panel dies. Saved networks and projects move to the **File menu**
  (Open Project / Save Project / recent list), which is where every desktop
  application keeps them and where nobody has to be taught to look.

## Phase 3 — The sidebar becomes an inspector, and only that

- Right sidebar shows **the selection and nothing else**: node identity,
  position/height/power, radio + preset, firmware, per-node region facts
  (observed and configured), energy once ADR-0011 lands. Multi-select shows
  the shared subset editable in bulk.
- The node *list* stops being a sidebar fixture — it is the Nodes & settings
  window (already good) plus the map. The filter box moves to the map itself
  (type-to-filter highlights matches, like every map tool).
- The node window keeps Console/Stats/Activity/Connect but its Settings tab
  merges into the same inspector component, so a node's settings look identical
  everywhere they appear.

## Phase 4 — Coherent placement fixes

Things in the wrong place today, each with its destination:

| thing | today | belongs |
|---|---|---|
| boundary GeoJSON path box | Load panel | Import window + Boundary window |
| "delete outside boundary" | Load panel | Boundary window (already also there) |
| seed | toolbar (stays) + also Configuration | toolbar only |
| speed control | Traffic tab | run-control strip (with ▶ ⏸ step) always visible in Run workspace |
| "run real firmware" | Traffic tab + Simulation menu | run-control strip; menu keeps it |
| coverage layer picker | Coverage menu | Plan workspace panel with the opacity/clear controls beside it |
| layer toggles (links/patterns/region) | floating strip on map | map-corner strip (stays) but collapsed behind one ⚙ at small sizes |
| fleet quick-commands | flat button rows | grouped: Flood · Regions · Radio · Info, collapsible |
| stress test | Scheduled-traffic window | Compare workspace (it is a measurement, not a schedule) |
| Configuration window | mixes app prefs + on-start CLI | split: **Preferences** (app behaviour) and **Provisioning** (what nodes are told on boot) |

## Phase 5 — Workflow polish that keeps being requested

- **Import → everything follows**: after import, auto-offer (not auto-run):
  boundary inference, terrain estimate/fetch, firmware start. One "get me
  going" strip of three buttons with costs stated, instead of four windows.
- **Right-click grows into the primary verb surface** (the "beauty of imgui"
  point): node → open window / console / neighbours / coverage / move / link /
  delete / *provision*; map → place / paste / fit / boundary here; timeline row
  → packet window / follow message / open both consoles.
- **Escape hatches everywhere**: every long operation (warm, coverage, import,
  inference) shows in one **Jobs** popover on the status strip with progress
  and cancel — today they are four differently-shaped progress indicators.
- **Empty states teach.** Every panel's "nothing yet" line says the *next
  action* ("run real firmware from the strip above", "select two nodes"),
  which is cheaper than any tutorial.

## Phase 5b — Live feed (added 2026-08-10)

Real packets drive the simulated mesh as they happen. Ingest live traffic
(CoreScope polling; MQTT streaming; beacon has no packet feed), take each
message at its **first hop only** — the copy straight from the origin, or one
relay in, so the transmitter is known rather than reconstructed — and inject
it at the same-**named** simulated node. Names, never keys: the simulation's
identities are generated from the seed and can never match the real network's.
The simulated mesh (real firmware, our RF) relays from there, and what it does
differently from the real network is the finding.

Injected frames are modified before transmission: the recorded path bytes are
the real network's hashes, so the path is rewritten to the simulated
injector's own hash and padded to a **minimum of 2 path bytes**, trimming the
remaining flood budget so replay does not become a collision storm the real
network never had.

## Phase 6 — The debt the gap review ranks first

Not UX, listed because the plan would be dishonest without them, in order:

1. **ADR-0015 validation chain** — replay observed traffic, residuals,
   calibration, shadow mode. The only work that makes the model falsifiable.
2. **Energy in the workbench** (ADR-0011) — the winter question.
3. **Real emitters + Ofcom** (ADR-0012).
4. **NDJSON event log + Wireshark live pipe** (ADR-0007) — cheap, high leverage.
5. **A/B splitter + bisect** on the existing assertions/divergence.
6. **Provider interface + registry** (ADR-0016) — unblocks the Import window.
7. **GPU coverage** when raster demand outgrows 40 stations (ADR-0025).

## Sequencing

Phase 1 first — everything else docks into it. Then 2 (Import window, provider
registry with it), 3+4 together (they are mostly moves), 5 opportunistically,
6 interleaved with 1–4 by the order above. ADRs to write as these land:
workspaces (amends 0001), the Import tool (amends 0016), provisioning split
(amends 0014).

## What this plan refuses to do

- No web tech, no second UI toolkit, no rewrite. The panels are fine; the
  furniture is wrong.
- No feature flags for the reorganisation — one layout that is right beats two
  that are configurable.
- Nothing added to the map that cannot be turned off from the map.
