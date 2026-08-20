# Contributing to MeshBench

The rules here are mostly **mechanical and enforced by CI**, deliberately —
taste does not survive scale, and this codebase is already 100,000 lines.
Reading this first is quicker than finding out from a failed run.

`CLAUDE.md` in the repository root states the same rules more tersely and is
**the authority** — it is what the build enforces. If this file ever disagrees
with it, this file is wrong.

## Before you start

Open an issue first for anything larger than a fix. Three templates exist
because this project gets three kinds of report — a bug, a board that will not
run, and a number that looks wrong — and they need different information.

**A simulation result you disagree with is welcome.** Read
[`docs/shortcomings.md`](docs/shortcomings.md) first: it records what the model
does not do and *in which direction* it errs. If the answer is in there, the
report is still worth making when the *size* of the error surprises you.

## What CI checks, so you can check it first

```bash
gofmt -l .          # must print nothing
go vet ./...
golangci-lint run   # pin to the version CI uses; a newer one disagrees
go test ./...
```

`golangci-lint` must be **the version `ci.yml` pins**. Versions genuinely
disagree about this tree — v2.1.6 and v2.12.2 differ by 29 findings — so
linting with a different one tells you the tree is clean when CI will not.

`go test -race ./...` runs on request rather than every push. Run it yourself
for anything touching concurrency; it has already caught a race that crashed
every startup.

## The rules a reviewer will hold you to

**Style.** [Effective Go](https://go.dev/doc/effective_go) plus the
[Google Go Style Guide](https://google.github.io/styleguide/go/). Where they
are silent and [Uber's](https://github.com/uber-go/guide) is specific, follow
Uber.

**Seven layers, and imports never point upward.** `rf` → `mesh` → `world` →
`sim` → `study` → `app` → `ui`. A package may import its own layer and
everything below it, never anything above.
`internal/layers_test.go` fails the build otherwise, so this is a check rather
than a description.

**A new package updates the layout map in `CLAUDE.md`, in the same commit.**
The map being wrong is worse than the map being short. This was broken once
this week and had to be repaired by hand.

**Limits**, because they scale and taste does not:

| | |
|---|---|
| File length | 300 lines soft, **500 hard** |
| Function length | 50 lines soft |
| Nesting depth | 4 |
| Dead code | none — git remembers |
| Filename | says what the file holds; never a plan phase, never a `2`/`b` suffix |
| Panel and widget files | one type per file, named after the type |
| Duplicated asset | none — one copy, and the code points at it |
| Build artifacts | never tracked |
| Speculative abstraction | none — write the interface at the *second* implementation |
| New dependency | justify it in the pull request, one line |
| Comments | explain *why*, never *what*; never cite a ticket the reader will not have |

**One pull request per independent change.** Two things share a pull request
only when they genuinely depend on each other. A pull request called "lint
fixes" is the thing this rule exists to prevent: a reviewer can judge thirty
type assertions, and nobody can judge thirty unrelated edits.

## Domain rules that are easy to get wrong

These are not style. Each one is a way to make the simulator quietly lie.

- **The channel decides nothing.** It sums waveforms and adds noise; whether a
  packet decodes is the demodulator's business. Never add a rule like "if two
  transmissions overlap, both fail" — capture effect must *emerge*, or this is
  a packet model with extra steps.
- **Every GPU kernel has a CPU twin, and they are tested against each other.**
  A wrong FFT does not crash. It produces a plausible waterfall and slightly
  wrong sensitivity, and nobody notices for months.
- **Reachability is asymmetric.** Compute and present both directions. A result
  that does not say *which direction* is wrong even when the arithmetic is right.
- **Antenna gain is directional.** Evaluate the pattern in the true direction to
  the far end, per direction. A scalar `gain` field is a bug.
- **Position uncertainty propagates.** A node imported at ±5 km does not get a
  confident answer.
- **Airtime must match the firmware's own `getEstAirtimeFor()`.** MeshCore's
  CSMA timing is built on it; if the channel disagrees, the two desynchronise
  silently.
- **Determinism is a feature.** Same seed, same scenario, same result. Use
  counter-based RNG, never a stateful stream shared across goroutines.
- **The simulator is kinder than the air.** Say so in the interface, and keep
  `docs/shortcomings.md` honest as the model changes.

## If you touch the interface

It is a native desktop application — Go and Gio. Not a web application. If a
design starts describing endpoints or a front end, it has drifted.

- **Panels never mutate state.** Controls fire verbs through `do(verb, params)`;
  the store owns the world; panels draw snapshots.
- **Silence is a bug.** A verb that declines says why. A control that cannot be
  pressed says why. A fallback is announced. A capped count says what was
  dropped.
- **"No data" is never drawn as zero**, and "did not apply" is a dash, not a
  cross.
- **Every colour and size comes from `theme.Theme`.** Nothing outside that
  package writes a literal colour or pixel count.
- **Nothing is done until it is seen running.** Screenshot the window — not the
  desktop — including the states that are not the happy one: empty, absent,
  refused, mid-fetch. That is where the design language is actually tested.
- A new panel joins `auditTargets`; a new state is reachable by flag
  (`-panel`, `-config-section`, `-drop-menu`, `-layers`), because a state only
  a click can reach is one nobody can capture.

## If you touch the boards

- **One emulated board at a time.** Several at once will take a twelve-core
  machine down.
- **Real released firmware only** — the published `.uf2` or `.bin`, never a
  build of our own. The point is what the shipped image does.
- **One file per board**: `board_<name>.go`, so a change to one cannot silently
  reach another.
- **Never rewrite a shipped fixture.** They go out with MeshBench, and a user's
  own on-disk copies would never see the change anyway.
- Update the board matrix in `README.md` in the same pull request as the change
  that moves it.

## Copyright, and why it is asked about

MeshBench is **GPL-3.0-or-later** ([`docs/licence.md`](docs/licence.md)).

Copyright is currently held by one person, which is what keeps relicensing,
dual-licensing or selling an exception possible. **A substantial outside
contribution ends that** unless it arrives under a contributor licence
agreement, and there is no CLA today.

So: small fixes and documentation are welcome as ordinary pull requests. For
anything substantial, **open an issue first** — the licensing question gets
settled before the code is written rather than after, which is fairer to
whoever writes it.

By contributing you agree your work is licensed GPL-3.0-or-later.

## Commit messages and pull requests

Say **why**, in the imperative. The diff already says what. If it fixes
something, describe what the symptom looked like from outside — that is how the
next person will recognise it.

The pull request template carries the checklist. The two items CI cannot check
are the ones most often missed: the layout map, and whether
`docs/shortcomings.md` still tells the truth after your change.
