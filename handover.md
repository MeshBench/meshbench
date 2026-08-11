# Overnight run — 2026-08-10

What was built, what the review found, and what is left.

## The correctness fix

**Radio configuration is per node** (ADR-0022). The channel took spreading
factor, bandwidth, frequency and coding rate from one shared config, so nodes on
different presets decoded each other and airtime was wrong for every node off
the default — the same bytes occupy the air ~40× longer at SF12/62.5 than at
SF7/250. Out-of-channel energy no longer counts as interference. SDR observers
stay exempt: wideband is what they are for.

## Panels the specs asked for and the workbench did not have

Reading the UX Spec, Feature Catalogues and User Workflows — none of which had
been opened during implementation — turned up most of this list.

- **Packet timeline** (the time graph): one lane per node, transmissions as bars
  to airtime scale, receptions joined to the transmission that caused them,
  zoomable from whole-run to symbol level. Clicking a bar opens that packet.
- **Link budget waterfall**: every term in order, both directions side by side,
  with the asymmetric case spelt out. The table you already had is untouched —
  these are additions.
- **Per-edge diffraction losses** on the cut-through, labelled where they occur,
  with vertical exaggeration always stated.
- **Waterfall and symbol view**: baseband synthesised from the frames in flight,
  click a burst for peak bin, runner-up, and the capture verdict.
- **Coverage layers**: best server, gaps, redundancy — from the `Combine` that
  had been computing them all along with nothing drawing them.
- **Antenna patterns** on the map, rotated to bearing; **link lines coloured and
  dashed** by outcome.
- **Reception ledger**: the five outcomes as columns with a *why?* per row,
  including the two no firmware instrumentation could show.
- **Assertions and first divergence**: what turns a scenario into a regression
  test, plus a baseline snapshot to compare against.
- **Node dragging** with live recompute — workflow A's primary "what if".
- **Seed always visible and editable**; speed pinned to 1× with a stated reason
  while a companion client is attached.

Earlier in the same run: companions over IP with per-node ports, CoreScope-style
packet dissection, the HopReach repeater layout, pcapng capture with filterable
Wireshark fields, the planning window, scheduled traffic and a stress ramp.

## Bugs the UX review found

Worth listing because they were all invisible until the screenshots:

- Vertical exaggeration was computed inverted and read **×0** on every profile.
- The seed field showed **0** rather than the seed in use.
- Adding a control strip at the top of the map **collapsed the map entirely** —
  positioned overlays leave the layout cursor where they finish, so it is now
  restored explicitly after them.

## Records

**ADRs written**: 0022 per-node radio · 0023 downloaded firmware and "a node is
a node" · 0024 the ledger records causes not absences · 0025 GPU measured and
deferred · 0026 companion ports · 0027 the instrument panels.

**Closed**: MSIM-11, 14, 18, 23, 25, 31, 37, 42, 43, 46, 47, 49.
**Raised**: MSIM-46 to 50, from the forgotten specs. **Reopened**: MSIM-36, the
MCP server, with its real intent recorded.

## Left

- **Live-session MCP socket** (MSIM-36) — designed on the ticket, not built. The
  largest single remaining piece.
- **The validation chain** (MSIM-27, 28, 29, 33): replay real traffic, compare
  against observed receptions, calibrate, shadow mode. **MSIM-28 is the one that
  makes the RF model falsifiable at all**, and it is untouched.
- **A/B firmware split and bisect** (MSIM-48). Assertions and divergence are
  built; splitting versions across nodes and automating the search are not.
- **External interference** (MSIM-20): "custom emitter" still places a repeater.
- **MQTT** (MSIM-32) skeleton; provider capabilities and adapters (MSIM-30).
- **Firmware breadth**: Windows/macOS/32-bit/arm64 (MSIM-44), MeshCore ≤ v1.15
  (MSIM-45), debug and SoftDevice-free variants (MSIM-39).
- **Docking and workspace presets** (MSIM-10 remainder) — the spec wants Plan /
  Debug RF / Firmware / Compare layouts.
- **MSIM-15, the licence.** Yours. Blocks going public and blocks reusing
  HopReach's optimiser design for MSIM-35.

## Verified

Build, `gofmt`, `go vet`, `golangci-lint` **0 issues**; full suite green; the
live firmware flood test passing; screenshot-confirmed running. The UI cannot be
type-checked on the VM (no GL headers) — elite is the arbiter.

Still unexercised headlessly, worth ten minutes by hand: the **companion socket**
with a real client attached, and **detachable node windows** on a second monitor.
