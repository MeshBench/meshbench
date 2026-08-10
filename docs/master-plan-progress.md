# Master plan — progress

Live tick list for `master-plan.md`. Updated as work lands; every tick is
verified on elite (build, lint, tests) before it is ticked.

## Phase 1 — Workspaces
- [x] Dockspace replaces the fixed layout; every panel is an imgui window
- [x] Panel registry (name → draw), Windows menu generated from it
- [x] Four workspace presets: Plan / Run / Debug RF / Firmware (Ctrl-1..4)
- [x] Layout persisted per workspace in the config file
- [x] Detach affordance on every panel, placing it outside the main window
- [x] Run-control strip always visible (run/pause/step/speed/firmware)

## Phase 2 — Import becomes a tool
- [x] Provider interface + registry (ADR-0016), Health() per source
- [x] Import window: source list from registry, health shown
- [x] Preview-then-commit: counts, bounding box on map, names before touching the scenario
- [x] Merge strategies: add-only-new (pubkey→name) / replace-matching / replace-all
- [x] Inference folded in ("read the traffic too") with the tick-boxes
- [x] Load panel removed; projects/saved networks in the File menu

## Phase 3 — Sidebar becomes an inspector
- [x] Inspector shows selection only; one component reused by node windows
- [x] Multi-select with bulk edit of the shared subset
- [x] Node filter moves to the map (type-to-filter highlight)

## Phase 4 — Placement fixes
- [x] Boundary path → Import + Boundary windows only
- [x] Speed control → run strip; Traffic tab slimmed (strip is Phase 1's, tab dissolved)
- [x] Coverage controls in one Plan panel (picker + opacity + clear together)
- [x] Fleet quick-commands grouped: Flood / Regions / Radio / Info
- [x] Stress test → Compare context (new Compare panel, docked in Run)
- [x] Configuration split: Preferences vs Provisioning

## Phase 5 — Workflow polish
- [x] Post-import strip: boundary / terrain / firmware offers with costs
- [x] Right-click: provision, follow message, open-both-consoles additions
- [x] Jobs popover: one place for warm/coverage/import/inference progress + cancel
- [x] Empty states name the next action (auditing every panel)

## Phase 5b — Live feed (added 2026-08-10)
- [x] First-hop ingest: CoreScope polling + MQTT streaming, hop 0/1 copies only
- [x] Matched by node *name* (keys can never match), injected at the origin or first repeater
- [x] Frame path rewritten to the sim's own hashes, padded to ≥2 bytes (less flooding/collisions)
- [x] Live feed panel: start/stop, counts, skip reasons; 1x lock; jobs-popover row

## Phase 6 — Ranked debt
- [x] ADR-0015: replay observed traffic against the model, residuals published (Validate panel)
- [x] ADR-0015: calibration from residuals (excess-path-loss term, ≥30 samples); shadow mode
- [x] ADR-0011: energy in the workbench (draw from real airtime, survive-December,
      DEM sun-path horizon; behind a Preferences toggle, off by default)
- [ ] ADR-0012: real emitter objects + per-receiver floor in the budget
- [x] ADR-0007: NDJSON event log; Wireshark live pipe (named pipe, same pcapng stream)
- [x] A/B splitter + bisect over assertions/divergence (Compare panel; BisectNodes)
- [x] ADR-0008 regression: re-pin speed on companion attach (Attached() through the pipe stack)
- [ ] GPU coverage kernel when >40 stations (ADR-0025)
