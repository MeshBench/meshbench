# The Gio redesign

A complete replacement of MeshBench's interface, built on Gio, with no feature
lost and a deliberate visual identity rather than an inherited one.

This document is the contract. Everything the current application does is listed
here with a box beside it, because the failure mode of a UI rewrite is not a bad
design, it is a quiet loss of the fourteenth thing nobody remembered.

**Scope:** 48 source files, 19,810 lines, 24 panels, 6 views, 6 window types, 87
control verbs, and about a dozen custom-drawn surfaces.

---

## 1. Principles

1. **No feature is lost.** Section 4 is the inventory. A box is ticked only when
   the feature works in the new UI, not when its code has been written.
2. **The control socket is the contract.** Every verb keeps working, and the
   parity test is generated from the dispatch switch rather than from this
   document, because a hand-counted list already proved itself wrong once. The
   handful of verbs whose semantics are entangled with the old layout model are
   named in 12.10 with what changes, rather than silently redefined.
3. **Layout is declared, not placed.** No pixel arithmetic in view code.
4. **Docking is not the organising idea.** A view is a fixed arrangement chosen
   for that kind of work. Any panel can still become a real OS window, because
   `panel.pop_out` is generic over all of them and scripts rely on it; what is
   retired is free-form dock rearrangement inside the main window.
5. **Drawing surfaces stay GPU work.** The map, waterfall and timelines are
   already the right shape; they are hosted, not rewritten.
6. **Nothing renders on the frame thread that does not have to.** The control
   socket, screenshots and headless mode stop being hostage to the renderer.
7. **Every number keeps its units and its uncertainty.** The interface's job is
   to make a result legible, including how much to trust it.

---

## 2. Architecture, before any pixels

The current UI's worst structural problem is not ImGui. It is that control verbs
are serviced on the frame thread, which is why headless mode needed an ADR,
screenshots fight the renderer, and a sweep blocks the console.

- [ ] **2.1** Introduce an application state type owned by a single goroutine,
      with the renderer and the control socket both talking to it by message.
- [ ] **2.2** Move verb handling off the frame thread. Verbs mutate state and
      return; the renderer reads a snapshot each frame.
- [ ] **2.3** Snapshot model for rendering: the frame reads an immutable view of
      state, so a long verb cannot tear a frame.
- [ ] **2.4** Panel registry becomes a declarative description: name, view
      membership, whether it can become a window, and its builder.
- [ ] **2.5** Window manager: main window plus N auxiliary windows, each with
      its own event loop, sharing the same state.
- [ ] **2.6** **Engine pacing moves out of the frame loop.** Stepping lives in
      the render loop today so runs are watchable; the state goroutine owns a
      tick loop decoupled from render, and a long verb or sweep cannot
      head-of-line-block `ui.state` (four verbs are special-cased as polls
      today; the new model needs no special cases).
- [ ] **2.6a** P0's exit gate: the parity script passes over the socket against
      the **existing ImGui UI re-pointed at the new state layer**. Not
      `meshbench test`, which never touches the UI or the socket and would pass
      vacuously.
- [ ] **2.7** Keep `internal/ui` compiling throughout by building the new UI in
      `internal/gui` and switching the binary over at the end.

---

## 3. Design system

The current palette is 11 colours in `theme.go` and the type is one embedded
face at three sizes. The new one is a system, defined once, with every value
named and justified.

### 3.1 Colour
- [ ] **3.1.1** Two complete palettes, dark and light, as semantic tokens:
      ground, panel, sunk, ink, dim, faint, rule, accent, good, warn, bad.
- [ ] **3.1.2** Neutrals biased slightly toward the accent hue rather than pure
      grey, so the interface reads as chosen.
- [ ] **3.1.3** Node-kind colours as a separate scale: repeater, advanced
      repeater, companion, room server, observer, emitter. Distinguishable in
      both themes and under the two most common colour-blindness types.
- [ ] **3.1.4** Signal scales for data: RSSI, SNR, duty cycle, reach. Sequential
      where the quantity is ordered, diverging where zero means something.
- [ ] **3.1.5** Follow the system theme by default, with an override.

### 3.2 Type
- [ ] **3.2.1** Three faces: a UI sans, a monospace for numbers and identifiers,
      and a colour emoji fallback. All embedded except emoji, which is found on
      the system with a graceful absence.
- [ ] **3.2.2** A type scale with named roles: display, title, section, body,
      label, caption, mono-data.
- [ ] **3.2.3** Tabular figures everywhere numbers line up in a column.
- [ ] **3.2.4** Text that cannot fit ellipsises rather than wrapping mid-word,
      and a tooltip carries the full value.

### 3.3 Spacing and density
- [ ] **3.3.1** A 4 px spacing scale; no arbitrary insets.
- [ ] **3.3.2** Three densities: comfortable, default, compact. Goldens run at
      the default; the other two are checked by the gallery smoke rather than
      pixel comparison, which keeps the matrix sane without cutting scope.
- [ ] **3.3.3** Consistent panel chrome: title, optional actions, body, optional
      footer.

### 3.4 Motion
- [ ] **3.4.1** Motion only where it carries meaning: panel open, selection
      change, a value updating, a run finishing.
- [ ] **3.4.2** Respect a reduced-motion preference.
- [ ] **3.4.3** Nothing animates during a run that would cost frame budget.

---

## 4. The component library

Built once, used everywhere. This is the part that makes the rest cheap, and the
part ImGui never gave us.

- [ ] **4.1 Button** — primary, secondary, quiet, destructive; disabled state
      with a reason on hover.
- [ ] **4.2 Toggle and checkbox** — with a label that is itself the hit target.
- [ ] **4.3 Text field** — placeholder, validation state, unit suffix, numeric
      stepping, clamping with the limit stated rather than silent.
- [ ] **4.4 Select** — searchable when over ten options.
- [ ] **4.5 Slider** — with a typed value beside it, always.
- [ ] **4.6 Table** — the big one. Virtualised, sortable by any column with a
      total order so rows never reshuffle on equal keys, resizable columns,
      sticky header, row selection, per-row actions, empty state, and a filter.
      The current firmware table's flicker was exactly this missing.
- [ ] **4.7 Tree** — for regions and layer groups.
- [ ] **4.8 Tabs** — for panel groups within a view.
- [ ] **4.9 Splitter** — draggable, with a remembered position per view.
- [ ] **4.10 Panel frame** — title, actions, body, footer, and a pop-out
      affordance where the panel supports it.
- [ ] **4.11 Toast and status line** — transient messages that do not scroll
      away before they are read; a log of the last twenty.
- [ ] **4.12 Modal dialog** — confirm, form, and a long-running variant with
      progress and cancel.
- [ ] **4.13 Tooltip** — with a delay, and rich content where a value needs a
      unit and a caveat.
- [ ] **4.14 Menu bar and context menus** — keyboard navigable.
- [ ] **4.15 Progress** — determinate and indeterminate, with what it is doing
      and how to stop it.
- [ ] **4.16 Empty and error states** — every list and every panel has one, and
      it says what to do next rather than "no data".
- [ ] **4.17 Chart primitives** — axes, gridlines, legends, tooltips, and a
      shared time axis so stacked charts line up.
- [ ] **4.18 Map canvas host** — a widget that owns a GPU surface, handles pan,
      zoom, hit-testing and overlay composition.
- [ ] **4.19 Code and log view** — monospace, selectable, searchable, with
      follow-tail.
- [ ] **4.20 Keyboard shortcut system** — a single registry, discoverable in a
      shortcuts sheet, no conflicts by construction.

---

## 5. The views

Six views today. Each gets a designed layout rather than an accumulated one.

### 5.1 Plan — build and site
- [ ] **5.1.1** Map dominant, left. Node list and Inspector right, stacked.
- [ ] **5.1.2** Tool strip on the map: select, place, move, link, measure,
      boundary.
- [ ] **5.1.3** Import and Boundary as slide-over panels rather than windows,
      because they are used once and then dismissed.
- [ ] **5.1.4** Planning results as a bottom sheet over the map, so the answer
      and the geography are visible together.

### 5.2 Run — exercise and watch
- [ ] **5.2.1** Map with live traffic trails, centre. Events and Packet timeline
      along the bottom, sharing one time axis.
- [ ] **5.2.2** Schedule and Scoreboard right.
- [ ] **5.2.3** Transport controls promoted: play, step, speed, seed, and the
      simulated clock large enough to read across a room.
- [ ] **5.2.4** Live feed as a strip that can be collapsed to a single line.

### 5.3 Debug — why did that happen
- [ ] **5.3.1** Packet timeline top, waterfall below, both on the same time
      axis, with a shared cursor.
- [ ] **5.3.2** Link budget and Inspector right, following the selection.
- [ ] **5.3.3** Console docked bottom, with node tabs.
- [ ] **5.3.4** A selected packet drives every other panel: this is the view
      where selection is the primary verb.

### 5.4 Verify — is it still true
- [ ] **5.4.1** Compare left, Validate right, Scoreboard beneath.
- [ ] **5.4.2** Residuals against reality as the headline chart.
- [ ] **5.4.3** A/B and bisect controls given real estate rather than a corner.

### 5.5 Bench — compare configurations
- [ ] **5.5.1** Sweep definition left, Runs table centre, Matrix right.
- [ ] **5.5.2** Timelines and Experiment log beneath, collapsible.
- [ ] **5.5.3** Arm provenance surfaced: every run shows the build checksum it
      attached, because that is now recorded and was invisible before.
- [ ] **5.5.4** Export as a first-class action, not a menu item.

### 5.6 App — write a client
- [ ] **5.6.1** Companion bench centre, with the protocol decoded both ways.
- [ ] **5.6.2** Endpoint card: address, transport, attachment state, copy.
- [ ] **5.6.3** Fault controls grouped and labelled by what they simulate.
- [ ] **5.6.4** Console and Events beneath, filtered to the served node.

### 5.7 View machinery
- [ ] **5.7.1** Each view is a declared layout, not an accumulated dock state.
- [ ] **5.7.2** Per-view saved arrangement: splitter positions, which optional
      panels are open, density.
- [ ] **5.7.3** Reset a view to its preset, per view and globally.
- [ ] **5.7.4** Saved custom views: name, save, load, delete, and set as
      default. Parity with `view.save`, `view.load`, `view.list`, `view.delete`.

---

## 6. Every panel, with its features

Twenty-five panels, plus the chrome. Each box is the panel working, not the
panel existing. Items marked **(review)** were found missing by the plan review,
which is the tick list doing its job before the code was written.

### 6.1 Inspector
- [ ] Node identity, kind, position, height, board, radio, firmware role and version
- [ ] Regions and default scope, editable, with the `#` asymmetry handled
- [ ] `AllowAnyFlood` switch with its warning
- [ ] Emitter duty, for emitters only
- [ ] Position uncertainty shown, and propagated into any answer derived from it
- [ ] Live counters: sent, heard, relayed, duty
- [ ] Follows selection from map, table, events and packets
- [ ] Can become its own window
- [ ] **(review)** Multi-select: "Selected - N nodes" with apply-a-preset-to-all

### 6.2 Nodes
- [ ] Virtualised list of every node, kind swatch, filter by name or kind
- [ ] Multi-select, and the map highlights the selection
- [ ] Sort by name, kind, sent, heard
- [ ] Row actions: centre on map, open window, delete, place a link
- [ ] The 150-row cap replaced by virtualisation, so no cap is needed

### 6.3 Link
- [ ] Both directions, always, with the asymmetry visible rather than implied
- [ ] Path profile with terrain, Fresnel zone and the worst obstruction
- [ ] Margin against the demodulator floor at the current modulation
- [ ] Pick either end from the map or the table

### 6.4 Budget
- [ ] Every term: transmit power, feedline, antenna gain both ends, free space,
      diffraction, excess loss, noise figure, required SNR
- [ ] Waterfall chart of the budget, so where it is spent is visible
- [ ] Editable assumptions with the result updating live

### 6.5 Waterfall
- [ ] Spectrogram from captured IQ, GPU rendered
- [ ] Frequency and time axes with real units
- [ ] Click a feature to select the packet that caused it
- [ ] Colour scale selectable, with the dynamic range stated
- [ ] **(review)** Symbol view, and "capture now"

### 6.6 Packet timeline
- [ ] Per-node lanes, transmissions and receptions, at real airtime widths
- [ ] Collisions marked, with the cause on hover
- [ ] Shared time cursor with the waterfall and events
- [ ] Zoom to a burst, and back out to the whole run

### 6.7 Events
- [ ] Every event with its cause, virtualised
- [ ] Filter by kind, node, cause, time range
- [ ] Follow-tail, and jump to the packet or node an event names
- [ ] Export to NDJSON, matching `events.dump`
- [ ] **(review)** Time scrub: the slider rewinds the event view and the traffic
      map draws at scrub time, not at now
- [ ] **(review)** Row actions: "follow this message", "open both consoles"

### 6.8 Scoreboard
- [ ] Per-node counters, sortable, with the metric definitions on hover
- [ ] Reach, duty, collisions, deaf time, airtime
- [ ] Highlight the worst offenders rather than making the reader scan

### 6.9 Console
- [ ] Per-node tabs, monospace, searchable, follow-tail
- [ ] Type a line and see the reply, matching `console.type`
- [ ] The documented caveat surfaced: replies do not arrive while a sweep owns
      the clock, said in the panel rather than in a document
- [ ] Command history, and completion for the MeshCore CLI (new, in scope)

### 6.10 Schedule
- [ ] Scheduled sends: node, time, repeat, command
- [ ] Assertions: kind, node, thresholds, with a live pass or fail
- [ ] Stress ramp controls
- [ ] Saved with the project, matching the fixture format
- [ ] **(review)** Snapshot as baseline, and Compare against a baseline

### 6.11 Compare
- [ ] Two runs or two arms side by side, metric by metric
- [ ] Deltas coloured by whether the direction is good, with the control's own
      spread quoted rather than a remembered floor
- [ ] Jump to the first divergence

### 6.12 Sweep
- [ ] Arms, seeds, senders, scope, run length, message size
- [ ] Matrix preview: how many runs this will be, before starting
- [ ] Start, stop, and progress with an estimate
- [ ] Build provenance per arm, visible before the run

### 6.13 Runs
- [ ] Every run with its arm, seed, state, result and build checksum
- [ ] Re-run one, or one arm, without redefining the sweep
- [ ] Failure reason inline

### 6.14 Experiment log
- [ ] The narrative log, searchable, with timestamps
- [ ] Copyable, because it ends up in reports

### 6.15 Matrix
- [ ] Arms against seeds, cell coloured by the chosen metric
- [ ] Metric selector, and a legend that states units

### 6.16 Timelines
- [ ] Receptions per second per arm, stacked, on a shared axis
- [ ] Brush to a window and have the other panels follow

### 6.17 Configuration (bench)
- [ ] The experiment's constants, including firmware versions per role
- [ ] The warning that a missing version resolves to an unpublished build,
      shown before the sweep starts rather than after it fails

### 6.18 Validate
- [ ] The ADR-0015 chain: fetch reality, replay it, compare
- [ ] Residuals chart with the sign convention stated
- [ ] What was excluded and why
- [ ] **(review)** Calibration write-back: apply +X dB excess loss from N
      observations, remove calibration, and shadow mode - these write into the
      RF model and losing them would have been silent

### 6.19 Energy
- [ ] Battery and panel from the board profile
- [ ] The December question, at the node's latitude
- [ ] Duty against survival, as a chart rather than a verdict

### 6.20 Live feed
- [ ] Real traffic replayed into the scenario
- [ ] Source, rate, and what is being dropped
- [ ] Start, stop, and a clear indication when it is live

### 6.21 Import
- [ ] Source, URL, token; fetch, preview, commit with strategy
- [ ] Preview states what will change before it changes it
- [ ] Counts of dropped nodes and why: null island, no position, uncertainty
- [ ] Inference: run, results per region with holder counts, apply, and the
      applied count reported
- [ ] **(review)** The "Get going" onboarding block: infer boundary, get
      terrain, start MeshCore now
- [ ] **(review)** In-panel saved networks save/load/add, and the "what had to
      be assumed" disclosure

### 6.22 Boundary
- [ ] Search a place, accept, prune, with the chosen set unioning
- [ ] Outline drawn on the map before pruning
- [ ] Saved boundaries listed and loadable offline
- [ ] Margin control
- [ ] **(review)** Terrain download for the area with tile count and "download
      it anyway", and "infer from the loaded network"

### 6.23 Planning
- [ ] Bridge two areas, cover a gap, redundancy against a failure
- [ ] Candidate sites ranked, with the criterion stated
- [ ] Place a candidate directly from the result

### 6.24 Fleet
- [ ] A command to every node, or a filtered subset
- [ ] The region commands, with the bare-versus-hash spelling handled
- [ ] Which commands invalidate a run, said before sending
- [ ] Per-node replies collected and diffed

### 6.25 Companion bench
- [ ] Endpoint per companion: TCP or serial, address, copy, attachment state
- [ ] One click for a mesh and an endpoint
- [ ] Protocol decoded both directions: contacts, channels, messages, acks
- [ ] Fault injection: drop the client, inject a stray frame
- [ ] Raw frame view for when the decode is the thing in question
- [ ] **(review)** LAN exposure toggle with its loopback-versus-network warning,
      virtual serial device, "take it over", and "sync contacts"

### 6.26 Chrome (review)
- [ ] Machine load meter in the menu bar: CPU and GPU, because emulated nodes
      saturate cores silently
- [ ] Unified background-jobs indicator with cancel, and its `jobs` count in
      `ui.state`
- [ ] The honesty line in the menu bar: results are a best case

---

## 7. Windows and dialogs

Six window types exist today. Each is reviewed for whether it should be a
window, a panel, or a slide-over.

- [ ] **7.1 Node window** — per node, all six tabs: Console, Companion,
      Settings (per-node radio preset), Stats with Neighbours, Activity,
      Connect. Stays a window; it is the thing people put on a second monitor.
- [ ] **7.2 Packet window** — one packet, decoded, with its path and its
      failures. Stays a window, opened from anywhere a packet is named.
- [ ] **7.3 Firmware library** — becomes a **panel** in a new Firmware view or a
      slide-over, because it is a browsing task, not a reference-while-working
      task. Retains: filters, search, downloads with progress, import, delete,
      use-for-role, wipe storage, the board and native distinction.
- [ ] **7.4 Nodes table** — the whole network editable in one grid. Stays a
      window; it is a spreadsheet task.
- [ ] **7.5 Preferences** — a proper settings dialog, grouped, searchable,
      with every option from section 9.
- [ ] **7.6 Provisioning** — moves into Preferences as a section, since it is
      settings, with the generated command list previewed live.
- [ ] **7.7 Modal dialogs** — open project, save as, delete confirmation,
      export, terrain download, with progress and cancel where they are slow.
- [ ] **7.8 Shortcuts sheet** — a window listing every binding, generated from
      the registry so it cannot drift.
- [ ] **7.9 About** — version, build, ADR links, licences of embedded fonts and
      libraries.

## 8. Menus

Every existing menu item, kept or deliberately moved.

- [ ] **8.1 File** — open project, save, save as, saved networks, import a
      network, load replacing the scenario, export event log, quit
- [ ] **8.2 View** — the six views, saved views, save current view, reset view,
      density, theme, dock everything back, node windows
- [ ] **8.3 Simulation** — play, pause, step, reset, speed, seed, start firmware
      on every node, stop firmware, wipe node storage, capture to file, capture
      to Wireshark, stop capture
- [ ] **8.4 Repeaters** — fleet commands, provisioning, set every repeater's
      region from the study area, firmware library
- [ ] **8.5 Planning** — bridge, cover, redundancy, coverage from here, coverage
      from the selected node, gaps, terrain download and estimate for this area
- [ ] **8.6 Window** — pop out, dock back, float inside, reset this view, close
- [ ] **8.7 Help** — shortcuts, documentation, about, the honesty line about
      results being a best case

## 9. Settings

Everything in Preferences, grouped and searchable.

- [ ] **9.1 Provisioning** — set name, position, clock, region from study area,
      default scope, cap advert hops with its value, stagger boot
- [ ] **9.2 Simulation** — real firmware, step, autowarm link matrix, excess
      path loss, seed policy
- [ ] **9.3 Radio** — preset picker with all twenty presets, and per-node
      override
- [ ] **9.4 Energy** — enable, battery and panel defaults
- [ ] **9.5 Automation** — control socket on or off, MCP, socket path
- [ ] **9.6 Appearance** — theme, density, UI scale, font size, reduced motion
- [ ] **9.7 Map** — basemap layer, tile fetching, offline mode, layer defaults
- [ ] **9.8 Storage** — cache locations, node storage root, wipe, sizes on disk

## 10. The map, in detail

The single most important surface, and the one with the most overlays.

- [ ] **10.1** Basemap tiles, cached, with an offline mode that fails loudly
- [ ] **10.2** Nodes by kind, with selection and hover states
- [ ] **10.3** Links, weighted by margin, with direction shown
- [ ] **10.4** Traffic trails during a run, decaying
- [ ] **10.5** Coverage raster from a node, with a legend in dB
- [ ] **10.6** Boundary outlines and the study-area margin
- [ ] **10.7** Terrain shading and elevation source attribution
- [ ] **10.8** Labels with collision avoidance, so names stop overlapping
- [ ] **10.8a (review)** Antenna pattern overlay, rotated to the node's bearing
- [ ] **10.8b (review)** Region layer, on by default alongside links
- [ ] **10.8c (review)** Show and hide neighbours per node
- [ ] **10.8d (review)** Right-click context menu: place kind here, link from
      and to here, move, delete, provision this node, coverage from here, open
      window
- [ ] **10.9** Pan, zoom to cursor, fit to nodes, centre on a node
- [ ] **10.10** Click to select, drag to move, shift-drag to box-select
- [ ] **10.11** Measure tool with distance and bearing
- [ ] **10.12** Layer panel controlling every overlay
- [ ] **10.13** Scale bar, attribution, and the honesty line
- [ ] **10.14** Frame budget at real scale (311 nodes, 1,223 links on the
      shipped Scotland and Ireland fixture), 60 fps. **Spiked, and open**: a
      naive per-frame implementation measured ~24 fps on real hardware; batching
      every link into one draw call recovered to ~35 fps. Neither clears 60.
      Closing the gap needs viewport culling and path caching across static
      frames, both named as P3 sub-tasks below rather than left as a wish.
      See `spikes/SPIKE-RESULTS.md`.
- [ ] **10.14a** Viewport culling: do not build draw ops for links or nodes
      outside the visible region.
- [ ] **10.14b** Cache the compiled path across frames where the camera has not
      moved and the network has not changed; reissue only on invalidation.

## 11. Interaction and accessibility

- [ ] **11.1** Full keyboard navigation: every action reachable without a mouse
- [ ] **11.2** A shortcut registry with no conflicts, and a sheet that lists it
- [ ] **11.3** Focus visible at all times, and focus order sensible
- [ ] **11.4** Selection is one concept across map, tables, events and packets
- [ ] **11.5** Undo for destructive scenario edits: delete, move, prune. New -
      nothing like it exists today - and in scope.
- [ ] **11.6** Confirmation only where an action is irreversible and expensive
- [ ] **11.7** Copy: any value, any table, any log, as text or CSV
- [ ] **11.8** Colour is never the only channel: shape or text carries it too
- [ ] **11.9** Minimum hit targets, and a comfortable density that meets them

---

## 12. Control socket parity

**89 verbs** at last extraction: 88 in the dispatch switch plus
`session.journal`, which is handled before it. The review found the previous
hand-count of 87 was wrong, which is the argument for 12.9 in one sentence.

- [ ] **12.1 Session and simulation** — describe, journal, status, play, pause,
      step, run, reset, speed, seed, state, inject, quit. `sim.run` also
      switches the workspace to Run when starting from a standstill, and the
      parity test asserts that side effect, not only the response.
- [ ] **12.2 Nodes** — list, place, delete, move, select, regions,
      allow_flood, window
- [ ] **12.3 Building a network** — boundary set, accept, prune; import
      source, fetch, commit; infer run, result, apply; radio preset; map centre,
      zoom, fit, filter; tool set
- [ ] **12.4 Firmware** — installed, download, import, set, start, state,
      wipe, delete, console type, fleet send. (`loop.detect` is a parameter of
      the experiment verbs, not a verb; the old count included it.)
- [ ] **12.5 Experiments** — base, define, vary, seeds, senders, start,
      stop, state, results, compare, export, assert check
- [ ] **12.6 Companion** — connect, disconnect, send, raw, advert,
      add_channel, configure, state
- [ ] **12.7 Capture and evidence** — file, wireshark, events recent, dump,
      scoreboard, coverage start, clear
- [ ] **12.8 Projects and layout** — project list, open, save; view list,
      save, load, delete; workspace set; panel open, dock, pop_out; panels list;
      window open, close; ui state, scale
- [ ] **12.9** A parity test **generated from the dispatch switch**, driving
      every verb and asserting responses and side effects, run in CI
- [ ] **12.10 Verbs whose semantics change, named** — `panel.pop_out` and
      `panel.dock` are generic over all 25 panels today; under the new model
      every panel keeps the ability to become a window, so the verbs keep their
      domain (this supersedes "a small number of panels" in principle 4).
      "Float inside" is a third state the old model had; it is retired, and
      `ui.state`/`panels.list` gain a documented mapping for their `docked`,
      `own_os_window` and `popped_out` fields. `view.save`/`view.load` cannot
      load old ImGui dock state: existing saved views are discarded at cutover,
      stated in the release notes.

## 13. Testing

The current UI has almost none, because immediate mode makes it hard. Retained
mode makes it possible, and that is a large part of the value.

- [ ] **13.1** Unit tests for the state layer: no rendering involved
- [ ] **13.2** Widget tests for the component library, headless
- [ ] **13.3** Golden-image tests for the **component gallery** at both themes,
      with a tolerance, in CI under software Vulkan (lavapipe) - not whole
      views, which the plan itself predicts would become disabled noise
- [ ] **13.4** The parity script from 12.9, run against a real scenario
- [ ] **13.5** Frame budget test: the 311-node fixture at 60 fps, **after**
      10.14a and 10.14b land - on the spike's evidence it fails before them -
      failing CI if it regresses once it passes
- [ ] **13.6** A smoke test that opens every panel and every window and asserts
      nothing panics

## 14. Documentation

The docs site is 21 pages and every screenshot is of the current UI.

- [ ] **14.1** Re-capture every screenshot from the new UI
- [ ] **14.2** Update the guides where the interaction changes
- [ ] **14.3** A page on the design system, for whoever extends it next
- [ ] **14.4** ADR: why Gio, what was rejected, and the spike evidence
- [ ] **14.5** Update ADR-0005 and ADR-0019, which describe the old model

## 15. Packaging

- [ ] **15.1** Gio needs Vulkan development headers to build, and CI golden
      tests need software Vulkan (lavapipe); both into the CI image and the
      build instructions
- [ ] **15.2** Confirm the built binary does not need them at runtime
- [ ] **15.3** Size check: the Gio spike was 13 MB against the current 32 MB
- [ ] **15.4** macOS and Windows: Gio supports both; revisit the blockers, since
      the reason Windows was blocked was radioserver rather than the UI
- [ ] **15.5** Emoji font: found on the system, absent gracefully, documented

---

## 16. Sequencing

Nine phases. Each ends with something runnable, and the old UI keeps working
until the last one.

| phase | what | ends with |
|---|---|---|
| **P0** | Architecture: state layer, verbs off the frame thread | `meshbench test` running against the new state layer |
| **P1** | Design system and component library | A gallery binary showing every component in both themes |
| **P2** | Shell: window, views, splitters, menus, status, shortcuts | The six views, empty, switchable |
| **P3** | Map and its overlays | The map complete: every overlay, tool and interaction of section 10 |
| **P4** | Tables and lists: Nodes, Events, Scoreboard, Runs, firmware | Every table virtualised and sortable; Run view assembles |
| **P5** | Charts: waterfall, packet timeline, timelines, matrix, budget, energy | Every chart on shared axes; Debug and Bench assemble |
| **P6** | Remaining panels: Import, Boundary, Planning, Fleet, Validate, Compare, Schedule, Live feed, Companion bench | All six views complete, each against its section 5 list |
| **P7** | Windows, dialogs, settings, menus in full | Feature parity, section 4 fully ticked |
| **P8** | Tests, docs, packaging, cutover | Old UI deleted |

---

## 17. Risks

- **The map at 300 nodes and 2,000 stroked links is the unproven surface.** The
  waterfall is a per-frame texture upload and likely fine; the map goes through
  Gio's vector renderer, and the spike proved 58 nodes, not 300. Both are in the
  pre-P0 spike.
- **The screenshot pipeline is a P2 gate, not an afterthought.** Every phase is
  verified by scripted window captures over SSH on KDE Wayland. The current app
  is an XWayland client; Gio prefers native Wayland, where window-targeting
  capture behaves differently. Validate capture at P2, or pin the X11 backend,
  before any phase depends on screenshots for its evidence.
- **Screen readers are out of scope, and that is a property of the toolkit.**
  Gio has no accessibility tree. Keyboard navigation and visible focus (11.1 to
  11.3) are achievable by hand; a screen reader is not, and silence here would
  read as an accidental promise.
- **Tabular figures cannot be toggled** - Gio's text stack exposes no OpenType
  features - so 3.2.3 is satisfied by face choice: the mono face for aligned
  numerals, chosen in P1, not discovered mid-P4.
- **Golden-image tests are fragile** across drivers and font versions. Tolerance
  and a single CI image, or they become noise that gets disabled.
- **19,810 lines is a lot of behaviour**, and the inventory above is from
  reading the code, not from using it for a year. Something will be missing;
  the tick list is how it gets found early.
- **Gio's widget set is small.** Every component in section 4 is ours to build
  and to maintain. That is the trade for the control it gives.
- **Two UIs in the tree for months.** Mitigated by building in `internal/gui`
  and cutting over once, but the temptation to fix a bug twice is real.

## 18. Scope posture

Budget is explicitly not a constraint on this plan. The additions the review
flagged as cuttable - undo, CLI completion, label collision avoidance, the
selectable waterfall scale, searchable selects, three densities - stay in
scope. The only review cut kept is scoping golden-image tests to the component
gallery, because that one was about test fragility rather than money: view
level goldens rot into disabled noise whoever pays for them.

## Explicitly not in scope

- Docking as a general framework. Views are fixed arrangements plus real windows.
- Mobile or web targets, though Gio would allow both.
- Changing what any verb means.
- The simulation, the engine, the firmware integration, or the RF model.

---

## 19. What this costs

Estimated in my own working time on Opus, at the pace this session has actually
run: writing, building, running, screenshotting and fixing on the real machine.

Reviewed estimates. The first draft said 58 to 84 hours; the review priced
P0, P1 and P7 against what they actually contain and came out roughly 1.5x
higher, with reasons that survived checking. These are the corrected figures.

| phase | estimate | why |
|---|---|---|
| P0 architecture | 8 - 14 h | Splitting a 941-line App struct, re-pointing 1,644 lines of handlers, engine pacing out of the frame loop, and the real exit gate of 2.6a |
| P1 design system and components | 16 - 24 h | Twenty components in a toolkit that ships almost none; the table alone is 6-10 h; menus, tooltips and modals need an overlay system Gio does not provide |
| P2 shell | 3 - 4 h | Views, splitters, menus, shortcuts |
| P3 map and overlays | 8 - 12 h | Eighteen features after the review's additions |
| P4 tables and lists | 6 - 8 h | The component from P1 does most of it |
| P5 charts | 8 - 12 h | Waterfall is a texture upload; the rest share axes |
| P6 remaining panels | 10 - 14 h | Fifteen panels, mostly forms over existing state |
| P7 windows, dialogs, settings, menus | 10 - 16 h | The 686-line Preferences, the firmware library, the editable nodes table, the six-tab node window, the packet window - depth, not breadth |
| P8 tests, docs, packaging, cutover | 8 - 14 h | Golden infrastructure needs software Vulkan in CI; 21 pages of screenshots |
| kept additions (undo, completion, label avoidance, densities and the rest) | 8 - 14 h | In scope by decision: budget is not a constraint |
| **total** | **85 - 132 h** | |

**In sessions:** roughly 22 to 33 working sessions of the length this one has
been. Calendar time depends entirely on how often we sit down.

**Confidence:** medium on P1, P2, P4, P6, P7, which are breadth over known
ground. Lower on P3 and P5, where the frame budget is unproven. P0 could go
either way: it is the smallest phase and the one most able to embarrass the
estimate, because it is where the existing coupling gets discovered properly.

**The cheapest thing that would sharpen this estimate** is the two-surface
spike in section 20: half a day, and it retires both frame-budget unknowns.

## 20. The spike, run

Done, on real hardware (AMD RX 5700 XT, RADV, confirmed - not software
rendering) against the real 311-node, 1,223-link Scotland and Ireland fixture,
composited over a synthetic basemap so the test pays the real map's compositing
cost. Full numbers and method in `spikes/SPIKE-RESULTS.md`.

**The waterfall clears at 60 fps**, once measured correctly - the first attempt
conflated CPU-side synthetic spectrogram generation with the texture-upload
question actually being asked, and isolating them (a pre-rendered ring of
frames) settled it: this surface was never the risk.

**The map does not clear 60 fps**, and the review's instinct that it was the
likelier long pole was right. Naive per-frame drawing measured ~24 fps; batching
every link into one draw call - the standard first optimisation - recovered to
~35 fps. Real headroom, not close to a toolkit ceiling, but short of the target
on its own. Two named, ordinary levers - viewport culling and path caching
across static frames - are neither exotic nor spiked yet, and are now explicit
P3 sub-tasks (10.14a, 10.14b) gating the frame-budget test in 13.5 rather than
an assumption riding on an unqualified "if both hold."

**P0 starts on this evidence.** The map risk is real, bounded, and has a named
fix; it does not reopen the toolkit choice.

## 21. Review

Reviewed by Fable against the source before any code was written. Verdict:
approve with changes, all incorporated above - the corrected verb inventory
(12), the fifteen missing behaviours (marked "review" in 6 and 10), the real
P0 exit gate (2.6a), achievable phase gates (16), the 1.5x re-budget (19), and
the Gio-specific risks (17). The review's summary sentence earned its place
here: the plan's biggest stated fear - quietly losing the fourteenth thing
nobody remembered - is exactly what the review found, fifteen times over.
