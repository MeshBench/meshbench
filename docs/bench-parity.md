# The Bench, against what workbench1 actually did

Recovered from `51a41d8^:internal/ui/experiment.go` (1111 lines) and
`benchview.go` (972). Written down because the gap is much wider than the
layout, and because two of the things found on the way are live faults rather
than missing features.

## Two live faults, before any parity work

**Collisions are permanently zero.** `ExpResult.Collided` is never assigned:
`runArm` sums only Sent/Heard/UniqueDelivery/RedundantRelay/AirtimeMs off the
scoreboard (`internal/session/experimentrun.go:256-262`). So the `collisions`
figure in `summarise` (`experiment.go:448`) and every delta `experiment.compare`
computes from it is zero by construction. A sweep comparing two firmwares on
collision behaviour currently cannot show one.

**Provisioning never reaches an experiment cell.** `runArm` provisions from
`ProvisioningFor(n)`, which is `provisioningWith(DefaultProvisioning(), n)`
(`internal/session/provisioning.go:28-30`) — the *defaults*, not the session's
settings. `s.provisionLines` is the settings-aware one and the runner does not
call it. So anything set in the Provisioning panel, including the `extra` CLI
lines, is silently dropped for every arm.

That second one matters immediately: the plan in
`docs/radio-state-progress.md` was to enable AGC resets with
`set agc.reset.interval 4` through provisioning `extra`, so the 1.17.1 gain
fault could be provoked. It would not have reached a single node.

## The arm

workbench1, `experiment.go:31-54` — nine fields:

| Field | Meaning |
|---|---|
| `Label` | display name, and the join key from results to summaries |
| `RepeaterVersion` | firmware for every non-companion running firmware |
| `CompanionVersion` | firmware for companions |
| `PathHashMode *int32` | the **companion's** path-hash mode: what a message carries |
| `RepPathHash *int32` | the repeaters' own, for the adverts they originate |
| `LoopDetect` | off / minimal / moderate / strict |
| `CAD` | on / off |
| `SpreadMs *int32` | overrides the experiment's sender stagger for this arm |

The pointers are not decoration. The comment records why: mode 0 is a real
value — one byte per hop — and also the zero value, so "unset" and "one byte"
were the same thing to every path that built an arm without naming the field.

Two application modes, and the split is load-bearing: `apply` writes all four
provisioning fields with `-1` for unset (used for the base arm only);
`applyOver` writes **only what the arm names**, over the base.

workbench2's `ExpArm` (`internal/session/experiment.go:37-41`) has three of the
nine, no base arm, and no apply/applyOver split.

## What could be varied

workbench1's dropdown, `benchview.go:282-284`:

| Parameter | Values offered | Label segment |
|---|---|---|
| companion path hash | 0, 1, 2 | `1-byte` / `2-byte` / `3-byte` |
| repeater path hash | 0, 1, 2 | `rpt N-byte` |
| loop.detect | off, minimal, moderate, strict | `loop <v>` |
| cad | off, on | `cad <v>` |
| firmware | 1.16.0, 1.17.0 | `<v>` |
| spread | 0, 5, 20 | `all at once` / `over Ns` |

**`add arms` takes the cross product**: three hash sizes by two firmware
versions is six arms, labels joined with ` · `. Firmware sets repeater *and*
companion together, prefixing `repeater-v` / `companion-v`.

workbench2 rejects everything but `repeater_version` and `companion_version`
(`experiment.go:154-157`), varies exactly one role per call, and **clears the
arm list on every vary** (`e.Arms = e.Arms[:0]`, :163-172) — so a cross product
is not reachable even by calling it twice.

The MCP tool description already advertises `path_hash_mode, loop_detect, cad`
(`internal/mcp/session_ui.go:238-241`); `experiment.define` reads none of them
and discards them silently.

## The other controls

workbench1 defaults: seeds `4417, 9001`, channel and scope `#sco`, fire at
45 s, measure for 40 s.

| Control | What it did | In workbench2 |
|---|---|---|
| scenario picker | swaps the project, **clears the senders** because they belonged to the old network | absent |
| senders: spread N / all / none / add one, with a removable list | `spread 6` is a max-min-distance walk over companions | verb takes a list; the GUI picks exactly one |
| fired N s apart | staggers senders across the burst | absent: all senders fire on one instant |
| message size bytes | pads or truncates the payload, because airtime is what collides | absent: text is fixed |
| observer | a listen-only companion | absent (and unread in wb1 too) |
| on channel | resolves the channel by name | hard-coded index 0 |
| scoped | sets the sender's scope before each send | absent; only a global default |
| fire at / measure for | `RunForMs` is a window **after** the burst | `RunForMs` is absolute simulated end — a wb1 "45 s then 40 s" is 85000 here |
| repeats / seeds | comma-separated, refuses an empty list | exists |
| run estimate | measured rate after the first cell | crude: runs × runFor |
| pause / resume | honoured between cells | absent, only start and stop |

## The results table

Eleven columns: `arm, runs, tx, to repeaters, to companions, msgs, dupes,
worst dupe, collisions, airtime, to quiet`. Rows are the arithmetic mean over
that arm's seeds, grouped in first-seen order.

**Delta cells.** The baseline row prints absolutes; every other cell prints
only `%+.1f%%` against the baseline. Colour polarity is inverted on purpose —
transmissions, duplicates, collisions, airtime and time-to-quiet are all
"less is better", so red is up, green is down, dim is within ±0.5 %.

**The "not a result yet" warning** is a specific statistic, not a vibe:

- per arm, `RXSpread = (maxRX − minRX) / 2 / meanRX` — half the range across
  seeds as a fraction of that arm's mean;
- `spread` = the **worst** arm's RXSpread;
- `between` = `(maxMeanRX − minMeanRX) / maxMeanRX` across arms;
- if `spread >= between`: *"the seeds disagree by more than the arms do on
  receptions (±X% within an arm, Y% between them). Not a result yet — add
  repeats."*

workbench2's `notAResultYet` is a different check — it catches nothing run,
one seed, one arm, a failed run, or a zero spread. It does not compare spread
against between, and there is no verdict and no investigation.

**The verdict and `investigate`** are worth porting for their own sake. When
the sweep finishes, wb1 compares every non-baseline arm on seven metrics and
either reports the largest change against the seed spread, or says there was
none — and when there is none it runs five ordered checks: are the ledgers
byte-identical, do the arms actually carry different settings, is the
transmitted packet-size histogram identical, were there any collisions to
arbitrate at all, and finally read the setting back off a node. That is the
"the setting never reached the firmware" detector, and it is exactly the class
of fault this simulator exists to catch.

workbench2's table has seven columns, no baseline, no deltas, no colour. Its
`rx spread` is a different statistic — the raw range, not the normalised
half-range.

## Timelines

Small multiples, one histogram per arm, never an overlay: *"Overlaying four
floods produces a solid block; small multiples let the eye do the work."*

Receptions per second after the burst, 1-second buckets, `RunForMs + spreadMs`
wide so a staggered arm's tail is not cut off, summed across seeds, and every
panel drawn on the **shared** peak scale so they are comparable.

workbench2 collects no `perSecond` series at all, so the panel has nothing to
draw — which is what "no counters yet" in the Bench view actually means.

## The run queue

arms × seeds, arm outer, seed inner; results are positional, so row *i* is
result *i*. A row is `done` / `failed` / `flagged` when `i < len(results)`, the
live phase name when `i == len(results)`, else `queued`. The phases are real:
`preparing, firmware, settling, connecting, sending, running, collecting` —
frame-stepped so the window keeps drawing.

The result column is `%d msgs  R %d/%d  C %d/%d`, then a per-message list of
`repeaters+companions` — *"Each message on its own, so one that got nowhere is
visible rather than averaged away."*

A hard failure ends the whole sweep rather than skipping the cell.

workbench2 has no run queue view: the snapshot carries only arm summaries, so
the GUI cannot draw per-run rows even though `experiment.results` returns them.

## Suggested order

1. **The two live faults** — assign `Collided`, and make `runArm` provision
   from the session rather than the defaults. Small, and both are wrong now.
2. **Arm model and crossing** — the nine-field arm, `applyOver`, and a `vary`
   that crosses instead of clearing. Everything else reads better once arms
   can express more than a version.
3. **Deltas and the spread-vs-between warning** — the reason to have a matrix.
4. **`perSecond` and the timelines** — cheap once the runner records it.
5. **The run queue** — needs per-run rows in the snapshot.
6. **The remaining controls** — spread, message size, channel by name, scope,
   sender helpers, pause.
