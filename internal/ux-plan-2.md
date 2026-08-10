# UX refinement 2 — simpler, and views that mean something

2026-08-10. Grounded in socket-driven screenshots of every view at current
main, after the first cleanup round (presets that reset, ASCII sweep, one
less chrome row, real glyphs, pop-out that works).

## The test a view has to pass

A view is a way of looking at the network while you ask one question. If it
is not a question, it is not a view. Held against that:

| view | question | verdict |
|---|---|---|
| Plan | where should things go? | real view |
| Run | what is it doing? | real view |
| Debug RF | why did that happen? | real view, badly furnished |
| Firmware | — | **not a question. Not a view.** |

## Findings, per view

**Firmware** is a run *mode*, not a way of looking. Everything in it already
lives somewhere better: starting firmware is the `fw ▶` button on the strip,
the consoles are a panel any view can open, fleet commands are the Repeaters
menu, the library is its own window. As a view it shows a map you do not
need and a console sliver. Dissolve it.

**Debug RF** has the right panels in the wrong shapes. The Link cut-through
— a *wide* terrain profile — is squeezed into the narrow right column where
it reads as a smear (screenshot: a 43 km profile drawn 350 px wide and 800
tall). The waterfall owns the wide bottom band while empty. No Inspector, so
selecting a node to interrogate it means leaving the view.

**Plan** is close to right. Import's long form is cramped in the right
column, but it is usable and it belongs there. The Link profile is wide at
the bottom, which is exactly what Debug RF should copy.

**Run** is right, with one overload: the Compare tab carries baseline +
stress + A/B — three dense blocks that are really the "is it still correct"
question, which is a different question from "what is it doing".

**Cross-view mess:**
- The **Coverage menu duplicates the Planning menu** — same four coverage
  actions in both, a leftover from adding Planning without deleting Coverage.
- **"Views" means two things**: the Views menu (saved layouts) and the view
  buttons (Plan/Run/…). One word, two systems.
- **Fleet, Boundary and Planning are floating windows**, not panels — so no
  view can include them, they cannot dock, and saved layouts cannot hold
  them. Two window systems where one would do.
- The **Windows menu** is one flat list mixing panels, windows and node
  windows.
- The **labels checkbox** sits on the toolbar while every other map display
  toggle (links/patterns/region) lives on the map corner strip.
- **Reset node memories** is in the Simulation menu; it is a firmware-storage
  action and already has a button in the Firmware library window.

## The plan

### 1. Four views, designed from what the operator is doing in them

**Plan (ctrl-1) — you are building and siting.** Importing the real network
or placing candidates; grabbing a repeater and dragging it along a ridge
while the link margins recolour; nudging the height slider to see what five
metres buys; setting the boundary; asking what coverage a site adds; fetching
terrain. Your hands are on the map and one node at a time.
→ map dominant · Inspector (the node in your hand) + Nodes + Import right ·
Link profile wide across the bottom, answering the pair you have selected.
Boundary and Planning one click away (panels, see §2).

**Run (ctrl-2) — you are exercising it and watching.** Press play; watch
messages hop on the map; watch events scroll; schedule sends and assertions;
type at a node's console to originate traffic; start the live feed and watch
real packets drive the simulated mesh; glance at the scoreboard for
duty-cycle offenders. You touch little — you observe a system behaving.
→ map · Schedule + Scoreboard + Live feed right · Events + Packet timeline +
Console wide bottom. (Live feed moves here from the first draft of this
plan: in use it is something you *watch drive the run*, not something you
audit.)

**Debug (ctrl-3) — you are asking why one thing happened.** Click the failed
message in Events; open the packet; follow its journey hop by hop; read the
miss reason — SNR, interferer, half-duplex; check the waterfall around that
moment; read both consoles; pull up the budget for the marginal link. It is
a chain of evidence, and every link of it must be in reach without changing
view.
→ map small · Inspector + Budget right · **Link + Waterfall + Console +
Events wide across the bottom** — the cut-through gets the width it needs
(today it is a smear in the right column), and selecting a node never leaves
the view.

**Verify (ctrl-4) — you are checking it is still true.** Snapshot a
baseline, change one thing, find the first divergence; split the fleet A/B
and bisect to the node that carries the difference; fetch real receptions
and read the residuals; apply or remove the calibration; leave shadow mode
watching the agreement figure. This is the falsifiability workflow given a
front door.
→ map · Compare + Validate right · Scoreboard + Events bottom.

- **Firmware view dissolved** — starting firmware is a thing you do (the
  `fw ▶` button), not a place you look. Its parts: strip button, Console
  panel in Run and Debug, Repeaters menu, Firmware library window.
- "Debug RF" renamed **Debug** — consoles and packet dissection are
  debugging too; RF was never the whole of it.
- The Link profile is *wide-bottom in every preset that shows it*. Never the
  right column.

### 2. One window system

Fleet commands, Boundary and Planning become registry panels — dockable,
poppable, listed once, included in presets (Boundary and Planning available
to Plan; Fleet available to Run/Debug), and captured by saved layouts.
Preferences, Provisioning, Firmware library and the packet window stay
floating: they are dialogues you visit, not places you work.

### 3. Menus: one home per thing

- **Coverage menu deleted** — Planning menu already owns coverage; the map
  corner strip keeps the layer toggles.
- **Views menu renamed Layouts** — it saves and restores arrangements; the
  view buttons keep the word "view".
- **Windows menu grouped**: Panels / Windows / Node windows, separated.
- **Reset node memories** moves to the Firmware library window only.
- Menu bar afterwards: File · Layouts · Simulation · Repeaters · Planning ·
  [Plan Run Debug Verify] · Windows · Help.

### 4. Small placements

- labels checkbox → map corner strip, beside links/patterns/region.
- Compare panel keeps baseline+stress+A/B together (they are one workflow),
  but gets the Verify view so it stops crowding Run.
- Debug preset opens the Inspector, so selecting a node never leaves the view.

## What this deliberately does not touch

- The toolbar's placement tools and the always-visible seed: frequent and
  load-bearing.
- The honesty line stays in the chrome.
- Panel *contents* — this plan is furniture only.

## Sequencing

1. **PR 1 — the view set**: dissolve Firmware, add Verify, rename Debug,
   re-cut all four presets (Link wide-bottom everywhere), ctrl-1..4.
2. **PR 2 — menus**: delete Coverage, Views→Layouts, group Windows, move
   reset-memories.
3. **PR 3 — one window system**: Fleet/Boundary/Planning to panels.
4. **PR 4 — placements**: labels toggle to the map strip, Debug polish.

Each PR verified the same way: drive every view over the control socket,
screenshot, and read the screenshots before merging.
