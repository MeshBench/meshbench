---
name: wb2-design-language
description: The workbench design language - load before building or changing any Gio panel, control, menu, or map drawing so new work matches what shipped
---

# The workbench design language

How MeshBench's Gio interface is built. These are decisions Alex has made,
mostly by rejecting something that did not follow them; breaking one recreates
a reported bug.

Almost none of it is enforced by a linter, because most of it is about what a
control *means* rather than about what compiles. Treat every rule below as a
convention you are expected to keep, not as something the build will catch.

## Colour and type

- **Every colour and size comes from `theme.Theme`** (`internal/ui/theme`).
  Palette is semantic: `Ink/Dim/Faint` for text, `Panel/Sunk/Ground` for
  surfaces, `Rule` for borders, one `Accent`, `Good/Warn/Bad` for states.
  Need a new colour? Add a token to both palettes, then use it. Nothing
  checks this, and `comp/mapbuildings.go` and `comp/mapcoverage.go` both
  break it today, so the existence of a literal is not permission for another.
- **Text drawn on the map uses `MapInk`/`MapInkDark`, never theme ink.** The
  basemap layer says whether its ground is dark (`basemap.Layer.Dark`);
  `MapView.baseInk` (in `comp/mapscale.go`) picks. Theme ink on a light
  basemap was invisible - no plates, no halos; the font changes, per Alex.
- **Mono (`comp.Mono`, `Column.Mono`) for anything compared by eye down a
  column**: numbers, versions, identifiers, hex.

## The shapes of things

- **Card + StatCell + CellGrid** (`comp/cards.go`) for settings and overview
  pages: a labelled value with the "why it matters" caption underneath. The
  caption is content, not decoration - it is the reason the number exists.
- **Chips** (`comp.Chip`, `comp/chips.go`) for filters and tabs: capsule,
  count beside the label, tinted when active. Cards above, chips below, table
  underneath - the events panel and the firmware library both follow this
  page shape.
- **Dropdowns own no list.** `comp.Dropdown` shows the value and a drawn
  chevron; pressing hands the choosing to a `choose func(title, opts, pick)`
  callback. Build it with `chooserIn(panel)`, not by reaching for `sh.Ask`
  directly: that routes the question through `windows.promptFor`, so a
  popped-out panel asks in **its own** window rather than in the main shell.
- **Switches are `comp.Check` (in `comp/comp.go`) drawn with `LayoutSwitch`** -
  same widget.Bool underneath, so the control audit still finds them.
- **Pills** (`comp.Pill`) for status words (Ready to run / Warming / Running).
  Capsule radii are `size.Y/2`, never a big constant: Gio does not clamp
  RRect radii, and an oversized radius smears fill across the window.
- **Glyphs are drawn, never an icon font** (`shell/glyphs.go`, the transport's
  symbols, the library's role icons). A font is a file a machine may not have.
- **Tables**: `comp.Table` for plain sortable data. Custom row renderers
  (events, firmware library) must force every cell to its declared column
  width - `d.Size.X = px` - or a 12px tick slides everything after it off its
  header. Column labels and widths live in one table-of-columns per panel.

## One table, or the key drifts

**A colour and the word for it come from the same table, read by both the
drawing and the legend.** `comp.ClassColour` and `comp.ClassLabel` are that
pair for event classes; `timelineKinds()` in `comp/timeline.go` is the same
move for the timeline, feeding `legend()` and `marks()` from one list. A key
maintained beside the thing it describes is a key that will disagree with it,
and a wrong legend is worse than none because it is believed.

The event classes grew from five to eight (`sent`, `received`, `half-duplex`,
`interference`, `collision`, `receiver-busy`, `floor`, `unclassified`), which
is what a table absorbs and a hand-written key does not.

## Behaviour

- **Panels never mutate state.** Controls fire verbs through `do(verb,
  params)`; the store owns the world; panels draw snapshots. A control that
  needs input the button cannot carry asks through the prompt - the verb
  itself must still refuse when the parameter is missing, because scripts call
  it directly.
- **Widget state belongs to the panel, never to the package.** A map keyed by
  action name at package scope was shared by every window in the process; two
  pop-outs writing it was a fatal concurrent map write, and a mutex would not
  have saved it, because they would still be sharing one `Clickable`.
- **Widget identity is address**: never rebuild a widget per frame; pool
  per-row widgets in a map keyed by a stable row key, and **bound the pool**.
  The events panel kept one clickable per distinct event for ever while the
  store's tail dropped them; it now rebuilds against what is on screen, with
  slack so typing in a filter does not rebuild every frame.
- **Nothing touches the disk on the frame goroutine.** The compare and runs
  panels share `runloader.go`, which loads off-frame and hands the result to
  whichever frame asks next.
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
  "did not apply" is a dash, not a cross. The same rule is why a miss shows
  the cause the engine established and `unclassified` where it established
  none, rather than a plausible default.
- **Settings that survive a restart** go through `session.Prefs`
  (`workbench2.json`) - loaded by the command, never by `Register`, so tests
  stay hermetic. The scenario itself deliberately stays in the fixture.

## It has to read docked, not just popped out

A panel is laid out in a narrow rail far more often than in a window of its
own, and a layout that is only checked popped out fails in the way that reads
as a rendering fault rather than as a layout one: the Link panel drew nothing
docked because a rigid header ate the width and the flexed chart was left zero
height.

The pattern to copy is in `workbench/linkprofile.go`: a width breakpoint, the
header stacking below it, the chart *measured* with a floor rather than
guessed, and the whole panel scrolling when the floor wins. Clip a chart to its
own box. **Capture both widths before believing it.**

## Menus and windows

- Menu structure is data on `shell.MenuItem` (Section/Icon/Shortcut); the
  table in `workbenchMenus()` feeds the rows, the key filter and the tests,
  so caption and binding cannot drift. Shortcuts match on exact modifiers.
- **Every panel names its `Menu` and its `Section`, and the entries are
  generated** by `Shell.PanelItems`. There is no curated daily set and no
  "Show all panels..." any more: the curated thirteen left twenty panels
  reachable only through a chooser that then threw them out of the window.
  A panel with no menu is a panel nobody finds.
- The Window menu is about windows and layouts (`layout.reset`,
  `window.raise_all`, `window.dock_all`), not about which panels exist.
- One entry lives in one menu.

## A file that hits 500 lines is split along a seam

The hard limit is a build failure (`tools/file-length.sh`), and `mapworld.go`
sitting at 499 blocked two unrelated changes. Splitting is a first-class move,
but split on meaning: what draws the **world** stayed in `mapworld.go`, what
draws a **study's answer laid over it** left for `mapcoverage.go`. A split by
line count alone leaves two files nobody can name.

## Verification (non-negotiable)

- **Nothing is done until it is seen running.** Green tests alone have let
  bugs survive; the pill smear, the invisible hillshade and the drifting
  columns were all caught only in captures.
- **Everything reachable by flag**: a panel, section, menu, or layer that
  only opens on a click is one nobody can capture. `internal/ui/workbench/main.go`
  carries around thirty of them (`-panel`, `-view`, `-config-section`,
  `-drop-menu`, `-layers`, `-look`, `-node-tab`, `-coverage`, ...) plus
  `-quit-after` and `-control-socket`. A new view adds one.
- **There is no blessed screenshot tool in the repository.** Driving is the
  control socket through `pkg/client-python` (`tools/soak/` is the worked
  example); capturing is the compositor's own grabber, because under Wayland
  the app does not render on Xwayland and an X11 grab returns a solid black
  screen that looks exactly like a crash. The closest thing to a committed
  capture path is the render tests, `workbench/brandshot_test.go` and
  `hardwareshot_test.go`, which write PNGs. `board.screenshot` is not this: it
  captures an emulated board's own display.
- **The control audit** (`workbench/audit_test.go`, targets in
  `audittargets_test.go`) walks panel structs for Button/Check/Field and
  presses everything. New panels join `auditTargets`; panels whose sections
  hide controls provide a flat `auditDraw`. It now *waits* for a control's
  effect rather than assuming two frames are enough, so a control that defers
  its work passes - and a destructive control that asks before acting spends
  the whole waiting budget, which is why the suite takes near two minutes.
- Controls that change the view rather than the world (sidebar rows) are plain
  `widget.Clickable` with their own test.

## A panel that opens itself

The Setup panel is the only one that does, and the rules it had to keep are
worth reusing:

- **It is a report, not a wizard.** No row acts on its own; a row nothing can
  fix carries the steps in words rather than pointing at a document.
- **It opens only when something is blocking or waiting to be told.** A
  machine that is set up sees nothing, which is what stops it being a splash
  screen.
- **It waits three seconds first.** A panel docked before the layout exists
  lands nowhere, and a check run before the fixture opens misreads every
  missing firmware as optional.
