# Three bugs from installing 0.0.1, and what fixing them means

Found by installing the release rather than by running the tests, which is
where this class of bug always comes from. Each section says what was
measured, what the cause is, and what the fix costs.

## 1. The fixtures are not found (all platforms, worst on the installed ones)

**Measured.** `-fixture` defaults to `fixtures/fixture-scotland-ireland-strict.json`
- a *relative* path - and there is no search-path logic anywhere in the code
(`grep` for one finds nothing). Every packaged layout puts the fixtures
somewhere else and launches the application with a working directory that is
not there:

| package | fixtures land in | working directory when launched |
|---|---|---|
| `.deb`, AppImage | `/usr/share/meshbench/fixtures` | `$HOME` (launcher) |
| macOS `.app` | `Contents/Resources/fixtures` | `/` (Finder) |
| Windows zip | beside `meshbench.exe` | wherever Explorer feels |
| tarball | beside the binary | only right if you `cd` in first |

So the tarball works if you follow the README exactly, and every other
install opens with nothing loaded. This is one bug, not four.

**Fix.** Two halves, and both are wanted:

1. **Embed the shipped fixtures** (`go:embed`, as `docs/release-packaging-plan.md`
   already proposed). `-fixture` then takes a *name* - `fife-strict` - as well
   as a path, and the default becomes a name. An application that cannot open
   its own example network without a file beside it is not installable, and
   embedding is the only fix that survives every packaging format.
2. **A search path for on-disk fixtures**, so people can drop their own in and
   so the packaged copies are still useful: beside the executable, then
   `../Resources/fixtures` (macOS bundles), then `/usr/share/meshbench/fixtures`,
   then `$XDG_DATA_DIRS/meshbench/fixtures`. First hit wins; `-fixture` with a
   path that exists always wins over both.

**Test that would have caught it**: run the built binary from `/` with no
`fixtures` directory anywhere and assert the default network opens. Cheap, and
it belongs in the packaging smoke test rather than the unit tests, because it
is a property of the *install*.

**Also worth doing**: File > Open should start in a directory that exists on
that platform rather than the process's working directory.

## 2. The published board images are missing from the Firmware Library

**This one is a migration gap, not a network fault**, and Alex named it: it
worked in workbench1.

**Measured.** Every layer below the panel is healthy. Listing the published
board images from `meshcore-dev/MeshCore` works unauthenticated, costs about
one API request (the releases come back with their assets inline, so the whole
catalogue is one page) and returns **7,982 images in about a second**. The
asset names still parse - `Ebyte_EoRa-S3_companion_radio_ble-v1.17.1-d929643-merged.bin`
matches the pattern exactly. Downloading one works from the command line on
**both** Linux and macOS:

    meshbench firmware -get RAK_4631/repeater
    RAK_4631/repeater repeater-v1.14.1 -> .../RAK_4631_repeater-v1.14.1-467959c.uf2

**Cause.** The Gio library only ever asks the *native* catalogue.
`firmware.library` fetches with `firmware.NativeCatalogue{}.List` and keeps
only images where `ForThisMachine()` is true (`internal/session/firmwarelib.go:62`),
and `publishedBuilds()` reads `NativeCatalogue.CachedImages()` for the same
reason. `BoardCatalogue` - the flasher images, the emulated-board half - is
never consulted for the *published* list at all.

Everything else is already in place, which is why this is small: `downloadBuild`
takes a board and uses `BoardCatalogue.Ensure` when it gets one
(`firmwarelib.go:327`), `state.FirmwareRow` carries a `Board`, and the panel
renders board rows. Only the listing is missing. workbench1 did exactly this
and its own comment records the shape of the problem it solved - "every
published version of every supported board is thousands of rows" - which is
why it filtered rather than listed everything.

**Why it looked like a macOS problem.** On Linux the native catalogue has
`linux-amd64` assets, so the library shows *something* and the missing board
half is easy to miss. On macOS `ForThisMachine()` matches nothing, because
`meshcore-native` publishes only `meshcore-*-linux-amd64` - so the panel is
completely empty and the fault becomes obvious.

**Fix, in two parts.**

1. **Give the library the board catalogue back.** Fetch `BoardCatalogue.ListAll`
   alongside the native list on the same worker, and merge. It must be
   filtered before it reaches the panel - 7,982 rows is not a library, it is a
   denial of service on the eye - so: only boards MeshBench can actually run
   (`scenario.Boards`), newest version per board and role by default, with the
   older versions behind the version chip. workbench1's rows are the reference
   for what to show.
2. **Native firmware for macOS and Windows** stays a real and separate gap:
   `MeshBench/meshcore-native` publishes only `linux-amd64`, so native nodes
   cannot run on a Mac at all. That wants a `macos-14` job and a Windows job
   in *that* repository, publishing with the same naming, after which
   MeshBench needs no change. Until then the library should say "no native
   MeshCore build for macOS yet - emulated boards still work" rather than
   showing an empty shelf.

**Noticed while testing**: `-get RAK_4631/repeater` fetched **v1.14.1** when
v1.17.1 exists. Whatever picks a version when the caller does not name one is
choosing arbitrarily rather than choosing the newest.

## 3. "Walking the links" does not progress on macOS

**Measured, and the obvious suspects are all innocent:**

| checked on the M4 | result |
|---|---|
| `gpu.Open()` | works, Metal backend, 326 ms |
| every GPU kernel against its CPU twin (`go test ./internal/gpu`) | all pass, including `TestPairLossMatchesCPU` |
| terrain download and profile maths (`meshbench link`) | 17.98 km link answered in 1.7 s |
| CPU coverage raster over 20 tiles | 200x200 in 8.7 s, correct output |

So the Metal path computes correctly and terrain works. Whatever is stuck is
not the arithmetic.

**What is left, in the order worth testing:**

1. **The fixture never loaded** - bug 1. With no network, the warm has nothing
   to walk, and a jobs strip that shows a link-measuring job which cannot
   advance is exactly what that looks like from the outside. This costs
   nothing to rule out and would explain the report completely.
2. **The first run is fetching terrain for a national fixture and saying the
   wrong thing about it.** The default network is Scotland-and-Ireland; on a
   machine with a cold tile cache that is a long download, and if the job line
   says "measuring every link" throughout, a working application is
   indistinguishable from a stuck one. The design language already requires
   that every long operation announce itself *and* estimate before spending -
   so if this is it, the bug is the reporting, not the speed.
3. **Progress callbacks not reaching the UI on macOS.** Least likely - the
   store and the jobs strip are platform-independent - but it is the only
   remaining path.

**How to settle it**: run the workbench on the Mac against a fixture given by
absolute path, and read the job progress from the control socket rather than
the screen. That needs Alex's say-so, because starting it puts a window on his
live desktop.

## Order

1. Fixtures (1) - it is the largest user-visible breakage, it is entirely in
   our hands, and it may be the whole of bug 3.
2. Re-test bug 3 on the Mac once fixtures load, with the socket reading
   progress.
3. The honest empty-shelf message for firmware (2.2) - small, and it is what
   users see meanwhile.
4. `meshcore-native` builds for darwin-arm64 and windows-amd64 (2.1) - the
   real fix, in another repository, and the one with the longest tail.
