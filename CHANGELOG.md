# Changelog

Notable changes to MeshBench, newest first.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**MeshBench is 0.x deliberately, and there is no 1.0 scheduled.** While the
major version is 0 the interface may change between releases, and this file is
the only notice: there is no deprecation period behind it. Three things are the
exception, because a mismatch there is refused at runtime rather than guessed
at: the control protocol number, the rule that a client and the workbench it
drives are the same release, and the fixture format, which refuses a file
written by a later build. [`docs/compatibility.md`](docs/compatibility.md) says
what each of those promises, and what would have to be true before 1.0 was
worth cutting.

**The 0.0.x entries below were reconstructed from the commit history.** Those
four releases shipped with identical installation notes and no record of what
had changed in them - which is the gap this file exists to close.

## [Unreleased]

### Fixed

- **No published board image could be downloaded.** The catalogue derives a
  build's version from its asset name, `v1.17.1`, while MeshCore tags its
  releases by role, `repeater-v1.17.1`. The download asked for
  `releases/tags/v1.17.1` and got a 404 for every board image ever offered,
  so an emulated board could not be started by anyone whose cache did not
  already hold one. It now finds the image the same way the library listed
  it.

- **Opening a packet in its own window crashed the workbench.** `missKinds`
  grew a sixth entry and the five chips beside it did not, so drawing the
  legend indexed past the end and the panel took the application with it. The
  chip arrays are now declared from the lists' own lengths, which is a
  compile-time check rather than a matching pair somebody has to remember.

- **Fetching buildings pointed at a tool no release ships.** A pull too large
  for a live Overpass call said to prepare the region with `tools/envgen`,
  which is a source tool: the one route the message offered was closed to
  exactly the people who hit the limit. It now names **Microsoft alone**,
  which is priced by download size rather than by area and can fetch the
  default network's footprints today, and suggests narrowing the network.
- **The Microsoft pull's size cap had never fired.** The dataset index writes
  its sizes for a person to read - `74.7KB` - and they were parsed as plain
  integers, so every file was priced at zero and the 8 GB guard was never
  reached. A pull of any size was accepted by a guard whose whole purpose is
  to price one before a byte moves.
- **Windows: the application started and disappeared.** `meshbench.exe` with
  no arguments printed its usage and exited 2. The installer's Start menu
  shortcut passes no arguments, and neither does a `meshbench.exe`
  double-clicked out of the zip, so both of them did that - and a release is
  linked `-H windowsgui`, which starts with no standard handles, so the text
  explaining it went nowhere. A bare invocation now opens the workbench, which
  is what the `.desktop` file and the macOS wrapper have always asked for and
  what the README beside the binary already promised.
- **Windows: a failure now says why.** With a terminal, output appears there;
  with none, it goes to `meshbench-error.log` in the cache directory, and that
  includes a panic's stack trace, which previously reached nobody at all.
  Output that was redirected or piped stays where it was sent.

## [0.0.5] - 2026-09-02

### Added

- **A written compatibility story, and the parts of it the build enforces.**
  `docs/compatibility.md` says what 0.x means here rather than leaving it to
  the convention that anything may break: the control protocol number and when
  it moves, the client and workbench pairing rule, the fixture format, how
  every platform stamps its version, where the GPL source archive stands, and
  seven concrete things that would have to be true before 1.0 was worth
  cutting. `MeshBench/gio` is the fork the `replace` directive points at, so a
  private one would make the source archive worthless; it is public, checked
  rather than assumed, and `docs/repositories.md` now says so and how it was
  checked.
- **A fixture carries the format it was written by, and a build refuses one
  from the future.** A file whose `format` is higher than this build reads is
  refused by name, with both numbers and the release to install, rather than
  read for the parts it recognises. Reading three quarters of a fixture does
  not fail: it answers a question about a network nobody described. Older
  files, including every one written before the field existed, still open.
- **The version stamp is pinned across all three build paths.** Linux, macOS
  and Windows are built by different jobs, and once they disagree the
  difference is invisible until a release is out. A test now reads the three
  build commands out of the pipeline and fails if any of them builds the binary
  without stamping the tag, drops its `v`, or if a fourth path appears without
  one. All three also refuse a version that is not a plain `X.Y.Z`, because a
  workbench stamped `vv0.1.0` is not a release as far as the pairing rule can
  tell and would pair with anything.

- **The emulator toolchain is fetchable.** `radioserver`, QEMU and Renode are
  rows on the Resources page like anything else the application downloads, with
  a size, the terms to read first, and a fetch that verifies the digest and
  unpacks into `~/.cache/meshbench/tools/`, already the third place the
  emulator lookup searches, so a fetched tool needs no configuration. A release
  tarball carries all three beside the binary, but the AppImage and the `.deb`
  carry only `radioserver` and a source checkout had no path to any of them.
  Where a platform has no build, or where this fetcher cannot install the one
  there is, the row says which rather than offering a download that could not
  work.
- **Resource manager.** Everything the application downloads that is not
  firmware, in one page: the Nordic SoftDevice plus the caches that fill
  themselves as the map is used - terrain, basemap, map tiles, building
  footprints - with measured sizes, the terms each arrived under, and a way to
  delete them. On the machine it was written for those caches had reached
  7.4 GB with nothing in the application to say so.
- **Board compatibility matrix.** Eight capabilities measured per board -
  build, boot, radio, tx, rx, flood, fem, power - against real published
  firmware under emulation, with three states and never a silent blank. Ten
  boards are described; the results are in the README.
- **Tabbed docking shell.** Panels dock as tabs and every one of them is
  reachable from a menu, replacing a modal chooser that could only pop panels
  out into windows that would not stay on top.
- **Coverage rasters on the GPU**, folded on the device, with CPU twins tested
  against each kernel - and drawn on the map rather than only written to a PNG.
- **Link profiles for the pair you actually picked**, including endpoints that
  are points on the ground rather than nodes.
- **A companion bench** where a companion is chosen, and a Radio tab whose two
  numbers stop claiming more than they know.
- **Version stamping**: one answer, set by the release pipeline and shown in
  the window.
- **Community furniture**: `SECURITY.md`, `CODE_OF_CONDUCT.md`,
  `THIRD_PARTY_NOTICES.md`, `CITATION.cff`, issue and pull-request templates.

### Changed

- **A run carrying an emulated node now says it cannot be repeated, instead of
  being quoted as though it could.** Determinism is documented as a property of
  the whole simulator, and it is not one on a scenario with an emulator in it:
  that node's firmware is a published image with nothing in it that can receive
  the engine's tick, so the acknowledgement comes from the chip model on our
  side of the socket while the guest runs against QEMU's or Renode's clock.
  Measured on one repeater at one seed, three runs put its first transmission at
  49.83 s, 45.72 s and 55.86 s. `sim.state` and `experiment.start` now answer
  `reproducible` and `not_reproducible_why`, the Go and Python clients carry
  both on their `SimState`, the sweep says it beside the cost estimate before
  the run as well as over the results, restoring a checkpoint says the replay
  will not land where it did, and `CLAUDE.md`, `CONTRIBUTING.md`,
  `docs/architecture.md` and `docs/shortcomings.md` state the exception rather
  than the rule alone. Use native for anything being compared.
- **What buildings buy off the excess-loss term is now measured rather than
  assumed: 0.70 dB.** Fitted against 451 live ScotMesh nodes with 4.5 million
  Microsoft ML footprints over Scotland loaded, the term comes out at 29.07 dB
  against the same night's bare-earth fit of 29.77 dB, while the footprints
  remove 37.6% of the link matrix. Both are true because the fit only sees
  paths that were heard, and a path buildings price into the ground stops being
  heard at all. `docs/studies/excess-loss-with-buildings.md` is the record and
  `docs/shortcomings.md` carries the consequence, including that a crossed
  building is charged once per building with no combination rule, so an
  environment-loaded urban path says "blocked" rather than a number of
  decibels. `DefaultExcessLossDB` is unchanged at 25.1 dB, and the study says
  what would move it.
- **QEMU on all three platforms, from one release.** The pin moves to
  `v9.2.2-meshbench-sx1262-10`, which cross-compiles Linux, macOS and Windows
  from one commit and refuses to publish if the SX1262 device, its DIO1 line or
  the GPIO interrupt behind it has gone missing. The macOS bundle and the
  Windows zip had been shipping without an ESP32 emulator, and the two
  explanations for that - that the fork built Linux alone, and that emulation
  had never run on Windows - are both retired. A node on Windows reaches the
  radio model over TCP, which is the path Renode has always used; what it still
  cannot do is fetch an emulator, because the download side reads ELF and
  Mach-O headers rather than PE.
- **`resource.list` returns the rows, not a count of them.** It answered
  `{"rows": 5}` and left the rows in the snapshot, where only a panel could
  reach them, so from outside the window there was no way to ask what this
  machine holds or what it could fetch. The rows now come back under
  `resources`, the fifth verb to be fixed the way `nodes.stats`,
  `firmware.library`, the study area and `console.read` were.
- **The control protocol is enforced at connect.** A client declares the wire
  version it speaks on the frame it was already sending - the token line on
  loopback TCP, the first request on a unix socket - and a version this build
  does not speak is refused before any verb runs, with both numbers and which
  end to upgrade. The rule is an exact match in both directions, because the
  number moves only when something an older client relied on has changed. A
  client that declares nothing is still served: it finds out from
  `session.hello`, as the shipped clients always have.
- **The tree is nine layers** (`diag`, `rf`, `mesh`, `firmware`, `world`,
  `sim`, `study`, `app`, `ui`), with `internal/layers_test.go` failing the
  build on an import that points upward, and `internal/layoutmap_test.go`
  failing when the layout map in `CLAUDE.md` stops matching the tree. Several
  packages were split out along the way: `rf/geo` (one great-circle
  implementation, not thirteen), `rf/propagation` out from under the raster,
  and the frame decoder out from under the capture recorder.
- **Verify became Validate**, and says where you are in it.
- Six files came back under the length limit; panel filenames say which panel
  they hold; the panel list is one file per family.


- **A cached link matrix could not say whether buildings were priced into it.**
  The measured matrix persists to disk under a fingerprint of the geometry it
  is about, and the environment was not in that fingerprint, so one key covered
  two different physics: a session opened over bare earth restored a
  building-priced matrix, found it already answered every pair, skipped the
  warm and reported itself measured, with a third of the country's links
  missing and nothing saying why. Unlike the excess-loss term, a priced rooftop
  is baked into the cached number and cannot be taken back out where the cache
  is read, so the environment now keys the matrix, switching it re-keys the
  session, and `matrixVersion` moves to 3 so no file written before this can be
  mistaken for either. Found while measuring what buildings buy, twice, as two
  arms that agreed to sixteen significant figures.
- **`licgen` wrote to a directory the seven-layer move had renamed**, so it
  exited 1 on every invocation and the release pipeline had been broken since
  19 August. It is now checked on every pull request rather than only at a tag.
- **A stopped store made its callers wait for ever** instead of refusing.
- **`resource.fetched` deadlocked the store** by posting a command to the
  goroutine that was running it - which would have hung the workbench the first
  time any download completed.
- **Disabled buttons still accepted clicks.** `comp.Button` applied
  `gtx.Disabled()` inside the `Clickable`, so every disabled control in the
  application pressed perfectly while drawn faded.
- **The job cancel that had existed for months and could never be called.**
- **A release stopped keeping two copies of itself.**
- Four sections of `docs/shortcomings.md` that had stopped being true, and the
  `CLAUDE.md` layout map that a new package had shipped without.

## [0.0.4] - 2026-08-16

Sweeps, and measuring rather than assuming.

- A sweep happens in front of you, and sends under a scope again.
- An arm has to demonstrate its manipulation before a cell measures anything.
- A fixture with a front-end module, because no shipped one had ever had one.
- The first published study, and the data behind it.
- Corrections: how close the links actually are, what a decibel was worth on
  the deliveries a run already made, and a fault that needs nobody to switch
  it on.

## [0.0.3] - 2026-08-14

- Removed the catalogue probe.

## [0.0.2] - 2026-08-14

The three bugs the 0.0.1 install turned up.

- The firmware library was losing half its contents; the published board images
  are back in it, it rebuilds when the catalogue answers, and it filters to the
  boards that can actually be emulated.
- Fixtures that install, and a GPU that has to prove itself before being used.
- A Windows check in CI, because DX12 had never been exercised.
- macOS bundles carry their emulators inside them.

## [0.0.1] - 2026-08-14

First release: one binary per platform.

- AppImage, `.deb` and tarball for Linux; `.dmg` for Apple Silicon; `.zip` for
  Windows. Neither the macOS nor the Windows build is code-signed.
- The radio model reachable over TCP where there is no unix socket.
- Two data races found by `-race` and fixed.

[Unreleased]: https://github.com/MeshBench/meshbench/compare/v0.0.5...HEAD
[0.0.5]: https://github.com/MeshBench/meshbench/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/MeshBench/meshbench/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/MeshBench/meshbench/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/MeshBench/meshbench/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/MeshBench/meshbench/releases/tag/v0.0.1
