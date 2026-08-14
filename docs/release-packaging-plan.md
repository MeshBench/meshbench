# Releases for end users: packaging, pipeline, and the licence window

The goal: somebody downloads one archive from GitHub Releases, unpacks it,
runs `./meshbench workbench`, and everything works - no toolchain, no package
manager, no hunting for emulators. And the application can show, in one
window, every licence it ships under: Alex's forks at the top, because that
is modified code, then everything else.

`.github/workflows/package.yml` already does much of the packaging half and
its decisions stand (native build because of cgo, glibc 2.38 floor from
cimgui, emulator forks fetched from our releases, the qemu symlink, README in
the bundle). This plan is what changes and what is added.

## 0. The licence: settled, GPL-3.0-or-later

**Resolved 14 August 2026 - see `docs/adr-0001-licence.md`.** MeshBench is
GPL-3.0-or-later; the LICENSE file is in the repository root and the licence
window shows it as its first entry.

Two consequences the pipeline implements:

- **Binaries carry their source.** GPL-3.0 §6 gives whoever receives the
  binary the right to its Corresponding Source, and the repository is private,
  so each release publishes `meshbench-<tag>-source.tar.gz` beside the bundle.
  When the repository is made public a link replaces it.
- **What is linked had to be checked, and was.** Everything compiled in is
  GPL-3.0-compatible; the one that needed a decision is
  `eclipse/paho.mqtt.golang`, dual-licensed EPL-2.0 **or** EDL-1.0, where only
  the EDL branch (BSD-3-Clause) may be combined with GPL. MeshBench takes EDL,
  the licence window says so on that entry, and `tools/licgen` now fails the
  build on EPL-only, GPL-2.0-only, AGPL or SSPL/BUSL dependencies.

The original constraints, kept because they still shape the bundle:

- The bundle redistributes our **QEMU fork - GPLv2**. That is fine alongside
  MeshBench's GPL-3.0 (separate programs in one archive, not one work - they
  talk over sockets, nothing is linked), and the bundle must offer the QEMU
  source, which the public `MeshBench/qemu` repository does. Same for
  tlib/renode-infrastructure under their upstream terms.
- **The fork releases are marked as ours.** The SX1262 device and the
  SEVONPEND fix are not upstreamed - no pull requests - so the artifacts in
  those forks' releases carry `meshbench` in the file name, and this pipeline
  matches them with wildcards on both sides rather than pinning where in the
  name the mark sits. A user who finds one of these files loose on disk should
  be able to tell it is a MeshBench build and not an upstream release.
- MeshCore itself is **downloaded at runtime, never redistributed by us** -
  the catalogue pulls builds from `MeshBench/meshcore-native` releases and
  board images from upstream's releases - so MeshCore's licence binds those
  repositories' releases, not this archive. The SoftDevice stays
  user-supplied, as `docs/repositories.md` already states.
- **MeshCIM is PaulMcGinley's, private, proprietary. Nothing from it may
  appear in any artifact, ever.** The pipeline should grep the assembled
  bundle for `meshcim`/`MeshCIM` and fail if anything matches - cheap
  insurance against an accidental fixture or doc drifting in.

The ADR gets written when the choice is made; the licence window's first
entry renders whatever it says.

## 1. What "all dependencies included" means here

The runtime surface, audited:

| need | where it comes from | in the bundle? |
|---|---|---|
| the app | `meshbench` (CLI + `workbench` subcommand) | yes - one binary since the cutover |
| Go libraries | statically linked | inside the binary |
| wgpu-native (GPU warm) | linked by cogentcore/webgpu | inside the binary |
| system libs (wayland/x11/GL/vulkan loader) | the user's distro | no - documented floor, checked at start (see 4) |
| native MeshCore builds | `MeshBench/meshcore-native` releases, on first use | no - downloaded, cached, shown in Firmware Library |
| board images / OTAFIX | upstream MeshCore releases, flasher.meshcore.io | no - downloaded on first use |
| QEMU fork (esp32 boards) | `MeshBench/qemu` releases | yes, as today |
| Renode portable (nRF52 boards) | `MeshBench/renode` releases, bundles dotnet | yes, as today |
| terrain + basemap tiles | AWS/CARTO/OSM/Esri, cached | no - downloaded, attributed |
| emoji-capable font | today: three hardcoded system paths | **change: embed** - see 3 |
| fixtures | repo `fixtures/` | **change: embed the shipped ones** via go:embed, so the workbench opens showing something with no files beside it |

Notable: since the cutover the extra `meshbench-workbench` binary in
package.yml is redundant - `meshbench workbench` is the Gio app. Keep ONE
shipped binary; the `-X gioui.org/app.ID=io.github.meshbench.meshbench` ldflag
moves onto it. (`cmd/workbench2` remains a dev tool, not shipped.)

## 2. Artifacts and platforms, phased

- **Phase 1 - Linux x86_64** (`meshbench-<ver>-linux-x86_64.tar.gz`):
  today's bundle, updated post-cutover. Plus `SHA256SUMS` uploaded beside
  every artifact, and the licence bundle inside (see 3).
- **Phase 1b - Linux arm64**: same job on `ubuntu-24.04-arm`. Emulator forks
  need arm64 releases first; until they exist the bundle ships without them
  and the README says which boards that costs.
- **Phase 2 - macOS arm64** (`macos-14` runner, `.tar.gz` or `.dmg`):
  workbench + native firmware only (meshcore-native needs darwin-arm64 host
  builds published first; QEMU/Renode forks have never been built on Darwin
  - ship without emulated boards and say so). Unsigned at first; the README
  carries the `xattr -d com.apple.quarantine` line until signing is set up.
- **Phase 3 - Windows**: Gio and the firmware bridge both plausibly work;
  needs its own investigation. Not in this plan's acceptance.
- **AppImage (later, optional)**: the tar.gz plus documented library floor is
  the honest v1. If field feedback shows library pain, wrap the same tree
  with linuxdeploy - it changes packaging, not the app.

Version stamping: `-ldflags -X internal/workbench.Version=<tag>`; shown in
Help > About and in `meshbench -version`; the licence window shows it too.

## 3. The licence window

A `Licences` panel, opened from **Help > Licences & attributions** (and
listed in Show-all). Content comes from a build-time-generated, embedded
file, so the window can never drift from what was actually linked.

**Order, per Alex - forks first, because that is modified code:**

1. **MeshBench itself** - the ADR-0001 licence, once chosen.
2. **Modified forks** (from `docs/repositories.md`, kept as the single
   source): `MeshBench/qemu` (GPLv2, what we changed, link to branch),
   `MeshBench/tlib`, `MeshBench/renode-infrastructure`, `MeshBench/renode`,
   `MeshBench/meshcore-native` (its NOTICE). Each entry: name, upstream,
   branch, one line on the change, full licence text.
3. **Bundled third parties**: Renode's bundled .NET runtime, imgui/cimgui
   (while workbench1 still ships), wgpu-native.
4. **Go libraries**: generated by `google/go-licenses` at build time -
   module, version, licence name, full text. The tool also FAILS the build
   on forbidden/unknown licence types, which is the enforcement half.
5. **Downloaded at runtime, not distributed**: MeshCore (upstream licence,
   stated verbatim), board images, the SoftDevice note (user-supplied,
   cannot be redistributed).
6. **Data attributions**: OpenStreetMap contributors, CARTO, Esri, AWS
   terrarium elevation, with each provider's required wording - the same
   strings the map already shows, listed once here in full.

Mechanics: `make licences` (also a workflow step) runs go-licenses into
`internal/workbench/licences/third_party/` plus a small generator that
merges sections 1-3, 5-6 from checked-in metadata
(`docs/licences.yaml`, hand-maintained, reviewed when repositories.md
changes) into one `licences.json`; `go:embed` both. The panel renders
sections with the design language's cards + a search box; every entry's
full text is expandable. A dev build without the generated file embeds a
stub that says "run make licences" - visible, never silently empty.

Test: a unit test asserts the embedded set is non-stub in release mode, that
every fork in repositories.md's table has an entry, and that section 2
renders above section 4.

## 4. Pipeline changes (package.yml)

1. **Post-cutover cleanup**: drop the second binary; app.ID ldflag on
   `meshbench`; README.txt rewritten (workbench = Gio app; `workbench1` is
   the old one, present but unmaintained).
2. **Licences job step** before Assemble: `make licences`, build fails on
   unknown/forbidden licence, artifact includes a top-level `LICENCES/` dir
   (same content as embedded - some users read files, not windows).
3. **MeshCIM tripwire**: `! grep -ri meshcim dist/` in Assemble.
4. **Checksums**: `sha256sum *.tar.gz > SHA256SUMS`, uploaded; release notes
   template says how to verify.
5. **Smoke test the artifact, not the tree**: unpack the tar into a clean
   container (`docker run ubuntu:24.04`), run
   `./meshbench workbench -fixture fixtures/... -quit-after 20s` headless?
   The workbench needs a display - so the smoke is: `meshbench -version`,
   `meshbench check` (exists), and launching under `weston --headless` +
   WLR_BACKENDS? Gio on a headless Wayland compositor works with
   `weston-headless`; if that proves flaky, the smoke is the CLI surface
   plus `ldd` against the stated floor, and the graphical smoke stays on
   elite before tagging. Written into the job either way: the thing tested
   is the artifact users download.
6. **Matrix**: linux-x86_64 now; arm64 and macos as phases land. Release
   step keys on tags `v*` as today.
7. **Library floor check at startup** (small app change): if a needed .so
   fails to load, say which package to install, by distro family - the one
   support question every Linux binary release gets.

## 5. Acceptance

- A fresh Ubuntu 24.04 VM with nothing installed: download, untar, run
  `./meshbench workbench` - map appears, fixture loads, play runs firmware,
  packet view opens. Same check on Fedora 40.
- Help > Licences shows: the project licence, all five forks at the top with
  their changes named, go-licenses output below, attributions at the bottom.
- The tarball contains `LICENCES/`, `SHA256SUMS` matches, and grep finds no
  meshcim anywhere in it.
- `meshbench -version` prints the tag the release was built from.

## Order of work

1. ADR-0001: Alex chooses the licence (blocks shipping, nothing else).
2. `docs/licences.yaml` + generator + embed + the Licences panel (window
   works in dev builds immediately).
3. package.yml: cutover cleanup, licences step, tripwire, checksums, smoke.
4. Embedded fixtures + emoji font; startup library check; `-version`.
5. Tag `v0.1.0-rc1`, run the acceptance on clean VMs, fix, tag `v0.1.0`.
6. Phases 1b/2 as their prerequisite releases (arm64 emulators,
   darwin-arm64 meshcore-native) appear.
