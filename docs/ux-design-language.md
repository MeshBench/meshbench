# MeshBench design language — what each thing is, and what that decides

2026-08-10. The whole-tool review. The complaint this answers: "it just
feels a bit everywhere" — and the diagnosis is that the feeling is accurate,
because the tool currently has about ten *kinds* of surface and no rule for
which kind a feature gets. The fix is not another round of moving buttons;
it is a small taxonomy, applied everywhere, so that what something *is*
decides what it *looks like* — the way it does in the tools this one should
sit beside (Wireshark, QGIS, KiCad, Blender, a DAW).

## The taxonomy

Seven kinds. Every current and future surface must be exactly one of them.

| kind | what it is | presentation | examples elsewhere |
|---|---|---|---|
| **View** | a perspective for one activity | a tab in one standout tab strip | Blender workspace tabs |
| **Panel** | a place you work, docked or popped out | dockable window with shared chrome | QGIS panels, IDE tool windows |
| **Browser** | a collection you pick from | a *table* in a single-instance window | font pickers, package managers |
| **Dialog** | settings you visit and dismiss | single-instance window, never docked | every Preferences ever |
| **Command** | a thing you do once | menu item with a shortcut; context menu for object verbs | everywhere |
| **Chrome** | always-on instruments | fixed strips that never move or reflow | DAW transport, status bars |
| **Overlay** | information *on* the stage | drawn on the map, toggleable from the map | QGIS map decorations |

The map is none of these: it is the stage, always centre.

## What is currently mis-kinded

- **The view switcher is Chrome pretending to be menu items.** Plan/Run/…
  sit *between* menus in the menu bar — a category error, and why "where do
  I look" has no answer. Views deserve the standout treatment: a dedicated
  tab strip.
- **Firmware library is a Browser wearing a window's clothes.** It is a
  catalogue — role, version, published, downloaded, in-use count — which is
  a *table you sort and pick from*, not a place you work. It must not dock
  (correctly), but today it presents as prose sections rather than the
  table it is. Same kind, same treatment: saved networks (already a table,
  correct), projects (currently menu rows — borderline, acceptable),
  layouts (menu rows, acceptable).
- **Fleet, Boundary and Planning are Panels wearing Dialog clothes** — you
  *work* in them, sometimes for an hour, yet they cannot dock, join a view,
  or be saved in a layout. (Already planned: ux-plan-2 §2.)
- **The status line is Chrome that reflows.** It appears inside the toolbar
  row and shoves the layout down when it has something to say — an
  instrument that moves the panel you were reading. Status belongs in a
  fixed-height bar.
- **The Jobs popover hangs off a toolbar button** that appears and
  disappears. Background work is status; it belongs at the status bar's
  right end, always in the same place.
- **Two menus own visibility** (Views for layouts, Windows for panels) and
  one menu owns nothing anyone can name (Simulation holds capture, event
  log, node memories, firmware — four different objects).

## The target

### Chrome: two rows on top, one on the bottom

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  View  Simulation  Repeaters  Planning  Window  Help    [best-case note] │  menu bar
│ ⟨Plan │ Run │ Debug │ Verify⟩   ▶ ▮▮ ▶▮ ↺  fw▶  1x  t=4.2s   4 nodes · seed 4417 │  control row
├──────┬──────────────────────────────────────────────────┬───────────┤
│ tool │                                                  │  panels   │
│ rail │                    map                           │  (right)  │
│  ▖   │   [filter]                    [layer toggles ⚙]  │           │
│      │                                                  │           │
│      ├──────────────────────────────────────────────────┴───────────┤
│      │                    panels (wide bottom)                      │
├──────┴──────────────────────────────────────────────────────────────┤
│ status text                                    jobs: 2 ▪  [dismiss] │  status bar
└─────────────────────────────────────────────────────────────────────┘
```

- **Control row** = view tabs (left, the standout), transport (centre),
  facts (right: node count, seed). One row where there are currently two,
  and the view switcher stops hiding among menus.
- **Tool rail**: the placement palette (select/move/repeater/companion/
  observer/emitter) becomes a slim vertical rail on the map's left edge —
  the KiCad/QGIS convention for stage tools. `fit`, layer picker and
  `get terrain` join the map's corner overlays where the other display
  controls already live. The old toolbar row disappears entirely.
- **Status bar**: fixed height, always present. Status text left; jobs
  (count, progress, cancel) right. Nothing above it ever reflows because of
  a message again.

### Menus: seven, each owning one object

| menu | owns |
|---|---|
| **File** | project, saved networks, import, quit |
| **View** | the four views (as commands with ctrl-1..4), panel visibility, saved layouts, UI scale |
| **Simulation** | play/step/restart, speed, firmware start, capture to pcapng, NDJSON log |
| **Repeaters** | fleet commands, provisioning, regions, firmware library |
| **Planning** | coverage, boundary, terrain, site tools |
| **Window** | node windows, pop-out management, dock everything back |
| **Help** | shortcuts, the honesty note expanded, about |

Views menu and Windows menu dissolve into View and Window. Coverage menu is
already condemned (ux-plan-2). "Reset node memories" leaves Simulation for
the Firmware library, beside the storage it wipes.

### Browsers become tables

**Firmware library**, rebuilt as the table it is:

| role | version | downloaded | in use by | |
|---|---|---|---|---|
| simple_repeater | v1.17.0 | yes | 38 nodes | [use everywhere] |
| simple_repeater | dev | — | 2 nodes | [use everywhere] |
| companion_radio | v1.17.0 | yes | 4 nodes | [use everywhere] |

sortable, one row per build, fleet composition read straight off the
"in use by" column — plus the storage section (wipe memories, cache paths)
below. Single-instance window from the Repeaters menu. Never docked.

The same shape review, applied to the other pickers: saved networks
(already a table — correct), radio presets (combo — correct, it is a
per-node property), basemap layers (combo — correct), projects and layouts
(menu rows — acceptable while they are short; promote to tables if they
grow).

### Rules going forward (the part that keeps it simple)

1. New feature → name its kind first. No eighth kind without a fight.
2. A Panel gets registry chrome (pop out / dock) or it is not a Panel.
3. A Browser is a table. If it has prose sections, it is a Dialog or a
   Panel pretending.
4. Commands live in exactly one menu, plus optionally a context menu and a
   shortcut. Duplicates are bugs.
5. Chrome never reflows. If it can appear and disappear, it is not Chrome.
6. Nothing goes on the map that cannot be toggled from the map.

## Sequencing (continues ux-plan-2's numbering)

5. **PR 5 — chrome**: control row (tabs + transport + facts), tool rail,
   status bar with jobs. The largest visual change; do it after the view
   set (PR 1) so the tabs have the right names.
6. **PR 6 — menus**: View and Window consolidation (absorbs plan-2 PR 2).
7. **PR 7 — firmware library as a table.**

Verification unchanged: drive every view over the control socket,
screenshot, read the screenshots before merging.
