# MeshBench flows and visual language

2026-08-10. Companion to ux-design-language.md. Two questions: *what does an
operator actually have to do, task by task* — and *why does it not look like
a modern tool yet*.

## Part 1 — twenty tasks, walked honestly

Each task: the steps today, a verdict, and the target where the verdict is
not "good". Findings that need code are numbered **F1–F8**.

| # | task | steps today | verdict |
|---|---|---|---|
| 1 | import my network from CoreScope | File > Import (or Plan view) → URL → check → fetch preview → merge → commit | good — but the URL is retyped **every launch** (**F1**) |
| 2 | save my work, reopen tomorrow | File > Save project as… → name; File > Open project | works, but there is no *current project*: every save asks for a name, no ctrl+S, no title-bar name, no unsaved-changes hint (**F2**) |
| 3 | start firmware, originate a message | `fw ▶` → right-click node → open window → Console → `advert` | good since the console teaches; acceptable |
| 4 | find out why B missed the message | Debug view → Events → filter → click row → packet → "Where it went" | good chain — but the *shortcut* from a miss row straight to the A↔B budget is missing (**F3**) |
| 5 | check one specific link | click A, ctrl-click B → Link panel | good |
| 6 | whole-network coverage | Planning > best server → watch jobs → overlay | good |
| 7 | set every repeater's region, verify | Repeaters > set region from study area → replies in Fleet | **trap**: silently does nothing unless a Provisioning checkbox is on — a menu command gated by a distant setting (**F4**) |
| 8 | drive the sim with real live traffic | Import: set corescope URL (Plan) → Run view → Live feed → start | acceptable; the panel names its dependency |
| 9 | validate the model against reality | Verify view → Validate → fetch and compare | good |
| 10 | put 40 nodes on SF9 | shift-click each node… forty times | **no select-all, no select-by-filter** — bulk edit exists but selection does not scale (**F5**) |
| 11 | drag a node, watch margins | move tool → drag | good; the tool rail will shorten reach |
| 12 | place a repeater at a known lat/lon | place tool → eyeball the map | **no numeric entry** — Inspector shows position read-only (**F6**) |
| 13 | rename a node | — | **cannot** (**F6**) |
| 14 | compare two firmware versions | Verify > Compare > A/B → assign → `fw ▶` → bisect | good |
| 15 | live-capture into Wireshark | Simulation > capture live → *retype the command from the status line by hand* | status text is not selectable; any message carrying a command needs a copy button (**F7**) |
| 16 | fix tiny UI on a 4K monitor | ctrl + | good |
| 17 | two-monitor layout, kept | pop out → arrange → Layouts > save | good |
| 18 | find one node among 400 | map filter box | good |
| 19 | duty-cycle compliance check | Run > Scoreboard, red duty column | good |
| 20 | quit | ✕ / ctrl+Q | good (finally) |

Flow findings, ranked by how often they bite:

- **F1 — persist the import source.** URL and source survive restarts;
  token stays session-only unless the operator opts in.
- **F2 — a current project.** Open/save establishes it; ctrl+S saves it;
  the title bar reads "MeshBench — scotmesh"; a dot marks unsaved changes.
- **F5 — selection that scales.** "Select all", "select filtered" (the map
  filter becomes a selection tool), and the bulk editor inherits them.
- **F4 — commands never gated by distant settings.** The Repeaters region
  action provisions with its own defaults and *says what it sent*; the
  Provisioning checkbox governs boot-time only.
- **F6 — the Inspector edits identity.** Name (uniqueness enforced) and
  numeric lat/lon/height fields, not just sliders.
- **F7 — copyable commands.** Any status or panel text that is a shell
  command gets a copy button (imgui text is not selectable).
- **F3 — "why not?" on miss rows** — context menu opens the budget for
  that pair directly.
- **F8 — first-run experience.** The demo scenario is good; add a one-time
  status line naming the three doors: import, place, play.

## Part 2 — the visual language

The honest verdict on today's look: 1995. Cause: the stock bitmap font at
13 px, stock ImGui dark theme, zero rounding, six ad-hoc colours applied
inline, and glyph sizes that wobble because symbols come from a different
font than their buttons.

### Typography

- **UI face: Inter** (OFL, embedded like DejaVu) at 15 px — the modern
  neutral for tool UIs; excellent at small sizes and dense tables.
- **Symbols: DejaVu Sans merged** (already shipped) — one merge, so ▶ ▮▮ ↺
  render at matching optical size inside equal-width buttons. Symbol
  buttons get one fixed square size; no more eyeballing.
- **Mono face: DejaVu Sans Mono merged for consoles, hex dumps and the
  event ledger** — aligned columns stop pretending with the UI face.
- Dynamic atlas keeps all three sharp at every ctrl+/- scale.

### Colour: tokens, not literals

One palette in one file (`internal/ui/theme.go`), every inline
`NewVec4(0.95, 0.72, …)` replaced by a name:

| token | role | value (dark) |
|---|---|---|
| `bg0` | app background | #0E1116 |
| `bg1` | panel background | #151A21 |
| `bg2` | inputs, headers | #1C232E |
| `accent` | selection, active tab, primary buttons | #4C8DFF |
| `text` / `textDim` | primary / secondary | #D7DEE8 / #8B94A3 |
| `ok` / `warn` / `err` | verdicts, margins, alerts | #46C574 / #E8B33D / #E05252 |

The six scattered warning/error/ok vec4s already agree approximately;
tokenising makes them agree exactly, and gives a light theme a place to
exist later.

### Shape and rhythm

- Rounding: frames 4 px, windows/popups 6 px, grabbers 4 px, tabs 4 px —
  the difference between 2010 and now is mostly corner radius and spacing.
- Spacing scale: window padding 10, frame padding 8×5, item spacing 8×6 —
  denser than stock, aligned to a 2 px grid.
- 1 px borders at low contrast for panel separation; no bevels.
- Button hierarchy: primary (accent fill — play, commit, fetch), standard
  (bg2), destructive-armed (err fill — the second click of restart/delete).
- Tables: header row in bg2, row striping at 3% — the firmware library,
  saved networks and scoreboard all read as one family.

### What stays

Dark-first, the amber honesty line, the map as the only saturated surface —
the UI should recede and the data should not.

## Sequencing (continues the numbering)

8. **PR 8 — theme and typography**: Inter + mono merge, theme.go tokens,
   rounding/spacing, symbol-button sizing. Pure look; no behaviour.
9. **PR 9 — flow fixes, first batch**: F1 persist source, F2 current
   project + ctrl+S, F4 ungate region action, F7 copy buttons.
10. **PR 10 — flow fixes, second batch**: F5 selection at scale, F6
    identity editing, F3 miss-row shortcut, F8 first-run line.

Order of the whole programme: PR 1 (views) → 5 (chrome) → 8 (theme) → 6
(menus) → 2/3/4 (windows/placements) → 9/10 (flows) → 7 (library table).
The look lands early on purpose: every later screenshot review then judges
the real thing.
