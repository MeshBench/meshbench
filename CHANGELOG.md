# Changelog

Notable changes to MeshBench, newest first.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the major version is 0 the interface may change between releases without
ceremony.

**The 0.0.x entries below were reconstructed from the commit history.** Those
four releases shipped with identical installation notes and no record of what
had changed in them — which is the gap this file exists to close.

## [Unreleased]

### Added

- **Resource manager.** Everything the application downloads that is not
  firmware, in one page: the Nordic SoftDevice plus the caches that fill
  themselves as the map is used — terrain, basemap, map tiles, building
  footprints — with measured sizes, the terms each arrived under, and a way to
  delete them. On the machine it was written for those caches had reached
  7.4 GB with nothing in the application to say so.
- **Board compatibility matrix.** Eight capabilities measured per board —
  build, boot, radio, tx, rx, flood, fem, power — against real published
  firmware under emulation, with three states and never a silent blank. Ten
  boards are described; the results are in the README.
- **Tabbed docking shell.** Panels dock as tabs and every one of them is
  reachable from a menu, replacing a modal chooser that could only pop panels
  out into windows that would not stay on top.
- **Coverage rasters on the GPU**, folded on the device, with CPU twins tested
  against each kernel — and drawn on the map rather than only written to a PNG.
- **Link profiles for the pair you actually picked**, including endpoints that
  are points on the ground rather than nodes.
- **A companion bench** where a companion is chosen, and a Radio tab whose two
  numbers stop claiming more than they know.
- **Version stamping**: one answer, set by the release pipeline and shown in
  the window.
- **Community furniture**: `SECURITY.md`, `CODE_OF_CONDUCT.md`,
  `THIRD_PARTY_NOTICES.md`, `CITATION.cff`, issue and pull-request templates.

### Changed

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

### Fixed

- **`licgen` wrote to a directory the seven-layer move had renamed**, so it
  exited 1 on every invocation and the release pipeline had been broken since
  19 August. It is now checked on every pull request rather than only at a tag.
- **A stopped store made its callers wait for ever** instead of refusing.
- **`resource.fetched` deadlocked the store** by posting a command to the
  goroutine that was running it — which would have hung the workbench the first
  time any download completed.
- **Disabled buttons still accepted clicks.** `comp.Button` applied
  `gtx.Disabled()` inside the `Clickable`, so every disabled control in the
  application pressed perfectly while drawn faded.
- **The job cancel that had existed for months and could never be called.**
- **A release stopped keeping two copies of itself.**
- Four sections of `docs/shortcomings.md` that had stopped being true, and the
  `CLAUDE.md` layout map that a new package had shipped without.

## [0.0.4] — 2026-08-16

Sweeps, and measuring rather than assuming.

- A sweep happens in front of you, and sends under a scope again.
- An arm has to demonstrate its manipulation before a cell measures anything.
- A fixture with a front-end module, because no shipped one had ever had one.
- The first published study, and the data behind it.
- Corrections: how close the links actually are, what a decibel was worth on
  the deliveries a run already made, and a fault that needs nobody to switch
  it on.

## [0.0.3] — 2026-08-14

- Removed the catalogue probe.

## [0.0.2] — 2026-08-14

The three bugs the 0.0.1 install turned up.

- The firmware library was losing half its contents; the published board images
  are back in it, it rebuilds when the catalogue answers, and it filters to the
  boards that can actually be emulated.
- Fixtures that install, and a GPU that has to prove itself before being used.
- A Windows check in CI, because DX12 had never been exercised.
- macOS bundles carry their emulators inside them.

## [0.0.1] — 2026-08-14

First release: one binary per platform.

- AppImage, `.deb` and tarball for Linux; `.dmg` for Apple Silicon; `.zip` for
  Windows. Neither the macOS nor the Windows build is code-signed.
- The radio model reachable over TCP where there is no unix socket.
- Two data races found by `-race` and fixed.

[Unreleased]: https://github.com/MeshBench/meshbench/compare/v0.0.4...HEAD
[0.0.4]: https://github.com/MeshBench/meshbench/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/MeshBench/meshbench/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/MeshBench/meshbench/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/MeshBench/meshbench/releases/tag/v0.0.1
