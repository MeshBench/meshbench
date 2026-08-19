# Layout plan: seven layers, and the two packages in the way

The code is fine. The *map* is not. This plan is about where things live and
what they are called, not about what they do.

Measured against `31d1330`.

> **Status.** Steps 0 and 1 have landed — the filename renames, the duplicated
> assets, and the tracked tool binaries. Everything from step 2 on is still a
> proposal.
>
> **Earlier drafts.** The first version proposed renames and a few extractions
> and nothing else, which left `internal/` a flat listing of 38 sibling packages
> — the same complaint one level up. The second added the seven layers. This one
> records what main has since done on its own, and what actually shipped.

## 0. What main did while this was being written

Three things landed between `34bad7e` and `31d1330` that this plan had called
for, and one that changes how it is enforced:

- **CI now checks the 500-line hard limit** on `.go`, `.wgsl` and `.lua`, with a
  `lint:file-length-exempt` escape hatch used exactly once (the Wireshark
  dissector, which Wireshark loads as a single file). The limit stopped being a
  convention the day that landed.
- **`41289b5` split six files back under it** — `waveform.go`, `mapchrome.go`,
  `session/engine.go`, `configcards.go`, `panels6c.go` and `main.go`. Each split
  is a seam, and the new names (`waveformbusy.go`, `mapscale.go`, `enginewarm.go`,
  `configinterface.go`, `configupdate.go`, `mainmap.go`) all say what they hold.
- **`eedb830` dropped the committed `engine.test` binary** and gitignored `*.test`.

Two consequences for this plan. `configcards.go` is now 379 lines and its name
is honest, so **the three-way split it proposed is withdrawn** — nothing is
wrong with it. And `session/engine.go` is now 332 rather than 502, so splitting
`type Sim` out of it is no longer forced by the limit; it is still worth doing,
for the reason in §5, but it is no longer urgent.

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

**`internal/geo` was the other prerequisite, and it is done (PR #102).** Cutting
this seam turned up two more upward references besides `Terrain`: `haversineKm`
and `bearingDeg`, declared in `raster.go` and called from `grid.go` and
`foldcpu.go`. Pulling on that found the same formula copied thirteen times, two
copies of it numerically worse than the rest. One implementation now, and the
seam has exactly one thing left to move across it.

`internal/coverage` has also grown `foldcpu.go` and `bestserver.go` since this
plan was written. `foldcpu.go` is another CPU twin the GPU calls and touches no
`Raster`, so it goes down with the kernels; `bestserver.go` produces a `Raster`,
so it stays. The generator that builds the explorer now fails loudly if a file
in either split package is unassigned, rather than quietly leaving it behind.

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

### Filenames, audited against contents

Every file's declarations were extracted and compared to its name. **33 files
carry a name that does not describe what they hold.** Three kinds:

**The package's central type, hidden in a file named for something else.**

```
session/engine.go   → sim.go + engine.go   502 lines, over the hard limit, and it
                                           declares type Sim — 79 methods, the whole
                                           point of the package. session.go is 13
                                           lines of package doc. A reader looking
                                           for Sim finds neither.
gpu/dechirp.go      → device.go + dechirp.go
                                           declares type Device, the package's
                                           central type, inside a file named for one
                                           of its four kernels.
```

**`logic_` is a layering prefix, not a topic — and all four files are undocumented.**
An earlier draft of this plan proposed renaming these to `verb_*`. That was wrong:
they are not verbs, they are the implementations the verbs call.

```
session/logic_bench.go     → benchserve.go    Sim.serve/endpoints/stopServing/dropClients
session/logic_importer.go  → importfetch.go   one function, ImportFrom
session/logic_observed.go  → observedpull.go  PullObserved + residualsOf
session/logic_planning.go  → routes.go        budgetChecker + routesBetween
session/manipulation.go    → armcomplaints.go armDidNotReachTheChip, armComplaints —
                                              experiment-arm diagnostics. The name
                                              suggests something is being changed.
```

Verb files take the **topic-first** form `planningverbs.go` already uses, so each
sorts beside the implementation it calls rather than beside unrelated verbs:

```
verbs.go → verbregistry.go   verbsbench.go → benchverbs.go   verbsclock.go → clockverbs.go
verbsbudget.go → budgetverbs.go   verbscoverage.go → coverageverbs.go
verbsimport.go → importverbs.go   verbsnodes.go → nodeverbs.go
verbssweep.go → sweepverbs.go     firmwarelib.go → firmwarelibrary.go
```

**One panel type per file, named after the type.** The workbench is regular —
each panel owns its methods — but the filenames do not say so:

```
panels.go        → nodespanel.go                    declares nodesPanel
panels6.go       → schedulepanel.go, consolepanel.go
panels6b.go      → fleetpanel.go, boundarypanel.go, timelinespanel.go
panels6c.go      → configpanel.go, logpanel.go
events2.go       → eventspanel.go
tables.go        → scorepanel.go                    doc says "the tables of P4";
                                                    the file holds one scorePanel
tables2.go       → runspanel.go                     doc says "installed firmware and
                                                    past runs"; holds only runsPanel
observed.go      → feedpanel.go, validatepanel.go   declares both
planning.go      → planpanel.go                     declares planPanel
importer.go      → importpanel.go                   declares importPanel
bench.go         → benchpanel.go                    declares benchPanel
fwlib.go         → firmwarepanel.go                 declares firmwarePanel
fwlibrow.go      → firmwarerow.go
actionpanels.go  → fleetcontrols.go, schedulecontrols.go,
                   planningcontrols.go, provisioningcontrols.go
livepanels.go    → benchcontrols.go, feedcontrols.go
networkpanels.go → importcontrols.go, boundarycontrols.go, validatecontrols.go
configcards.go   — withdrawn: 41289b5 took it to 379 lines and the name is honest
```

And the `node*` prefix hides which of three panels a file serves:

```
nodecompanion.go → companiontab.go        488 lines declaring companionTab, the
                                          package's most-implemented type at 25
                                          methods — while companionclient.go,
                                          companionradio.go and companiontcp.go
                                          already hold its methods under the right
                                          prefix. Only the file declaring the type
                                          breaks the pattern.
nodefirmware.go  → nodeviewfirmware.go    nodeViewPanel
nodeobserver.go  → nodewindowobserver.go  nodeWindowPanel
noderadio.go     → nodewindowradio.go     nodeWindowPanel
```

Also `tools/mockup/panels2.go` → `panelsnetwork.go` and `panels3.go` →
`panelsstudy.go` — the same digit-suffix habit outside `internal/`.

> **Underscore caution.** Go treats `name_GOOS.go` and `name_GOARCH.go` as build
> constraints. None of the suffixes above (`model`, `run`, `system`, `test`) is a
> GOOS or GOARCH value, so all are safe — but check any future one against that
> list before adding it.

### `sim/engine` stays whole

`Engine` has 89 methods across 21 of its 24 source files — the same shape as
`Sim`, and more concentrated. At 24 files it is not what makes the tree hard to
walk. Leave it; revisit only if it grows.

---

## 5b. What reading the files turned up

The renames above came from an audit rather than from the directory listing.
Three other things came out of it, and two of them are more serious than any
filename.

### The 500-line hard limit — now CI's problem, not a convention

All four files that breached it have been fixed: `waveform.go`, `configcards.go`
and `session/engine.go` by `41289b5` on main, and `panels6c.go` by the renames
above. **Nothing in the tree is over 500 lines except the Wireshark dissector,
which carries an exemption and says why in its own header.**

### Three packages are built and imported by nothing

Not by the application, not by tools, not by another package's tests.

| Package | Lines | |
|---|---:|---|
| `internal/replay` | 754 | turns observed traffic back into transmissions |
| `internal/validate` | 532 | compares predicted against heard |
| `internal/mqttclient` | 57 | the paho implementation of `provider.Subscriber` |

`mqttclient` is a deliberate plug point — `provider` declares the interface and
stays dependency-free — but nothing ever plugs it in. CLAUDE.md lists MQTT as one
of the optional feeds; today it is optional in the sense of absent.

Worse, **`internal/validate` and `internal/replay/validate.go` are parallel
implementations of the same thing.** Both declare `type Report`; both declare
`Compare(rx []provider.Reception, …)`. And the residual computation the workbench
actually runs is a *third* one — `session/validate.go` plus `residualsOf` in what
is currently `logic_observed.go`, neither of which imports either package.

That matters more than layout. `internal/validate`'s own doc comment says it is
"the only thing in the project that can tell you whether any of it is true", and
CLAUDE.md's honesty rules lean on exactly that. Three implementations means at
most one is being maintained.

**Decide per package: wire it up, or delete it.** CLAUDE.md already says dead code
is not kept — git remembers. What must not continue is all three sitting in the
tree looking like architecture.

### Comments written against a document nobody has

**29 files cite a plan milestone in their doc comment** — `P4`, `P6`, `(6.19)`,
`10.7`, `12.9`, `(6.25)`. Renaming `panels6c.go` does not help if its replacement
still opens with "Configuration and the experiment log (6.17, 6.14)". A milestone
reference is worse than no reference, because it looks like one.

**54 files of 150 lines or more open with no doc comment at all**, including
`gui/comp/mapchrome.go` (481), `session/gpuwarm.go` (474), `scenario/boards.go`
(459) and `mcp/tools.go` (457). CLAUDE.md asks comments to explain *why*; these
say nothing.

Neither is a layout problem, and neither blocks the moves. Both are exactly the
sort of thing that never gets its own ticket, so fold them into the rename
commits: when a file is touched, fix its comment.

---

## 6. Deletions

**Done.** All of these have been removed:

- `internal/ux/` — 9 PNGs and a README, byte-identical to `docs/ux/`. No code.
- `internal/output/` — 4 PNGs and a WAV, byte-identical to `docs/output/`. No code.
- `shaders/` — `//go:embed` cannot reach outside its own package directory, so
  this copy could never be the live one. By the time it was removed it had
  already rotted: `dechirp.wgsl` and `pairs.wgsl` had been deleted from it while
  `internal/gpu` kept its own, and the `coverage.wgsl` left behind **no longer
  matched** the one actually compiled. A comment in `internal/dsp/dechirp.go`
  still pointed at a path that had not existed for some time. That is the whole
  argument for the duplicated-asset rule, and it took nine months to bite.
- `envgen`, `goldencap` — tracked ELF binaries at the repo root, 3.4 MB and
  4.0 MB, shadowing the packages at `tools/envgen` and `tools/goldencap` so that
  tab-completing the name got you a stale binary. Removed and gitignored
  alongside `/workbench2`, with `/meshcoresim` added for the same reason.
  (`engine.test` was already dropped by `eedb830`, and `*.test` gitignored.)

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
| 0 | ✅ **Done.** Deletions and hygiene (§6) | minutes | none |
| 1 | ✅ **Done.** The workbench renames (§5), doc comments fixed as each file was touched | 1 day | none — `git mv` plus splits at type boundaries |
| 2a | ✅ **Done.** `internal/geo` — one great-circle implementation, not thirteen (PR #102) | half a day | none — prerequisite found while cutting the seam |
| 2 | Split `coverage` → `rf/propagation` + `study/coverage`; move `Terrain` down | half a day | low — file-aligned |
| 3 | Split `capture` → `mesh/packet` + `sim/capture` | half a day | low — zero shared symbols |
| 4 | Move all packages into the seven layers; update ldflags and CI shards | 1 day | low, very wide diff |
| 5 | Add the layer-enforcement test (§4) | hours | none — locks in 0–4 |
| 6 | Decide `replay` / `validate` / `mqttclient`: wire or delete (§5b) | half a day | **needs a decision, not a refactor** |
| 7 | `gui/comp` → `ui/comp` + `ui/mapview` | half a day | low |
| 8 | `ui/workbench/panel`, then `packetview` as pilot | 2–3 days | medium |
| 9 | `app/session` decomposition, starting with `analysis` | week+ | high — wants an ADR |

Steps 2–5 are the spine: they are what makes the tree navigable *and* keep it
that way. Steps 0 and 1 are done — they were free — so 2 and 3 are next, because
everything after them depends on them.

Step 1 shipped as 15 straight renames plus 8 splits at type boundaries, and was
verified by comparing every top-level declaration in `internal/workbench` and
`tools/mockup` before and after: **589 declarations, identical on both sides.**
Code moved between files; no signature changed, none was added, none was lost.
That check is worth repeating for steps 2, 3 and 4, which are the same kind of
move at a larger scale.

Step 6 is the only one that is not mine to decide. Three packages and 1,343
lines are either load-bearing or dead, and nobody can tell which by looking —
which is why it belongs in a layout plan at all.

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
| Doc comment | required above 150 lines, and never cites a plan phase or ticket number |
| Unreferenced package | wire it or delete it — never leave it looking like architecture |
| Duplicated asset | none — one copy, and the code points at it |
| Build artifacts | never tracked |

And replace the Layout section's table with the seven layers, noting that a new
package updates it in the same commit. `internal/session` grew to 69 files
partly unnoticed because it was never on the map at all.
