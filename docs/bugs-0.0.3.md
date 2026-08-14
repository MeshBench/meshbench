# Four bugs from the first Mac run, and what fixing them means

All found by running 0.0.3 on a Mac. None is macOS-specific in cause; the Mac
is simply the first platform where the assumptions stopped holding.

## 1. Every node fails: the fixtures pin a version only Linux has

**Measured.** The shipped fixtures pin their firmware:

    46 nodes  simple_repeater      repeater-v1.17.0
     9 nodes  companion_radio      companion-v1.17.0
     1 node   simple_room_server   room-server-v1.17.0

and `meshcore-native` publishes, today:

| version | platforms |
|---|---|
| `repeater-v1.17.1` | darwin-arm64, linux-amd64, windows-amd64 |
| `repeater-v1.17.0` | linux-amd64 |
| `repeater-v1.16.0` | linux-amd64 |
| `companion-v1.17.0` | linux-amd64 |

Only **v1.17.1** was rebuilt when macOS and Windows were added, so a fixture
pinned to v1.17.0 has nothing to run anywhere except Linux. Hence "firmware on
0 of 309 nodes".

**Fix, and the order matters:**

1. **Backfill the older versions.** `meshcore-native`'s workflow already takes
   a list of refs, and that repository is public, so its macOS runners are
   free. Building `repeater-v1.17.0`, `companion-v1.17.0`, `room-server-v1.17.0`
   and the v1.16 line for darwin-arm64 and windows-amd64 is a dispatch, not a
   code change. This alone unbreaks the shipped fixtures.
2. **Stop a pinned version being a dead end.** When the pinned build has no
   image for this platform, MeshBench should offer the nearest one it can run
   rather than failing 309 nodes - "repeater-v1.17.0 has no macOS build; the
   nearest is v1.17.1" with one button - and say so once, not per node.
3. **Consider what a fixture should pin at all.** A shipped example that names
   an exact upstream tag ages badly by construction: it breaks whenever the
   tag stops being built for somebody's platform. A role with no version, or a
   floating "newest", would open on any machine. That is a design decision
   rather than a bug fix, so it wants a moment's thought rather than a patch.

## 1b. The pin never reaches the thing that starts firmware

**This is the one that matters**, and it explains the report exactly: the
library said 273 nodes were on `repeater-v1.17.1`, and starting them asked for
`repeater-v1.17.0`.

**Measured.** `firmware.set` writes the version in two places
(`firmwarelib.go`):

    s.nodes[i].Firmware.Version = version   // the session's scenario
    w.Nodes[i].Firmware = version           // what the panel draws

The engine is neither of them. `engine.New` takes the nodes and keeps its own
`[]*Node`, each with a copy of the spec, and `AttachNative` reads the version
from *that* copy (`engine/firmware.go:72`: `key := role + "@" + n.Spec.Firmware.Version`).
Nothing pushes a changed pin into it - `setFirmware` for a single node has the
same gap - and `startFirmware` attaches to the existing engine rather than
rebuilding from the scenario.

So the count in the library, the version on the row, and the message on screen
all come from the session's list, while what actually runs comes from a copy
made when the network was opened. They agreed until somebody changed a pin.

**Fix.** The engine's copy is the one that runs, so a pin has to reach it.
Either:

- **push the change through**: `firmware.set` and `setFirmware` ask the engine
  to update the spec of the nodes they touched, which is a small method on the
  engine and keeps the single-copy design; or
- **rebuild before starting**: `startFirmware` reconciles the engine's specs
  with the scenario first, which is one place rather than two but pays for it
  on every start.

The first is better: the change is small, it is where the user's intent
arrives, and it leaves starting fast. Either way this wants a test that sets a
pin and asserts the engine agrees - the two lists silently disagreeing is
exactly the kind of thing that only shows up as a wrong answer much later.

**Note how it presented**: because the pin looked applied everywhere a person
can see, the failure read as "the version I chose is not available" - which
sent me to the catalogue, not to the pin. The bug below (the error naming the
role rather than the platform) made that worse.

## 2. The error blames the wrong thing

    MeshCore repeater-v1.17.0 has no simple_repeater build for darwin-arm64;
    it publishes simple_repeater

It names the missing role and then lists that same role as available, which
reads as nonsense. `native_catalogue.go:296` collects the roles published for
that version **ignoring the platform**, so it answers a question nobody asked:
the role is not what is missing, the platform is.

**Fix.** Say what is actually true: which platforms that version was built
for, and which versions do have a build for this one. The message is what
somebody reads at the moment everything has failed, so it should name the way
out.

## 3. Downloading a build appears to do nothing

**Measured.** `firmware.download` adds a job with `Total: 1`, downloads, then
posts `job.progress ... Finished` and calls `firmware.installed`.

Two things are missing from that:

- **`firmware.installed` refreshes `w.Builds`, not `w.Library`.** The panel
  being looked at is drawn from `w.Library`, so the row keeps its ✗ until
  something else rebuilds it. This is the same shape as the bug fixed
  yesterday, where the published-catalogue fetch landed in a field nothing
  read again - `fillLibrary(w)` exists now and simply is not called here.
- **The job has no intermediate progress.** `Total: 1` means the bar is at
  zero until the file is finished, so a 1.2 MB image over a slow link is
  indistinguishable from a stall. The catalogue's `Ensure` knows the byte
  count; the job should carry bytes rather than a single step.

## 4. Older versions vanish from the library after a download

Not a display bug - the rows were never really there. On a Mac the library can
only show:

- what is **on disk** (three builds, all v1.17.1),
- what is **published for this machine** (the same three, because that is all
  that exists for darwin), and
- **what the scenario is running**, which is how `repeater-v1.17.0` appeared
  at all.

Once the nodes were repointed at v1.17.1, nothing referenced v1.17.0 any more
and its row had no reason to exist. It looks like a disappearance because the
row was a shadow of the scenario rather than a build.

**Fix.** Fixing (1) makes this mostly moot - the older versions become real
rows because they are published for this machine. Beyond that, a row that
exists only because a node points at it should say so ("pinned by 46 nodes,
not published for macOS") rather than looking like an ordinary build that
later evaporates.

## Order

1. Backfill darwin and windows builds for the versions the fixtures pin (1.1)
   - a dispatch, and it unbreaks the shipped example on two platforms.
2. Rebuild the library after a download, and give the job real byte progress
   (3) - small, and it is the one people will hit every day.
3. Make the failure message name the way out (2).
4. Then decide what a shipped fixture should pin (1.3), and mark scenario-only
   rows for what they are (4).
