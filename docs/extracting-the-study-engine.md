# Extracting the study engine

The ten `experiment.*` socket verbs do not exist in the Gio build, which is why
the 1.16/1.17 study had to be run in the old workbench. They cannot simply be
reimplemented: the new `SweepPlan` carries one metric per cell, and the old
results carry transmissions, receptions, reach, collisions, deaf time, airtime
and the radio counters. Wiring the verbs to the simpler runner would return
numbers that look right and disagree with the old workbench, on a tool whose
whole job is deciding whether a firmware difference is real.

So the engine moves, and both workbenches drive it.

## What makes this cheap

`internal/ui/experiment.go` (1,111 lines) and `expreport.go` (251) contain
**zero imgui references**. Nothing in them ever needed a window. This is a
package boundary that was never drawn, not a dependency to be broken.

The split, measured:

| part | lines |
|---|---|
| pure - types, summarise, verdict, divergence | 500 |
| bound to `App` - the runner | 582 |

## The approach that does not work

Moving only the pure half and leaving the runner in `internal/ui` requires
**17 fields** to cross the package boundary, because the runner reads the
experiment's own working state: `arm`, `seed`, `burstMs`, `spreadMs`,
`pending`, `started`, `runStart`, `ledger`, `perSecond` and the rest.

Exporting those names collides. `done`, `node`, `seed` and `arm` are fields on
jobs, nodes and half a dozen unrelated structs; a rename touched 34 files and
collided on `spreadMs` against an existing `SpreadMs`. Tried on
2026-08-13 and reverted.

## The approach that does

Move **both** halves into `internal/study`, with the runner behind a `Host`
interface. Then the working state stays private and only what a panel actually
draws crosses the boundary.

Measured, that is **7 fields at 53 sites in 4 files**:

| field | reads |
|---|---|
| `results` | 23 |
| `running` | 9 |
| `baselineArm` | 9 |
| `status` | 4 |
| `paused` | 3 |
| `log` | 3 |
| `phase` | 2 |

by file: `benchview.go` 28, `control.go` 15, `expreport.go` 8, `expdupes.go` 2.

Give those seven accessor methods rather than exported fields, so a panel can
read the state and nothing outside can write it mid-run.

`Host` is what the runner needs from whichever workbench owns the engine:

    Nodes() []scenario.Node
    Engine() *engine.Engine
    BuildEngine()
    StartFirmware(); FirmwareProgress() (starting, done, total int, err error)
    Config() *Settings; ApplyStartupConfig()
    Companions() map[string]*companionLink
    CompanionConnect(node string) error; CompanionDisconnect(node string)
    CompanionAddChannel(node, name string) error
    CompanionConfigure(node string) error
    CompanionSend(node, text string) error
    CompanionSetScope(node, scope string) error
    Playing() bool; SetPlaying(bool); RunUntilMs() uint32; SetRunUntilMs(uint32)
    Seed() uint64
    OnStart()   // the old switchWorkspace; a no-op where there is no workspace

`switchWorkspace` is the only genuinely interface-shaped call in the runner,
and it becomes `OnStart` so a headless driver can ignore it.

## Order

1. `internal/study` with both halves and `Host`; `internal/ui` implements it.
   The old workbench must produce identical numbers afterwards - the control
   arm repeats exactly, so any change is the refactor's fault.
2. `internal/session` implements `Host` too.
3. The ten `experiment.*` verbs, against the same engine.
4. Re-run the 1.16/1.17 study in the Gio build. Agreement with the old
   workbench is the first real evidence the two are comparable, which is the
   reason `internal/ui` is still here.
