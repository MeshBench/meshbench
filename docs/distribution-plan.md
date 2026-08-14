# Distribution: install, open, go

The bar: a user on Windows, macOS or Linux installs MeshBench the way they
install anything else on their platform, opens it, and it works - map, fixture,
firmware, packet view - with nothing else to find or configure. This plan is
how every channel (Homebrew, apt, AUR, Flathub, winget, plain downloads) gets
there without each one becoming its own packaging project.

Builds on `docs/release-packaging-plan.md`, which built the pipeline, the
licence window and the Linux tarball. This is the spread outward from that.

## The one architectural change everything else leans on

Today the tarball carries the emulators inside it. That works for a tarball
and for nothing else: Homebrew, apt, AUR, Flathub and winget all want to
package *our* program, not a 100 MB zoo of prebuilt QEMU/Renode/.NET binaries
that their audits, their licence scanners and their update cadences were never
designed for - and a Flatpak can't even assume the bundled binaries' library
floors exist inside its runtime.

MeshBench already solves this exact problem for firmware: built in a MeshBench
repo, published as releases, **downloaded on first use, cached, announced in
the jobs strip**. The emulators become the same thing:

- An **emulator catalogue** beside the firmware one (`internal/firmware`'s
  download pattern is the template): knows the release URL per fork, the
  artifact name per OS/arch (the `meshbench`-marked names), a SHA-256 pinned
  at MeshBench build time, and the cache location
  (`~/.cache/meshcoresim/emulators/<name>-<version>/`).
- First time a scenario needs an emulated board: "fetching QEMU (meshbench
  build), 38 MB" in the jobs strip, verify checksum, unpack, run. Never again
  until the pinned version changes. Estimates said before spending, per the
  design language.
- No emulator releases for the platform yet → the same honest line the
  Firmware Library already uses: the board is listed, marked "emulator not
  available on macOS yet", nothing pretends.

Every package on every channel then ships the same thing: **one binary plus
its licences**. The tarball keeps a `-full` variant with emulators pre-seeded
into the same layout, for offline use and for people who distrust downloads -
the cache format is the interface, so both paths exercise identical code.

## What "works with pretty much everything" requires per platform

### The binary itself

- **Delete workbench1 + `internal/ui` first.** cimgui's prebuilt archive is
  what pins glibc 2.38 and drags a second GUI stack through every platform's
  build. Removing it (one commit, long planned) drops the Linux floor to the
  runner's toolchain (2.35 on ubuntu-22.04), removes the biggest audit
  surface, and leaves Gio + wgpu - which build clean on all three OSes.
- Already done and staying: emoji font beside the binary, fixtures beside the
  binary, licences embedded and beside the binary, version stamped, app ID
  `io.github.meshbench.meshbench`.
- Still to do from the first plan: the startup library check that names the
  missing distro package instead of failing to launch (Linux only; macOS and
  Windows ship everything in-process).

### Linux

Channels, in the order they pay off:

1. **AppImage** (primary download): one file, double-click, no install. Built
   from the same tree as the tarball with linuxdeploy; bundles the system
   libs above the floor; carries the `.desktop` file and icon. This is the
   "just give me the thing" artifact for every distro at once, arch included.
2. **AUR** (`meshbench-bin`): a PKGBUILD that repacks the release tarball.
   Covers Arch, Manjaro, EndeavourOS with the install command those users
   actually type (`yay -S meshbench-bin`). Maintained by us via a release
   automation (an action that bumps pkgver + sha and pushes to AUR).
3. **Own apt repository** (`deb [signed-by=...] https://apt.meshbench.dev`
   or GitHub Pages): one `.deb` per arch built on the oldest supported base,
   Depends on the real library floor, `.desktop` + icon included. This is
   `apt install meshbench` without waiting years for Debian proper.
   Debian/Ubuntu *official* archives are explicitly a non-goal: cgo,
   runtime-downloading software and a fast release cadence are three things
   distro policy is built to reject; the honest route is our repo.
4. **Flathub** (the long-term consumer answer): the runtime solves libraries
   forever, the store gives discoverability and updates. Needs: public repo,
   appstream metainfo, the app ID we already chose, and the emulator
   downloads working (sandbox: network fine, spawning bundled emulators from
   the cache fine - they're user-space processes; no talkback needed).
   Do it after the emulator catalogue lands, not before.
- rpm/COPR for Fedora: same shape as the apt repo; do it when someone asks,
  the AppImage covers Fedora meanwhile.

### macOS

**Deferred until there is a self-hosted Mac runner** (Alex, 14 August 2026).
Hosted macOS minutes are billed at 10x on private repositories, and MeshBench
is private; the emulator fork is public so its macOS legs are free, but
`aarch64-apple-darwin` has never built there anyway. Both are parked rather
than deleted - re-enabling each is one line. Everything below is what happens
once the runner exists, and none of it blocks Linux or Windows.

- **A real `.app` bundle** (`gogio` produces one for Gio apps; the CLI stays
  reachable as `meshbench` via a symlinked helper), packed into a `.dmg`.
- **Sign and notarise from day one of public release.** An Apple Developer ID
  (USD 99/yr) is the difference between "drag to Applications" and a support
  queue full of quarantine workarounds. Unsigned rc builds are fine while the
  repo is private; the public v1 should not ship unsigned.
- **Homebrew**: start with our own tap - `brew install meshbench/tap/
  meshbench` (cask for the .app; the formula-vs-cask choice follows from
  shipping a bundle). A tap needs nothing from anyone and automates with the
  standard bump action. homebrew-core/cask-main later, once public and once
  the notability bar is met - not a launch dependency.
- arm64 first (`macos-14` runner). Intel via `macos-13` only if telemetry or
  requests justify it - MeshCore's own audience skews recent.
- Emulators on macOS: meshcore-native needs darwin-arm64 host builds
  published (native nodes), then the Renode fork portable for darwin (Renode
  upstream supports macOS; our portable bundling extends), QEMU fork last.
  The catalogue's honesty line covers the gap meanwhile.

### Windows

Ordered by what unblocks what:

1. **radioserver's QEMU transport**: the Unix-socket path needs the TCP path
   the Renode side already has (the firmware bridge is already TCP -
   `internal/firmware/bridge.go` listens on TCP today). This is the one real
   code blocker.
2. **The app itself**: Gio's Windows backend is pure syscalls and wgpu ships
   Windows static libs, so the binary is a native-runner build, not a port.
3. **Artifacts**: a plain `.zip` first (unzip, run `meshbench.exe`), then an
   installer (WiX MSI) once the .zip proves out - the installer is what
   winget prefers and what puts the Start Menu entry and uninstaller in
   place.
4. **Channels**: **winget** (the built-in one - manifest PR per release,
   automated by winget-releaser) and **Scoop** (a bucket file, trivial to
   maintain, loved by the dev audience). Chocolatey: skip unless asked.
5. Code signing on Windows (EV/OV cert) is expensive and SmartScreen will
   growl at an unsigned installer for a while; accept the growl at first,
   revisit when there's revenue or volume.
- Emulator builds for Windows: Renode upstream supports Windows so the fork
  portable extends; the QEMU fork via MSYS2/mingw is known territory. Both
  wait on the transport work; the catalogue's honesty line covers the gap.

## Release engineering that spans all of it

- **One version, one tag, every artifact from one workflow run**: the matrix
  grows legs per OS (`ubuntu-22.04`, `ubuntu-24.04-arm`, `macos-14`,
  `windows-2025`), each leg produces its artifact + licences, one release
  step collects everything plus `SHA256SUMS` and the source archive.
- **Channel automation is release-triggered**, never manual: AUR bump action,
  Homebrew tap bump, winget-releaser, apt repo publish. A channel that needs
  a human forgetting things is a channel that rots.
- **The smoke test grows with the matrix**: each leg unpacks its own artifact
  and runs `meshbench workbench -version` plus the licence-files check;
  Linux adds the clean-container run; the graphical acceptance stays manual
  on real machines per platform before tagging.
- **In-app update notice** (not an auto-updater): a startup check against the
  releases feed - off by default for packaged installs (their manager owns
  updates), on for AppImage/zip/tarball, always just a notice + link. The
  channels that own updating keep owning it.
- **Public repository is the gate for every channel.** AUR, Homebrew, winget,
  Flathub and an apt repo all fetch artifacts anonymously. Until the repo
  goes public, everything above can be built and rehearsed against a private
  release, but nothing ships. (GPL §6 source archives already ship beside
  binaries and simply continue.)

## Order of work

1. Emulator catalogue + runtime download/cache/checksum (Linux first, where
   fork releases exist to test against). The `-full` tarball variant lands
   here too. **Prereq: the forks publish their meshbench-marked releases.**
2. Delete workbench1 + `internal/ui`; drop the glibc floor; simplify every
   downstream build.
3. Desktop integration files (`.desktop`, icon, appstream metainfo under the
   existing app ID) - shared by AppImage, apt, Flathub.
4. AppImage + AUR + apt repo (Linux consumer channels).
5. macOS leg, **once the self-hosted Mac runner exists**: .app/.dmg, tap,
   signing. meshcore-native darwin builds in parallel (separate repo,
   separate work).
6. Windows leg: radioserver TCP transport → zip → MSI → winget + Scoop.
7. Flathub, once 1-4 are stable.
8. Acceptance per platform, then the public v1 release fans out to every
   channel from one tag.

## Acceptance (the "open and go" test, per platform)

A machine with nothing developer-ish installed:

- **Ubuntu/Fedora/Arch**: install via AppImage double-click, `yay`, or `apt`
  → icon in the launcher → opens to the map → place nodes, play, firmware
  downloads announce themselves, packets flow. No terminal required.
- **macOS**: `brew install` or drag from the .dmg → Gatekeeper says nothing
  (signed) → same flow.
- **Windows**: `winget install meshbench` or the MSI → Start Menu → same
  flow.
- Every platform: Help > Licences shows the full inventory; `-version`
  matches the tag; deleting the cache and relaunching redownloads with
  announcements and works offline afterwards for everything except tiles it
  has not seen.
