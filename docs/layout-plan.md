# Layout plan: seven layers, and the two packages in the way

The code is fine. The *map* is not. This plan is about where things live and
what they are called, not about what they do.

Measured against `34bad7e`. Nothing here has been applied. Interactive version:
the Layout Explorer artifact, which walks the proposed tree and the import graph.

> **Earlier draft.** The first version of this document proposed renames and a
> few extractions inside `internal/session` and `internal/workbench`. That was
> too small to matter: it left `internal/` a flat listing of 38 sibling
> packages, which is the same complaint one level up. This version replaces it.

---

## 1. The actual problem

`internal/` has **38 packages as flat siblings**. Nothing in the listing says
which are physics, which are MeshCore, which are the application, which are
Gio. You cannot form a mental model from `ls`, so every navigation starts from
grep.

The oversized packages are real but secondary — source files only, since tests
must live beside their package and inflate the counts misleadingly:

| Package | Source files | Lines | Diagnosis |
|---|---:|---:|---|
| `internal/session` | 69 | 16,374 | `Sim` has 79 methods across 33 files |
| `internal/workbench` | 68 | 17,747 | already cohesive, just undivided |
| `internal/engine` | 24 | 8,234 | `Engine` has 89 methods across 21 files |
| `internal/gui/comp` | 24 | 5,618 | two packages wearing one coat |

Both `session` and `engine` are god-object packages: a method must live in its
receiver's package, so **neither can be split by moving files**. That is a
constraint on the plan, not a target of it.

---

## 2. Seven layers

Group by domain, and make the group order a strict dependency order:

```
rf  →  mesh  →  world  →  sim  →  study  →  app  →  ui
```

A package may import its own layer and everything beneath it, never anything
above. So `ui` can reach the physics; the physics cannot reach a widget.

```
internal/
  rf/           Radio physics. Knows nothing of nodes, networks or the app —
                you could compute a link budget with it and never say "MeshCore".
    channel/        ← internal/rf            sums waveforms, adds noise, decides nothing
    dsp/            ← internal/dsp
    gpu/            ← internal/gpu           + the .wgsl files, which must live here
    lora/           ← internal/lora
    antenna/        ← internal/antenna
    terrain/        ← internal/terrain
    environ/        ← internal/environ
    propagation/    ← internal/coverage {grid.go, pairs_cpu.go} + the Terrain interface

  mesh/         MeshCore itself: what a node is and what it says.
    firmware/       ← internal/firmware
    shim/           ← internal/firmware/shim
    companion/      ← internal/companion
    proto/          ← internal/companion/proto
    packet/         ← internal/capture {dissect.go, payload.go}
    console/        ← internal/console
    energy/         ← internal/energy

  world/        What is being simulated, and where it came from.
    scenario/       ← internal/scenario
    provider/       ← internal/provider
    mqtt/           ← internal/mqttclient
    boundary/       ← internal/boundary
    basemap/        ← internal/basemap
    sdr/            ← internal/sdr

  sim/          Running it, and recording what happened.
    engine/         ← internal/engine
    capture/        ← internal/capture {ledger.go, pcapng.go}
    replay/         ← internal/replay

  study/        The questions asked of a simulation.
    coverage/       ← internal/coverage {raster.go, combine.go}
    linkbudget/     ← internal/linkbudget
    planning/       ← internal/planning
    pathview/       ← internal/pathview
    validate/       ← internal/validate

  app/          Orchestration, no toolkit.
    state/          ← internal/gui/state
    session/        ← internal/session  (+ 7 subpackages, §5)
    fixture/        ← internal/fixture
    control/        ← internal/control
    mcp/            ← internal/mcp

  ui/           Gio. The only layer permitted a toolkit.
    theme/ comp/ shell/ desktop/ float/ pick/   ← internal/gui/*
    mapview/        ← internal/gui/comp {map*, tiles*, affine}
    workbench/      ← internal/workbench  (+ 6 subpackages, §5)
```

**`internal/` goes from 38 entries to 7.** That is the navigability win;
everything else in this document is detail.

---

## 3. It verifies — after two edits

Checked mechanically against `go list` output for all 40 packages:

```
UPWARD IMPORTS (violations of rf < mesh < world < sim < study < app < ui):
  rf     internal/gpu       ->  study  internal/coverage
  sim    internal/engine    ->  study  internal/coverage
  world  internal/provider  ->  sim    internal/capture
total violations: 3
```

Three violations, no others — and all three come from **two packages that are
each doing two unrelated jobs**. That fusion is precisely why the flat layout
resisted grouping.

### 3.1 `internal/coverage` is propagation maths *and* a study product

`internal/gpu` imports it for `HeightGrid`, `GridLossParams`, `GridLossCPU`,
`PairProfiles`, `ProfilePairLossCPU`, `NoDataLoss` — **the CPU twins of the GPU
kernels**, which CLAUDE.md requires to exist and be tested against each other.
That is physics. `internal/engine` imports it for exactly one symbol:
`coverage.Terrain`, a five-line interface.

Meanwhile `Raster`, `Compute`, `Cell`, `Options`, `Combine` are the coverage
*map* — an answer to a question, not a computation of physics.

Split along the existing file boundary:

| To | Files | Symbols |
|---|---|---|
| `rf/propagation` | `grid.go`, `pairs_cpu.go` | `HeightGrid`, `GridLossParams`, `GridLossCPU`, `PairProfiles`, `ProfilePairLossCPU`, `NoDataLoss`, `NoDataHeight`, `RasteriseHeights` |
| `study/coverage` | `raster.go`, `combine.go` | `Raster`, `Compute`, `Cell`, `Options`, `Endpoint`, `Combine`, `LossBetween` |

The `Terrain` interface is currently declared in `raster.go:19` and must move
**down** into `rf/propagation` — `grid.go` already takes it as a parameter, and
it is the only thing `sim/engine` needs. Leaving it in the study layer is what
makes the engine import upward.

### 3.2 `internal/capture` is packet dissection *and* recording

`internal/provider` imports it for exactly one symbol: `capture.Dissect`.

The two halves **share not one symbol** — verified: nothing in `ledger.go` or
`pcapng.go` references `Dissect`, `Field` or `Dissection`, and nothing in
`dissect.go` or `payload.go` references `Reception`, `Ledger`, `Outcome` or
`Pcapng`. It is two packages in one directory.

| To | Files | Symbols |
|---|---|---|
| `mesh/packet` | `dissect.go`, `payload.go` | `Dissect`, `Dissection`, `Field`, `PseudoHeader`, `RewritePath` |
| `sim/capture` | `ledger.go`, `pcapng.go` | `Reception`, `Ledger`, `Outcome`, `OutOfRange`, `NotDemodulated`, `Accepted`, `PcapngWriter` |

`sim/engine` needs both halves; both are then downward. `world/provider` needs
only `mesh/packet`, and the violation dissolves.

### 3.3 Result

With those two splits modelled, re-run:

```
total violations: 0
```

The seven-layer order holds across all 40 packages with **no other change** —
no interface extraction, no dependency inversion, no shuffling to make it fit.
The architecture was already there; it just had no directory to live in.

---

## 4. What keeps it this way

A layout that is not enforced decays back. The repository already has the
mechanism — `internal/session/boundary_test.go` fails the build if the session
imports a toolkit. Generalise it: one table of the seven layer names in order,
one test that walks `go list` and fails on any import pointing upward.

That converts the architecture from a description into a check, so the next
package lands in the right layer because the wrong one does not compile. It is
about forty lines, and it is the same idea as Chromium's `DEPS` files, Android's
no-upward-dependency rule, or the layering that hexagonal architecture describes
but rarely enforces.

The grouping itself is Go's own convention: the standard library is domain-first
and two levels deep — `net/http`, `crypto/tls`, `encoding/json` — not forty
packages in a row.

---

## 5. The two packages that are still hard

Layering fixes the top of the tree. Two packages are large for a different
reason and need their own work.

### `app/session` — 69 source files, `Sim` with 79 methods across 33 of them

Cannot be split by moving files. Two patterns, at very different prices:

- **Pattern A — extract a component.** The subpackage owns a type, `Sim` holds
  a pointer, methods move. `Sim`'s own field list names the candidates:
  `sdrServers`, `bench`, `gpuProbe`, `prefs`, `consoles`.
- **Pattern B — extract a pure function.** The subpackage exposes
  `func Something(ctx, w *state.World, …)`, and `Sim`'s method shrinks to a
  three-line call.

| Subpackage | Lines | Pattern | Notes |
|---|---:|---|---|
| `session/analysis` | ~1,180 | **B** | mostly reads `World` — **start here** |
| `session/environ` | ~1,010 | B/A | tile and building pulls |
| `session/warm` | ~580 | **A** | deletes 5 `Sim` fields wholesale |
| `session/experiment` | ~2,050 | **A** | already owns `ExpArm`, `ExpResult`, `SweepPlan` |
| `session/companionbench` | ~1,290 | A | owns `compSession` |
| `session/provision` | ~1,320 | A | owns `Provisioning` |
| `session/traffic` | ~1,315 | A | owns `sdrServer` |

**Do not extract the verbs.** The thirteen `verbs*.go` / `logic_*.go` files all
take `s *Sim`; moving them would mean exporting ~40 methods purely to cross a
package boundary. Rename them instead — a `verb_` prefix clusters all thirteen
in the listing for free, and the "write the interface at the *second*
implementation" rule says wait for a second front end.

### `ui/workbench` — 68 source files, already decomposed into types

This one is easy: `configPanel`, `packetPanel`, `nodeWindowPanel`,
`companionTab` each already own their methods. The enabling move is
`ui/workbench/panel` — `panelDeps`, `Do`, `callbacks`, `menuDeps` exported once
so the families can leave. Six-plus families exist, so it is overdue rather than
speculative.

Then: `packetview` (12 files, ~3,450 lines — the pilot), `nodeview`,
`companionpanel`, `experiment`, `library`.

Filenames are renamed regardless of whether the split happens, because they
currently encode plan milestones rather than content:

```
panels.go        → nodespanel.go
panels6.go       → schedulepanel.go, consolepanel.go
panels6b.go      → fleetpanel.go, boundarypanel.go, timelinespanel.go
panels6c.go      → configpanel.go, logpanel.go          (563 lines, clears the 500 limit)
events2.go       → eventspanel.go
tables.go        → scorepanel.go                        (holds only scorePanel)
tables2.go       → runspanel.go                         (holds only runsPanel)
actionpanels.go  → fleetcontrols.go, schedulecontrols.go,
                   planningcontrols.go, provisioningcontrols.go
livepanels.go    → benchcontrols.go, feedcontrols.go
networkpanels.go → importcontrols.go, boundarycontrols.go, validatecontrols.go
configcards.go   → configcards_{model,run,system}.go    (558 lines, clears the limit)
```

`P6` and `P4` are phases of a planning document; the `2`/`b`/`c` suffixes mean
"written later", which is the one fact git already records. Both `tables.go` and
`tables2.go` also carry doc comments describing contents they no longer have —
fix those in the same commit, or the misdirection just moves to a new filename.

### `sim/engine` stays whole

`Engine` has 89 methods across 21 of its 24 source files — the same shape as
`Sim`, and more concentrated. At 24 files it is not what makes the tree hard to
walk. Leave it; revisit only if it grows.

---

## 6. Deletions

Verified byte-identical duplicates and tracked artifacts. None is referenced by
any Go file, so removing them should be a build no-op:

- `internal/ux/` — 9 PNGs and a README, identical to `docs/ux/`. **No code.**
- `internal/output/` — 4 PNGs and a WAV, identical to `docs/output/`. **No code.**
- `shaders/` — identical to `internal/gpu/*.wgsl`. `//go:embed` cannot reach
  outside its own package directory, so this copy can never be the live one —
  and CLAUDE.md's layout section points at it.
- `engine.test`, `envgen`, `goldencap` — tracked binaries at the repo root.
  `envgen` and `goldencap` shadow the real packages at `tools/envgen` and
  `tools/goldencap`.

Also: `internal/mockup` is used only by `tools/mockup` and `tools/render`, never
by the application → `tools/internal/mockup`, where the path says so.

---

## 7. Constraints

Load-bearing. Two of them shape the plan.

**Release ldflags pin an import path.** `package.yml:100,518` set
`-X github.com/MeshBench/meshbench/internal/workbench.Version`. Moving the
package to `internal/ui/workbench` **requires updating both call sites in the
same commit**. A broken ldflag does not show up in `go test`.
`package.yml:68` pins `internal/workbench/licences/licences.json` too.

**CI shards tests by package path.** `ci.yml:116-151` shards
`./internal/session/...`, `./internal/workbench/...`, `./internal/dsp/...` and
excludes those three from the catch-all with `/internal/(session|workbench|dsp)(/|$)`.
All three move under this plan, so **the shard paths and the exclusion regex
must change with them** or the sharding silently collapses into one job.

**`//go:embed` cannot escape its package directory.** Settles `shaders/`, keeps
root `fixtures/` at the root, and keeps the `.wgsl` files inside `rf/gpu`.

**A method must live in its receiver's package.** The whole of §5.

**Work is in flight on this branch.** `internal/session` and `internal/engine`
are being actively edited. Anything touching them waits.

---

## 8. Sequencing

Ordered so each step is independently revertible, and so the wide mechanical
moves happen before the refactors that need thought.

| # | Step | Effort | Risk |
|---|---|---|---|
| 0 | Deletions and hygiene (§6) | minutes | none |
| 1 | Workbench filename renames (§5) | hours | none |
| 2 | Split `coverage` → `rf/propagation` + `study/coverage`; move `Terrain` down | half a day | low — file-aligned |
| 3 | Split `capture` → `mesh/packet` + `sim/capture` | half a day | low — zero shared symbols |
| 4 | Move all packages into the seven layers; update ldflags and CI shards | 1 day | low, very wide diff |
| 5 | Add the layer-enforcement test (§4) | hours | none — locks in 0–4 |
| 6 | `gui/comp` → `ui/comp` + `ui/mapview` | half a day | low |
| 7 | `ui/workbench/panel`, then `packetview` as pilot | 2–3 days | medium |
| 8 | `app/session` decomposition, starting with `analysis` | week+ | high — wants an ADR |

Steps 2–5 are the spine: they are what makes the tree navigable *and* keep it
that way. Do 0 and 1 first because they are free, then 2 and 3 because
everything else depends on them.

Step 4 is a very large diff touching almost every import line. Keep it in its
own commit with no logic changes — a wide boring diff reviews easily; mixed with
edits it does not.

---

## 9. Rules to add to CLAUDE.md

The existing rules are all about size, and none of these were size problems.

| Rule | Limit |
|---|---|
| Layer | every package sits in one of the seven, and imports only downward |
| Filename | says what the file holds — never a plan phase, never a `2`/`b`/`c` suffix |
| Panel & widget files | one type per file, named after the type |
| Package scope | one job — if two halves share no symbol, they are two packages |
| Duplicated asset | none — one copy, and the code points at it |
| Build artifacts | never tracked |

And replace the Layout section's table with the seven layers, noting that a new
package updates it in the same commit. `internal/session` grew to 69 files
partly unnoticed because it was never on the map at all.
