# MeshBench documentation

**User and reference documentation lives in `MeshBench/docs`.** What
is here is either something the code depends on, or source material that has
not been migrated yet.

Decisions live in Plane project **MSIM** as ADRs.

## What the code depends on

These cannot move. The build reads them, or the application prints their path.

| file | why |
|---|---|
| `licence.md` | named by `tools/licgen`, `world/scenario/boards.go` and `ui/workbench/licences_test.go` |
| `licences.json`, `licences/` | `tools/licgen` reads them and fails the build if a linked module's licence cannot be named |
| `repositories.md` | the fork table is pinned by `ui/workbench/licences_test.go` |
| `shortcomings.md` | the application prints this path to users; the docs site generates its *What it does not do* page from it |
| `verb-reference.md` | written by `tools/verbdoc/verbdoc.py`; the docs site's control socket page is a copy of it, and its build refuses when the copy has fallen behind |
| `ux/` | written by `go run ./tools/mockup` |
| `output/` | written by `go run ./tools/render` |

### `shortcomings.md`

The honesty document: what the model does not do, what it gets measurably
wrong, and in which direction. CLAUDE.md makes keeping it accurate a rule as the
model changes, which is why it lives beside the code rather than on the site.

The site renders it with `tools/sync-limits.py` and refuses to build if its copy
has fallen behind this one.

## Moved out of the README

The front page is meant to be readable in a minute, so the detail behind it
lives here and **is** maintained - unlike the working notes below.

| file | what it holds |
|---|---|
| `install.md` | per platform, including the macOS and Windows signing caveats |
| `firmware-build-settings.md` | what each setting in a build's own window does, and what it costs |
| `native-and-emulated.md` | where the real firmware ends and the simulation begins, and the two ways a node can run |

## Machine-specific, and deliberately so

`development-machines.md` — which host does what, what the lab runners are, and
what will not run where. Split out of `CLAUDE.md` so that the rules there apply
to anyone on any machine. This one is expected to go stale, and going stale
harms nothing.

## Working notes

Source material for pages that have not been written, and thinking worth
keeping. Each is technical writing in the working register — it gets rewritten
for the site rather than copied, because the site is neutral and impersonal and
these are not.

**Every one now opens with the date it was last true**, and says so where the
code has moved underneath it. That stamp is the point: `architecture.md` opened
with "read this before writing code" while eleven of the twelve package paths it
named no longer existed, which is worse than having no architecture document.

`architecture.md`, `rf-chain.md`, `firmware-integration.md`, `fixtures.md`,
`experiments.md`, `driving-the-workbench.md`, `gio-workbench.md`,
`emulated-published-firmware.md`, `packaging-emulation.md`,
`lower-radio-shim.md`, `radio-state-from-firmware.md`, `mini-companion.md`,
`study-protocol-ideas.md`, `adr-0019-headless.md`

`virtual-sx1262.md` is **not** in that list any more: two packages name it, so
it is load-bearing and belongs above.

Several already have a thinner page on the site under the same name. Those need
reconciling, not replacing.

## Studies

`study-report-rx-delay.md`, `study-report-relay-suppression.md` and
`study-protocol-ideas.md` belong in `MeshBench/meshbench-reports`, which is
public and already publishes one report. They stay here until they are written
as pages there — an unconverted markdown file in a published site is worse than
one that has not moved yet.

## The rules that are easy to violate

- **The channel decides nothing.** No code path may say "if two transmissions
  overlap, both fail".
- **Every GPU kernel has a CPU twin**, and a test asserts they agree. A wrong
  FFT does not crash — it produces a plausible waterfall and slightly wrong
  sensitivity that nobody notices for months.
- **Airtime must match the firmware's `getEstAirtimeFor()`**, or the simulation
  desynchronises from the firmware's own CSMA silently.
- **Reachability is asymmetric.** Both directions, always.
- **Position uncertainty propagates.** Too-uncertain nodes get no verdict.
- **The simulator is kinder than the air**, and must say so.
