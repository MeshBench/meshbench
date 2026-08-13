# The menus, redesigned

From the UX design (2026-08-14): every dropdown gains section headings, icons
and shortcuts, and the Window menu stops being a wall.

## Groupings, exactly as designed

- **File**: *Open & Save* (Open a saved network Ctrl+O, Save this network
  Ctrl+S, Save this run Ctrl+Shift+S) · *Import & Export* (Firmware library,
  Import a live network, Export the event log) · *Exit* (Quit Ctrl+Q).
- **View**: *Overview* (Nodes running, Companion bench) · *Diagnostics*
  (Experiment log, Configuration) · *Preferences* (Settings).
- **Simulation**: *Control* (Play or pause Space, One step, Back to the
  start) · *Nodes* (Start firmware on every node, Wipe every node's memory) ·
  *Tools* (Originate a packet, Capture the waterfall, Capture to a pcapng
  file).
- **Repeaters**: *Commands* (Send a command to the fleet) · *Boot* (What they
  are told at boot) · *Analysis* (Coverage from the selection).
- **Planning**: *Tools* (Routes between two selected nodes, Boundary).
- **Window**: the generated panel list becomes scrollable with a
  "Show all panels..." overflow row, one icon per panel.
- **Help** unchanged.

## Principles the design states, kept as acceptance criteria

Group by purpose (headed sections); scannable (icons + consistent spacing);
consistent (same order and group structure across menus); action first (the
most common action tops each menu); Window menu is one list with show-all
for overflow.

## Implementation notes

- shell.MenuItem grows Section, Icon and Shortcut fields; menuDrop draws
  headed groups with a faint rule between, icons from small drawn glyphs
  (the transport's approach - no icon font).
- Shortcuts: register Ctrl+O/S/Shift+S/Q and Space on the shell's key filter,
  dispatching the same actions as the entries; show the binding right-aligned
  in the row, as the design does.
- workbenchMenus() stays the single table the tests read; sections are data,
  so TestEveryMenuItemReachesSomething keeps passing untouched.
- The Window menu keeps generation from the panel registry; the overflow row
  opens the Choose overlay listing everything.
