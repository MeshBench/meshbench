# Layout plan: making the tree navigable again

The code is fine. The *map* is not. This plan is about where things live and
what they are called, not about what they do.

Written against `7eb0a23` on `msim-waveform-truth`. Nothing here has been
applied.

---

## 1. What is actually wrong

The complaint is "too flat", and the measurement agrees, but the flatness is
concentrated in three places rather than spread evenly. Source files only —
`_test.go` excluded, because tests must live beside their package and inflate
the counts misleadingly:

| Package | Source files | Lines | Diagnosis |
|---|---:|---:|---|
| `internal/session` | 69 | 16,374 | one god struct, 33 files of methods on it |
| `internal/workbench` | 68 | 17,747 | already cohesive, just undivided |
| `internal/gui/comp` | 24 | 5,618 | two packages wearing one coat |
| `internal/engine` | 24 | 8,234 | **fine** — 28 of its 52 files are tests |
| `internal/firmware` | 15 | 4,938 | fine |

`internal/engine` looked like the third-worst offender at 52 files and is not a
problem at all: 13 of those are `_live_test.go` in `package engine_test`. Worth
saying out loud so effort does not go there.

Beyond size, five separate defects make the tree harder to read than its size
alone would explain.

### 1.1 Filenames encode plan milestones, not content

This is the worst of them, because it defeats every navigation method at once —
you cannot guess the name, `grep` it, or learn it.

```
internal/workbench/panels6.go     "Three of P6's panels"
internal/workbench/panels6b.go    "Three more of P6's panels"
internal/workbench/panels6c.go    "Configuration and the experiment log (6.17, 6.14)"
internal/workbench/tables.go      "The tables of P4, on the virtualised component from P1"
internal/workbench/tables2.go     "The other two tables of P4"
internal/workbench/events2.go     "The events panel, redesigned"
```

`P6` and `P4` are phases of a planning document. Six months from now nobody will
know what P6 was, and the file will still be called `panels6b.go`. The `2`/`b`/`c`
suffixes mean "written later", which is the one fact git already records.

The underlying shape is admirably regular — **one type per panel, each with
`Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot)`** — so the fix is
mechanical. `panels6b.go` holds exactly `fleetPanel`, `boundaryPanel` and
`timelinesPanel`. Three types, three obvious names.

### 1.2 `internal/gui/state` is not GUI

It is the single-writer store and the immutable `Snapshot` — the application's
data model. It imports no toolkit, and there is a boundary test in
`internal/session/boundary_test.go` enforcing that it never will.

That test's own comment gives the game away:

> `internal/gui/state` is allowed, and is the one that would surprise somebody.

A path that needs a test comment to explain why it is permissible is misfiled.
**61 of the 69 `internal/session` source files import it** — it is the most
depended-on package in the repository, and it is filed under the one directory
the session package's own doc comment forbids it from touching.

### 1.3 Two directories inside `internal/` contain no code

```
internal/ux/       9 PNGs + README   — byte-identical to docs/ux/
internal/output/   4 PNGs + a WAV    — byte-identical to docs/output/
```

Both are tracked. `diff -rq` reports no differences against their `docs/`
counterparts. Almost certainly a screenshot tool run with `--output internal/ux`
from the wrong working directory. They inflate `internal/` from 38 packages to
40 and appear in every `find` and directory listing.

### 1.4 `shaders/` is a dead duplicate that CLAUDE.md points at

```
shaders/coverage.wgsl   identical to   internal/gpu/coverage.wgsl
shaders/dechirp.wgsl    identical to   internal/gpu/dechirp.wgsl
shaders/pairs.wgsl      identical to   internal/gpu/pairs.wgsl
```

The `internal/gpu` copies are the live ones — `//go:embed` cannot reach outside
its own package directory, so `shaders/` can never become the source of truth
without moving the Go code to it. Meanwhile `internal/gpu/demod.wgsl` exists in
only one place, so the duplication is not even consistent.

CLAUDE.md's layout section lists `shaders/` as *the* home for WGSL. The map
directs you to the dead copy.

### 1.5 Build artifacts are committed at the repo root

```
engine.test   envgen   goldencap     (tracked)
workbench2                           (untracked, gitignored)
```

`envgen` and `goldencap` shadow the names of the real packages at
`tools/envgen/` and `tools/goldencap/`, so tab-completing `goldencap` at the
root gets you a binary.

### 1.6 Smaller irritations

- **`fixtures/` (root) vs `internal/fixture/`** — near-identical names, entirely
  different things. Root `fixtures` is the embedded example networks; `internal/fixture`
  is the on-disk project-file format. The root one genuinely must stay at the
  root (`//go:embed` scope, and packaging installs it to `/usr/share/meshbench/fixtures`).
- **`internal/mockup/`** is a drawing library used *only* by `tools/mockup` and
  `tools/render`. It is not part of the application but sits next to packages that are.
- **`docs/` is 49 flat `.md` files** — the same disease, one level up.
- **Two files breach the 500-line hard limit**: `internal/workbench/panels6c.go`
  (563) and `internal/workbench/configcards.go` (558). Both are on the list to
  split anyway.
- **CLAUDE.md's layout table describes ~13 packages; there are 38.** Undocumented:
  `session`, `engine`, `provider`, `environ`, `planning`, `coverage`, `energy`,
  `basemap`, `boundary`, `control`, `console`, `mcp`, `mqttclient`, `pathview`,
  `replay`, `validate`, `linkbudget`, `lora`, `gpu`, `fixture`, `gui/*`.
  `internal/session` — the second-largest package in the tree — is not on the map at all.

---

## 2. Constraints any plan must respect

These are load-bearing. Two of them shape the whole design.

**CI shards tests by package path** — `.github/workflows/ci.yml:116-151`:

```yaml
packages: ./internal/session/...
packages: ./internal/workbench/...
packages: ./internal/dsp/...
# and the rest:
pkgs=$(go list ./... | grep -vE '/internal/(session|workbench|dsp)(/|$)')
```

The `/...` suffix and the `(/|$)` regex both already include subpackages. **New
packages nested *under* `session/` and `workbench/` need no CI change; packages
moved out to be siblings silently relocate into the catch-all shard.** This is
why the plan below nests rather than flattens — it is not aesthetic preference.

**Release ldflags pin an import path** — `.github/workflows/package.yml:100,518`:

```
-X github.com/MeshBench/meshbench/internal/workbench.Version=$VERSION
```

`internal/workbench` must keep a package at exactly that path with a `Version`
var. `version.go` (8 lines) stays put. A `workbench` reduced to nothing but a
shell package would still satisfy this, but the constraint must not be
discovered during a release.

**`package.yml:68`** checks `internal/workbench/licences/licences.json` for drift
— that path is pinned too.

**`//go:embed` cannot escape its package directory.** Fixes `shaders/`'s fate,
and keeps root `fixtures/` where it is.

**A Go method must live in its receiver's package.** This is the entire
difficulty of `internal/session`, and section 4.3 is about nothing else.

**Another agent is working this branch** (`internal/sdr`, `internal/session`,
`internal/engine` are dirty). Stage 0 and Stage 1 avoid all three; the session
work in Stage 3 must wait for their branch to land.

---

## 3. The principles to apply

Four rules. The first two would have prevented every defect in section 1.

1. **A file is named for what it holds, never for when it was written.**
   No `2`, `b`, `c` suffixes; no plan-phase numbers. If two names collide, the
   names are not specific enough.
2. **One panel type per file, named after the type.** `configPanel` →
   `configpanel.go`. This is already how the code is *shaped*; only the
   filenames disagree.
3. **A directory is a boundary, not a bucket.** A subdirectory earns its place
   by having an import edge that points one way. `gui/mapview` may import
   `gui/comp`; the reverse is a design error and should be visible as one.
4. **Split at the type, not at the line count.** A package with 24 cohesive
   files is healthier than three packages sharing a `common.go`.

---

## 4. The target tree

### 4.1 Root and `internal/` top level

```
cmd/                      unchanged
fixtures/                 unchanged — embed root, installed to /usr/share (document why)
docs/                     subdivided (§4.5)
packaging/                unchanged
tools/                    + tools/internal/mockup   (moved from internal/mockup)
shaders/                  DELETED — dead duplicate of internal/gpu/*.wgsl

internal/
  state/                  MOVED from internal/gui/state — the data model, toolkit-free
  gui/
    theme/  comp/  shell/  desktop/  float/  pick/    unchanged paths
    mapview/              NEW — split out of gui/comp
  session/                subdivided (§4.3)
  workbench/              subdivided (§4.4)
  engine/ firmware/ rf/ dsp/ lora/ antenna/ terrain/ environ/ gpu/
  scenario/ capture/ sdr/ companion/ coverage/ energy/ planning/
  provider/ basemap/ boundary/ console/ control/ mcp/ mqttclient/
  pathview/ replay/ validate/ linkbudget/ fixture/
                          all unchanged
  ux/                     DELETED — duplicate of docs/ux
  output/                 DELETED — duplicate of docs/output
```

Deletions and the `state` move are the highest value-per-unit-risk changes in
the document.

### 4.2 `internal/gui/comp` → `comp` + `mapview`

13 of 24 source files are map rendering; the other 11 are generic widgets. They
share nothing but the package clause.

```
internal/gui/comp/        cards chips table tablerows list menurow splitter
                          shapes timeline waterfall budget energy links matrix
                          comp.go
                          (11 files, ~2,100 lines)

internal/gui/mapview/     mapview mapworld mapchrome mapinput mapfocus
                          maplabels mapmenu mapzoom tiles affine
                          (13 files, ~3,500 lines)
```

`mapview` imports `comp`, `state`, `theme`, `basemap`. One direction, no cycle.
`internal/workbench` and `internal/session` both already import `gui/comp` and
will import both after the split — a mechanical import-line change.

### 4.3 `internal/session` — the hard one

**Why file-moving does not work here.** `Sim` has 79 methods spread across 33
files. A method must live in its receiver's package, so moving `gpuwarm.go` to
`internal/session/gpuwarm/` does not compile — every `func (s *Sim) …` in it
breaks. Splitting this package is a refactor, not a `git mv`.

There are two patterns for doing it, and they cost very different amounts.

**Pattern A — extract a component.** The subpackage owns a type; `Sim` holds a
pointer; the methods move to the new type. `Sim`'s own field list already names
the candidates: `sdrServers`, `bench benchLive`, `gpuProbe`, `prefs`, `consoles`.
Real work, real payoff.

**Pattern B — extract a pure function.** The subpackage exposes
`func Something(ctx, w *state.World, …) (…, error)`, and `Sim`'s method shrinks
to a three-line call. Works wherever the method is a computation over the World
rather than a mutation of `Sim`. Cheap.

Proposed decomposition, ordered by value ÷ risk:

| New package | Files | Lines | Pattern | Notes |
|---|---:|---:|---|---|
| `session/analysis` | coverage, coveragemap, linkprofile, excessloss, nodestats, validate, energy, inventory | ~1,180 | **B** | mostly reads `World`; start here |
| `session/environ` | environfetch, environmicrosoft, terrainprefetch, hillshade, boundary | ~1,010 | B/A | self-contained tile + building pulls |
| `session/warm` | gpuwarm, matrixdisk | ~580 | **A** | `gpuProbe`/`gpuOnce`/`gpuMu` move wholesale — deletes 5 `Sim` fields |
| `session/experiment` | experiment, experimentarm, experimentarms, experimentreport, experimentrun, sweep, benchlive | ~2,050 | **A** | already owns `experiment`, `ExpArm`, `ExpResult`, `SweepPlan` |
| `session/companionbench` | companion, companionconfig, companionsession, companionview, meshcli | ~1,290 | **A** | owns `compSession` |
| `session/provision` | provisioning, provisioning_settings, radioreconcile, firmwarecatalogue, firmwarelib | ~1,320 | A | owns `Provisioning` |
| `session/traffic` | capture, packetledger, packetview, wireshark, waterfall, sdrserve | ~1,315 | A | owns `sdrServer`; **currently dirty — wait** |

What deliberately stays in `internal/session`: `session.go`, `engine.go`,
`enginefirmware.go`, `enginereadout.go`, `ui.go`, `prefs.go`, `runs.go`,
`runkind.go`, `logs.go`, `socket.go`, `simctl.go`, `fleet.go`, `importcommit.go`,
`fixture.go`, `schedule.go`, `console.go`, `rfmode.go`, `manipulation.go` — the
`Sim` lifecycle and the store wiring. Roughly 18 files. Navigable.

**The verbs are the open question.** Thirteen files (`verbs*.go`, `logic_*.go`,
`planningverbs.go`) register store handlers, and every one takes `s *Sim`.
Moving them to `session/verbs/` requires exporting ~40 `Sim` methods purely to
cross a package boundary — a large API surface created for a directory listing.

**Recommendation: do not extract the verbs. Rename them instead.** A `verb_`
prefix clusters all thirteen in the listing and costs nothing:

```
verbs.go          → verb_registry.go     (the Register fan-out)
verbsbench.go     → verb_bench.go
verbsbudget.go    → verb_budget.go
verbsclock.go     → verb_clock.go
verbscoverage.go  → verb_coverage.go
verbsimport.go    → verb_import.go
verbsnodes.go     → verb_nodes.go
verbssweep.go     → verb_sweep.go
planningverbs.go  → verb_planning.go
logic_bench.go    → verb_benchlogic.go
logic_importer.go → verb_importlogic.go
logic_observed.go → verb_observed.go
logic_planning.go → verb_planninglogic.go
```

That gets most of the navigation benefit for none of the API cost. Revisit only
if a second front end ever needs the verbs independently — the "write the
interface at the second implementation" rule applies.

Reassess after `session/analysis`. If Pattern B lands cleanly, the rest follows;
if it fights, stop after the extractions and take the renames as the win.

### 4.4 `internal/workbench` — the easy one

Unlike `session`, this package is *already* decomposed into types
(`configPanel`, `packetPanel`, `nodeWindowPanel`, `companionTab`, …), each with
its own methods. The directory just does not reflect it.

**The enabling move:** `panelDeps`, `Do`, `callbacks` and `menuDeps` are the
shared plumbing every panel needs. They become a small exported
`internal/workbench/panel` package. Six-plus panel families already exist, so
this satisfies the "interface at the second implementation" rule several times
over — it is overdue, not speculative.

```
internal/workbench/            shell and wiring — main, ui, uiverbs, windows,
                               wiring, menus, menubar, menuactions, panellist,
                               prompts, settings, shutdown, startupflags, fps,
                               sessionlog, version.go  ← Version stays here
  panel/                       panelDeps, Do, callbacks, menuDeps  (NEW)
  packetview/                  12 files, ~3,450 lines  ← biggest, cleanest seam
  nodeview/                    8 files,  ~2,350 lines
  companionpanel/              4 files,    ~890 lines
  experiment/                  sweeparms sweeppanel sweepresults compare bench
  library/                     fwlib fwlibrow licencepanel + licences/
  licences/                    unchanged — pinned by package.yml:68
```

**Renames, applied whether or not the split happens** (rule 2 — one panel per
file, named for the type):

```
panels.go        → nodespanel.go
panels6.go       → schedulepanel.go, consolepanel.go
panels6b.go      → fleetpanel.go, boundarypanel.go, timelinespanel.go
panels6c.go      → configpanel.go, logpanel.go          (also clears 563 > 500)
events2.go       → eventspanel.go
tables.go        → scorepanel.go                          (holds only scorePanel)
tables2.go       → runspanel.go                           (holds only runsPanel)
actionpanels.go  → fleetcontrols.go, schedulecontrols.go,
                   planningcontrols.go, provisioningcontrols.go
                   (`Do` moves to panel/)
livepanels.go    → benchcontrols.go, feedcontrols.go
networkpanels.go → importcontrols.go, boundarycontrols.go, validatecontrols.go
configcards.go   → split by card family                  (also clears 558 > 500)
```

Note that `tables.go` and `tables2.go` each hold exactly one type already, so
they are straight renames — but both carry a stale doc comment (`tables.go` says
"the tables of P4" and holds a single `scorePanel`; `tables2.go` says "installed
firmware, and past runs" and holds only `runsPanel`). Fix the comment in the
same commit as the rename, or the misdirection just moves to a new filename.

These renames are worth doing **first and on their own**. They are pure `git mv`
plus splitting a file at type boundaries — no import changes, no API changes, no
CI changes — and they remove the single most disorienting thing in the tree.

### 4.5 `docs/`

49 flat `.md` files, mixing ADRs, plans, study reports, design notes and user
guides.

```
docs/
  adr/          adr-0019-headless.md, adr-gap-review.md
  plans/        master-plan*.md, plan-2026-08.md, distribution-plan.md,
                release-packaging-plan.md, firmware-bug-detection-plan.md,
                extracting-the-study-engine.md, overnight-2026-08-12.md
  design/       architecture.md, rf-chain.md, gio-workbench.md,
                lower-radio-shim.md, radio-state*.md, mini-companion.md,
                firmware-integration.md, packaging-emulation.md,
                emulated-published-firmware.md, driving-the-workbench.md
  studies/      study-report-*.md, study-protocol-ideas.md, bench-parity.md
                (existing docs/studies/ merges in)
  reference/    licence.md, licences/, licences.json, shortcomings.md,
                repositories.md, fixtures.md, experiments.md
  bugs/         bugs-0.0.1.md, bugs-0.0.3.md
  user/         unchanged
  images/ ux/ output/   unchanged
docs/README.md  stays at top level — the index
```

`docs/shortcomings.md` is named in CLAUDE.md's domain rules; if it moves to
`reference/`, CLAUDE.md must move with it in the same commit.

---

## 5. Sequencing

Ordered so that each stage is independently revertible and the risky work
happens last. **Stages 0–2 touch none of the files the other agent has open.**

### Stage 0 — deletions and hygiene *(no Go code changes; minutes)*

- `git rm -r internal/ux internal/output` — verified byte-identical to `docs/`.
- `git rm -r shaders/` — verified byte-identical to `internal/gpu/*.wgsl`.
- `git rm --cached engine.test envgen goldencap`; add to `.gitignore` alongside
  the existing `/workbench2` entry.
- Update CLAUDE.md's layout table: drop `shaders/`, add the ~25 missing packages,
  note where WGSL actually lives.

Verify: `go build ./... && go test ./...`. Nothing above is referenced by any
Go file, so this should be a no-op to the build — which is the point.

### Stage 1 — workbench renames *(pure `git mv` + file splits)*

Section 4.4's rename table. No import changes, no API changes, no CI changes.
Split the two >500-line files at their type boundaries while renaming, which
clears both hard-limit breaches as a side effect.

Verify: `gofmt -l .` empty, `go vet ./...`, `golangci-lint run`, `go test ./...`.

### Stage 2 — `internal/gui/state` → `internal/state` *(mechanical, wide)*

```bash
git mv internal/gui/state internal/state
grep -rl 'internal/gui/state' --include='*.go' . \
  | xargs sed -i 's|internal/gui/state|internal/state|g'
gofmt -w .
```

Touches ~100 files but only import lines. **Also update the allow-list in
`internal/session/boundary_test.go`** (the `internal/gui/state` entry and the
comment explaining it — the comment can simply go).

Do this in its own commit. A wide, boring diff is easy to review; mixed with
logic changes it is not.

### Stage 3 — `gui/comp` → `comp` + `mapview` *(one real package split)*

Section 4.2. Some currently-unexported helpers shared across the seam will need
exporting; keep that list short and note each one in the PR. If it grows past a
handful, the seam is in the wrong place — move it rather than exporting more.

### Stage 4 — workbench subpackages *(needs the `panel` package first)*

`internal/workbench/panel` first, then `packetview` as the pilot — it is the
biggest and cleanest. Judge the remaining families on how that one goes.
Confirm `Version` still resolves at `internal/workbench.Version` before merging;
a broken release ldflag will not show up in `go test`.

### Stage 5 — session decomposition *(after the other agent's branch lands)*

Verb renames first (free, immediate). Then `session/analysis` as the Pattern B
pilot. Then reassess.

**This stage wants an ADR.** It changes the shape of `Sim`, and CLAUDE.md points
ADRs at the Plane project's Pages under MSIM.

---

## 6. Rules to add to CLAUDE.md

The layout rules did not prevent any of section 1's defects, because they are
about size and none of these are size problems. Proposed additions to the
**Limits** table:

| Rule | Limit |
|---|---|
| Filename | says what the file holds — never a plan phase, never a `2`/`b`/`c` suffix |
| Panel/widget files | one type per file, named after the type |
| Duplicated asset | none — one copy, and the code points at it |
| Build artifacts | never tracked |

And a line under **Layout**: *the table is the map; a new package updates it in
the same commit.* The reason `internal/session` grew to 69 files partly unnoticed
is that it was never on the map to begin with.

---

## 7. What this is worth

| Stage | Effort | Risk | Payoff |
|---|---|---|---|
| 0 — deletions | minutes | none | 2 phantom packages and a misleading `shaders/` gone |
| 1 — renames | hours | none | the worst navigation defect, fixed outright |
| 2 — `state` move | ~1 hour | low, wide | the most-imported package stops lying about what it is |
| 3 — `mapview` | half a day | low | 5,600 lines → two honest packages |
| 4 — workbench | 2–3 days | medium | 68 files → ~20 + six named families |
| 5 — session | week+ | high | 69 files → ~18 + six components; needs an ADR |

Stages 0–2 are most of the benefit for almost none of the risk, and none of
them collide with work in flight. If only one thing gets done, do Stage 1.
