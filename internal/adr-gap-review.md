# ADR gap review — 2026-08-10

Every MSIM ADR read end to end (exported from Plane to
`scratchpad/adrs/`), compared against the code as it stands. One verdict per
ADR: **done**, **partial** (with exactly what is missing), or **not started**.
ADR-0021 is referenced from code (basemap terms) but has no page — it was never
written; the reference should either get a page or lose the number.

## Done

- **ADR-0001** Go + Dear ImGui desktop app — with the docking/workspace caveat
  under 0027 below.
- **ADR-0002** Real MeshCore natively via the Radio seam.
- **ADR-0003** Sample-accurate baseband; sensitivity verified against Semtech.
- **ADR-0005** Terrain + diffraction (Bullington + P.526 rework).
- **ADR-0006** Virtual antennas: patterns, rotation, per-direction gain, map
  overlay.
- **ADR-0010** Board profiles ×7; native backend. (Emulated: see 0009/0040.)
- **ADR-0013** GeoJSON region, RF margin, clipped import/terrain — plus the
  Boundary window and inference the ADR never asked for.
- **ADR-0019** Boundary search/multi-select; terrain estimate-then-confirm.
- **ADR-0020** Prebuilt firmware first (catalogue, direct-URL fetch), import
  second (`msim firmware import`), build third (`build.sh`).
- **ADR-0022–0027** — written this run, describing what was built.

## Partial

- **ADR-0004 GPU/CPU twins** — dechirp has its GPU twin and equivalence test.
  Coverage rasters (the declared next kernel, 0025) remain CPU; fine at ≤40
  stations, not at 400.
- **ADR-0007 Observability** — pcapng + pseudo-header + Lua dissector +
  filterable fields: done. In-app waterfall/symbol view: done. **Missing:**
  live capture over a pipe to Wireshark; SigMF/cf32 export is headless-only
  (no UI arm/bound control); **structured NDJSON event log** for grep/diff;
  live IQ streaming over TCP.
- **ADR-0008 Companion interfaces** — TCP + virtual serial (PTY), byte-clean,
  exclusive claim: done. **Missing:** emulated BLE (spike MSIM-9 said probably
  not worth it — decision stands unless MeshCIM needs it); auto speed-pin works
  but only for links opened via the UI map (companion links opened while a
  client is attached hold 1×; nothing re-pins on *attach* of an already-open
  listener — the `onAttach` hook was dropped in the pipe rewrite).
- **ADR-0009 / MSIM-13 firmware library** — catalogue + per-node role/version +
  cached list: done. **Missing:** build-from-git-ref with commit-hash
  attribution; keep-or-wipe-identity choice *per node* on swap (global wipe
  only); A/B split UI (assertions + divergence exist; the odd/even splitter and
  bisect do not).
- **ADR-0011 Energy** — headless `msim energy` exists. **Missing in the
  workbench entirely**: no battery/solar panel, no per-node draw from actual
  firmware airtime, no "does it survive December" view, no terrain-horizon sun
  path. The node inspector shows nothing about power.
- **ADR-0012 Interference** — the noise floor takes a figure, and "custom
  emitter" places a *repeater*. **Missing:** a real emitter object (duty cycle,
  bandwidth, spectrum), propagation of emitters through the same terrain
  engine, Ofcom import, per-receiver floor elevation shown in the link budget.
  Receive filters (0012 stretch) blocked behind it.
- **ADR-0014 Control plane** — virtual UART, multi-console, fleet commands,
  timestamps, RF-vs-firmware gap: done. **Missing:** layer 3 instrumented
  builds (MESH_DEBUG) with the distortion warning; the structured event log
  shared with 0007.
- **ADR-0015 Replay + validation** — **not started in any meaningful sense.**
  `internal/replay` exists unwired; no observed-vs-simulated comparison, no
  residuals, no calibration (MSIM-28/29), no shadow mode (MSIM-33). This is
  the falsifiability ADR — the model still cannot be contradicted.
- **ADR-0016 Providers** — CoreScope (nodes, packets, regions), Beacon, file:
  done as concrete types. **Missing:** the declared-capability interface,
  Health(), Poll→Live / Live→History adapters, MQTT beyond a skeleton, and a
  provider registry the UI enumerates instead of a hardcoded combo.
- **ADR-0017 Coverage + planning** — single-node + best-server/gap/redundancy
  overlays, bridge and cover-area tools: done. **Missing:** GPU rasters at
  scale (hard cap 40 stations today, silently-ish), site-search optimiser,
  antenna/radio optimisation (licence-blocked on MSIM-15 for HopReach reuse),
  KML/plan export-and-share.
- **ADR-0018 MCP** — headless tools + live-session socket + 10 session tools:
  done. **Missing:** the Claude *skill* that teaches workflows; `list_scenarios
  / load_scenario / compare_runs` style tools (projects exist now, so these are
  cheap); provenance block (seed/firmware/zoom + kinder-than-air note) is not
  yet in *every* session-tool response.

## Not started

- **Emulated↔native diff run** (MSIM-40) — the 0010 cross-check has machinery
  and no published result.
- **Windows/macOS/arm builds** (MSIM-44), **MeshCore ≤ v1.15** (MSIM-45),
  **debug + SoftDevice-free builds** (MSIM-39).
- **MSIM-15 licence** — Alex's decision; blocks public release and HopReach
  optimiser reuse.

## The two structural findings

1. **The validation chain (0015) is the largest broken promise.** Everything
   else makes the instrument richer; only this makes it *right*. It should
   outrank every remaining feature.
2. **The UX has outgrown its layout.** Twenty-odd features have been bolted
   onto a single window designed for four. That is the subject of
   `docs/master-plan.md`.
