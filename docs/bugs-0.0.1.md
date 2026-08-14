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

## 2. No firmware to download on macOS or Windows

**Measured.** `MeshBench/meshcore-native` is public and has releases, but every
release contains exactly one asset and it is Linux:

    companion-v1.17.0  ->  meshcore-companion_radio-linux-amd64
    repeater-v1.17.0   ->  meshcore-simple_repeater-linux-amd64

No release in the repository carries a darwin or windows asset. MeshBench asks
for `meshcore-<role>-<GOOS>-<GOARCH>` (`native_catalogue.go:310`) and matches
on `runtime.GOOS`/`GOARCH`, so on a Mac it correctly finds nothing. The bug is
not in the download code; there is nothing to download.

**Fix.** Two repositories, and the order matters:

1. **`MeshBench/meshcore-native` builds for more than Linux.** It compiles
   MeshCore's host variant, so it needs a compiler per target rather than
   anything clever: a `macos-14` job for `darwin-arm64` (free - that repository
   is public), and mingw or a `windows-latest` job for `windows-amd64`.
   Publish them into the same releases with the same naming, and MeshBench
   picks them up with no change at all.
2. **MeshBench says so when the shelf is empty.** Today an unavailable
   platform reads as a failure. The Firmware Library should say "no native
   MeshCore build for macOS yet - emulated boards still work" in the same
   voice the emulator gaps already use. That is a small change and it is worth
   making regardless, because it is what the user sees while (1) is being
   built.

Note the ordering consequence: **native nodes are the common path**, so until
(1) lands, a Mac or Windows user gets emulated boards and planning but cannot
run native firmware. That should be in the release notes, not discovered.

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
