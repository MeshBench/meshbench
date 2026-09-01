# MeshBench manual QA checklist

Human testing, on a fresh machine, of everything a release is meant to do. It
exists because CI proves the code runs, not that the product *works* — a window
that opens to the wrong thing, a button that does nothing, a board that boots to
a black screen all pass every unit test in the tree.

## How to use it

1. Start from **a fresh machine or a fresh user account** — no Go toolchain, no
   caches, nothing this project has ever written. That is the state a first user
   is in, and half the bugs here only exist in it.
2. Work top to bottom. Each check is a numbered row with the steps to run and
   what should happen. Put a result in the table: **P** (pass), **F** (fail),
   **B** (blocked — could not run it), or **—** (not applicable to this
   platform).
3. When something fails, write what actually happened in the notes, with a
   screenshot where it is visual. **One failed row becomes one ticket** (see
   [Filing tickets](#filing-tickets)); the row number goes in the ticket so the
   fix can be checked off against it.
4. Record the build and platform in [Run header](#run-header) before you start,
   so a result months from now still says what it was a result *for*.

A check is only as honest as the state it ran in. If you had to install
something, clear a cache, or already had the app open, say so in the notes —
that is often the finding.

## Run header

Fill this in per pass; copy the block for each machine.

| Field | Value |
|---|---|
| Build (release tag or commit) | |
| Platform (OS + version) | |
| Machine (CPU, RAM, GPU) | |
| Fresh account / machine? | |
| Tester | |
| Date | |

---

## A. First run, on nothing

The release must run with nothing else installed — that is the whole promise on
the tin.

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| A1 | Download and launch | Download the build for the platform from the latest release; run it (Linux: `chmod +x meshbench-*.AppImage && ./meshbench-*.AppImage`). | A window opens with no prior install, no terminal errors that stop it. | | |
| A2 | Opens on a real network | Watch the first frame. | It opens on the Scotland network (~161 nodes) with hillshaded terrain underneath, not a blank map. | | |
| A3 | Play | Press **Play**. | Simulated time advances; adverts propagate between nodes; the clock and counters move. | | |
| A4 | Explain a link | Click two nodes. | Both link margins appear (each direction), plus the terrain cut-through between them. | | |
| A5 | Signing caveats | On macOS/Windows, follow `docs/install.md`'s signing steps. | The documented steps actually get it open; note any that are wrong or missing. | | |
| A6 | Second launch | Quit and relaunch. | It reopens without re-downloading terrain (the cache is used) and without complaint. | | |

## B. Plan a network

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| B1 | Coverage raster | Open a node's coverage. | A coverage raster draws over terrain, not a flat disc — it follows the ground. | | |
| B2 | Link margin, both directions | Pick a marginal pair. | Two margins, one per direction, and they can differ — reachability is asymmetric. | | |
| B3 | Terrain cut-through | Open the path view for a pair. | The Fresnel zone and each diffracting edge's own loss are shown, in both directions. | | |
| B4 | Where the next node goes | Run planning / next-node search. | It proposes a location and says why (what it would cover). | | |
| B5 | Import a live network | Import from CoreScope or Beacon. | Real nodes appear with positions; a node imported at low confidence is shown as uncertain, not as a confident point. | | |
| B6 | Position uncertainty | Inspect an imported node's answer. | A node with ±km position does not get a single confident margin. | | |

## C. The physics, and being honest about it

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| C1 | Waterfall | Open the waterfall on a busy moment. | A spectrogram of the band, with signals visible against noise. | | |
| C2 | IQ and symbols | Open the IQ / dechirped symbol view during a collision. | Two overlapping frames are visible; which one captured is shown, not asserted. | | |
| C3 | Capture, not a rule | Force two nodes to transmit at once. | Sometimes one is decoded and one is not — capture is emergent, never "both fail". | | |
| C4 | RF mode | Switch between calculated and waveform RF. | Reception is decided by link budgets in one and by the full receive chain in the other; the UI says which. | | |
| C5 | Realism switches | Toggle oscillator error, multipath, fading, etc. | Results change; with everything off, the UI still says the simulator is kinder than the air. | | |
| C6 | Honesty about limits | Look for where the model's shortcomings are stated. | The UI / `docs/shortcomings.md` says what is not modelled; nothing implies the sim is the real air. | | |

## D. Firmware: real MeshCore, real boards

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| D1 | Run a dev checkout | `meshbench dev` pointed at a MeshCore checkout. | The workbench runs the built firmware, not a reimplementation. | | |
| D2 | Published image boots | Import a published `.uf2` / `.bin` for a supported board. | It boots under QEMU or Renode; the board comes up. | | |
| D3 | Board matrix | Open the board capability matrix. | It says which boards have been watched doing what — and does not claim more than has been seen. | | |
| D4 | Firmware A/B | Put half the repeaters on one build, half on another, same traffic. | Both run; the difference is shown as a diff, not a guess. | | |
| D5 | Import: refuse the un-bootable | Import an app-only `.bin` (no bootloader) for an ESP board. | It is refused with a reason naming the merged/factory image, not accepted to fail minutes later. | | |
| D6 | Import: name and role | Import a build, name it, choose a role. | The library shows the name given, not a bare timestamp; the role offered is one an emulator can run. | | |
| D7 | Airtime agrees | Compare a frame's airtime to firmware expectation. | Channel airtime matches the firmware's own `getEstAirtimeFor()` — no silent desync. | | |

## E. Driving an emulated board

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| E1 | See the screen | Open a running board's display. | The board's screen is shown (or captured as a PNG), matching what the firmware drew. | | |
| E2 | Buttons and touch | Press a button / touch the panel. | The firmware reacts (screen wakes, menu moves); a held press differs from a tap. | | |
| E3 | Serial and logs | Open the board's Output (serial, emulator, radio). | Each stream shows the right thing; serial shows the firmware's own console. | | |
| E4 | Console input | Type a command at an emulated board's console. | It reaches the firmware and is answered, not swallowed. | | |
| E5 | Radio readout | Open the node's Radio. | The radio configuration the model assumes, and, for a running node, what it reports and where they differ. | | |

## F. A companion app against the mesh

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| F1 | TCP companion | `meshbench serve` exposing a node over TCP; connect a client (e.g. `meshcore-cli`). | The client speaks the real companion protocol and cannot tell it from a radio. | | |
| F2 | Serial pty | Serve over a serial pty; point a client at it. | Same protocol reaches the client over the pty. | | |
| F3 | Bluetooth | Serve over Bluetooth (where supported). | A companion connects; where BLE is not available it is refused with a reason, not silently. | | |
| F4 | Distance | Put nodes and a hill between the app and the far end. | The app still works, and the path is what a real one would be. | | |

## G. Scripting and headless

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| G1 | Attach a client | With the workbench open, connect the Python or Go client. | It connects over the control socket and `hello` reports the mode. | | |
| G2 | Build a mesh from a script | Place nodes, run, read back stats. | The scripted network matches what was asked for. | | |
| G3 | Headless | Start a headless session (no window). | It runs with no display and answers the same verbs. | | |
| G4 | Be told, not poll | `subscribe` to status/snapshot; do something. | Notifications arrive as things change; a client that never subscribes sees no difference. | | |
| G5 | Checkpoint / restore | `checkpoint` a running session; blank it; `restore`. | The network and the moment come back; a long run's restore is shown replaying, not hung. | | |
| G6 | The published Python client | On a machine with no checkout: `pip install meshbench`, then run the README's first snippet. | It installs, imports and drives a workbench; `meshbench.__version__` is the release's version, not an older one. | | |
| G7 | The published Node client | On a machine with no checkout: `npm install @meshbench/client`, then `Workbench.attach()` against a running workbench. | It installs and attaches; the installed version is the release's version. | | |

## H. Repeatable tests and interop

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| H1 | Run a fixture | `meshbench test <fixture>` on real firmware. | It runs the fixture and checks its assertions, pass/fail, suitable for CI. | | |
| H2 | Determinism | Run the same seed and scenario twice. | Identical result both times. | | |
| H3 | Wireshark live | Open the live UDP dissector while traffic flows. | Every receiver's view of every frame appears in Wireshark. | | |
| H4 | pcapng | Save a capture and open it in Wireshark. | The saved pcapng opens and dissects. | | |
| H5 | SDR export/stream | Export IQ or stream it to an unmodified SDR client. | The client sees the simulated band. | | |

## I. Persistence, offline, and teardown

| # | Check | Steps | Expected | Result | Notes |
|---|---|---|---|---|---|
| I1 | Save and reopen | Save the current setup; quit; reopen it. | The whole study comes back — nodes, boundary, margin, sends, assertions — not just the nodes. | | |
| I2 | Board memory persists | Configure an emulated board, stop, start again. | It keeps what it was told between runs, as hardware does; a wipe resets it. | | |
| I3 | Offline terrain | With terrain cached, disconnect the network; open a cached area. | It works offline; an *un*-cached area fails loudly, not silently wrong. | | |
| I4 | Clean teardown | Quit while a run and emulators are live. | Everything stops; no orphaned QEMU/Renode/radio processes are left behind. | | |
| I5 | Disk honesty | Check what was downloaded. | The app can say what it cached and what it cost the disk. | | |

---

## Feedback summary

One line per failed or doubtful row, so the whole pass reads at a glance and
each line becomes a ticket.

| Row | Severity (blocker / major / minor / polish) | What happened | Ticket |
|---|---|---|---|
| | | | |

## Filing tickets

- **One row, one ticket.** Put the row number (e.g. `E2`) and the build in the
  ticket, so the fix can be verified against the exact check that found it.
- **Say the machine.** A bug that only happens on a fresh account, or only
  without a GPU, or only on Windows, is a different bug from one that happens
  everywhere — the QA header is half the report.
- **Attach what you saw**, not what you expected: the screenshot, the terminal
  line, the orphaned process. The expected result is already in the row.
- Label by severity from the summary table. A blocker is *cannot get past it on
  a fresh machine*; polish is *works, looks wrong*.

Once tickets exist, they are the backlog; this document goes back to all-empty
for the next build.
