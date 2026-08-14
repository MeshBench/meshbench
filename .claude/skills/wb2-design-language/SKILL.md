---
name: wb2-design-language
description: The workbench2 design language - load before building or changing any Gio panel, control, menu, or map drawing so new work matches what shipped
---

# The workbench2 design language

How MeshBench's Gio interface is built. These are decisions Alex has made,
mostly by rejecting something that did not follow them; breaking one recreates
a reported bug.

## Colour and type

- **Every colour and size comes from `theme.Theme`** (`internal/gui/theme`).
  Nothing outside that package writes a literal colour or pixel count.
  Palette is semantic: `Ink/Dim/Faint` for text, `Panel/Sunk/Ground` for
  surfaces, `Rule` for borders, one `Accent`, `Good/Warn/Bad` for states.
  Need a new colour? Add a token to both palettes, then use it.
- **Text drawn on the map uses `MapInk`/`MapInkDark`, never theme ink.** The
  basemap layer says whether its ground is dark (`basemap.Layer.Dark`);
  `MapView.baseInk` picks. Theme ink on a light basemap was invisible - no
  plates, no halos; the font changes, per Alex.
- **Mono (`comp.Mono`, `Column.Mono`) for anything compared by eye down a
  column**: numbers, versions, identifiers, hex.

## The shapes of things

- **Card + StatCell + CellGrid** (`comp/cards.go`) for settings and overview
  pages: a labelled value with the "why it matters" caption underneath. The
  caption is content, not decoration - it is the reason the number exists.
- **Chips** (`comp.Chip`) for filters and tabs: capsule, count beside the
  label, tinted when active. Cards above, chips below, table underneath -
  the events panel and the firmware library both follow this page shape.
- **Dropdowns own no list.** `comp.Dropdown` shows the value and a drawn
  chevron; pressing hands the choosing to the shell's chooser
  (`Prompt.Choose`) via a `choose func(title, opts, pick)` callback wired in
  main.go. One way to pick from a list, everywhere.
- **Switches are `comp.Check` drawn with `LayoutSwitch`** - same widget.Bool
  underneath, so the control audit still finds them.
- **Pills** (`comp.Pill`) for status words (Ready to run / Warming / Running).
  Capsule radii are `size.Y/2`, never a big constant: Gio does not clamp
  RRect radii, and an oversized radius smears fill across the window.
- **Glyphs are drawn, never an icon font** (`shell/glyphs.go`, the transport's
  symbols, the library's role icons). A font is a file a machine may not have.
- **Tables**: `comp.Table` for plain sortable data. Custom row renderers
  (events, firmware library) must force every cell to its declared column
  width - `d.Size.X = px` - or a 12px tick slides everything after it off its
  header. Column labels and widths live in one table-of-columns per panel.
- **Event classes** map to colour in exactly one place: `comp.ClassColour`.
  Cards, chips, pills and cause text all read it.

## Behaviour

- **Panels never mutate state.** Controls fire verbs through `do(verb,
  params)`; the store owns the world; panels draw snapshots. A control that
  needs input the button cannot carry asks through `sh.Ask` (prompt or
  chooser) - the verb itself must still refuse when the parameter is missing,
  because scripts call it directly.
- **Every long operation announces itself** in the jobs strip (`job.progress`
  / `job.done`) or is not long. Estimates are said before spending ("fetching
  412 of 500 tiles, roughly 25 MB"). Three healthy waits were reported as
  crashes because nothing said so.
- **Silence is a bug.** A verb that declines says why (`w.Say`, `ui.said`).
  A fallback (GPU to CPU) is announced. A count that is capped says what was
  dropped and why.
- **Destructive actions ask twice in place** (delete → "sure?"), never via a
  modal.
- **Honesty lines are content**: "results are a best case: no multipath, bare
  earth, ideal demodulator" stays visible; "no data" is never drawn as zero;
  "did not apply" is a dash, not a cross.
- **Settings that survive a restart** go through `session.Prefs`
  (`workbench2.json`) - loaded by the command, never by `Register`, so tests
  stay hermetic. The scenario itself deliberately stays in the fixture.

## Menus and windows

- Menu structure is data on `shell.MenuItem` (Section/Icon/Shortcut); the
  table in `workbenchMenus()` feeds the rows, the key filter and the tests,
  so caption and binding cannot drift. Shortcuts match on exact modifiers.
- The Window menu lists the curated daily set (`Panel.InWindowMenu`);
  everything else stays one step away behind "Show all panels...".
- One entry lives in one menu.

## Verification (non-negotiable)

- **Nothing is done until it is seen running.** Build on elite, drive through
  the control socket (`/tmp/ctl2.py`), capture window-targeted screenshots
  (`/tmp/shot.sh`), and look at them. Green tests alone have let bugs
  survive; the pill smear, the invisible hillshade and the drifting columns
  were all caught only in captures.
- **Everything reachable by flag**: a panel, section, menu, or layer that
  only opens on a click is one nobody can capture (`-panel`,
  `-config-section`, `-drop-menu`, `-layers`, ...).
- **The control audit** (`audit_test.go`) walks panel structs for
  Button/Check/Field and presses everything. New panels join `auditTargets`;
  panels whose sections hide controls provide a flat `auditDraw`. Controls
  that change the view rather than the world (sidebar rows) are plain
  `widget.Clickable` with their own test.
- Widget identity is address: never rebuild a widget per frame; pool per-row
  widgets in a map keyed by a stable row key.
