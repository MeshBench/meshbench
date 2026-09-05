# MeshBench

`MeshBench/meshbench` — an RF-accurate MeshCore network simulator. It runs **real MeshCore firmware**
against a **sample-accurate LoRa baseband channel** with real noise, so the
question it answers is not "would a packet get through" but "what actually
arrived at the antenna, and why".

**GPL-3.0-or-later** - see `docs/licence.md`. The repository is public, so
section 6's source offer is already met by the tree a binary was built from
sitting at the same tag as the binary. The pipeline attaches a source archive
to every release anyway, because an archive pins the exact tree where a tag can
be moved or deleted; dropping it is a decision to record in `docs/licence.md`,
not a tidy-up. MeshCore is *not* linked into our binary: it is built in
`MeshBench/meshcore-native` and downloaded at runtime, which is what freed the
licence choice, so do not reintroduce a direct dependency on it without a new
ADR.

## Tracking

Plane project **MSIM** at http://plane.lab, which is an instance on the
maintainer's own network and reachable from nowhere else. Work items are
`MSIM-<n>`; that ID goes in the branch name and the PR title. ADRs live in the
project's Pages. An outside contributor needs none of it: `CONTRIBUTING.md`
routes them through GitHub issues and pull requests, and a PR title without an
`MSIM-<n>` is what a change from outside looks like.

## Two files, one set of rules

`CONTRIBUTING.md` states these same rules for somebody arriving cold, with the
reasoning spelled out. **This file is the authority**: it is what the build
enforces and what a change is held to. If the two ever disagree, that is a bug
in `CONTRIBUTING.md`, and the fix is to change it rather than to argue.

## Style guide

**Effective Go** plus the **Google Go Style Guide**. Where they are silent and
the Uber Go Style Guide is specific, follow Uber.

Enforced in CI, not by review:

```bash
gofmt -l .          # must be empty
go vet ./...
golangci-lint run
go test ./...
```

## Layout

Nine layers. **A package may import its own layer and everything below it,
never anything above.** `internal/layers_test.go` fails the build otherwise, so
this is a check rather than a description, and `internal/layoutmap_test.go`
fails when the map below stops matching the tree.

```
cmd/meshbench/    the binary

internal/diag/      opt-in diagnostic logging, chosen by domain (MESHBENCH_LOG)

internal/rf/        radio physics — knows nothing of nodes, networks or the app
  geo/                great-circle distance and bearing, once
  channel/            sums waveforms, applies gain and delay, adds noise
  dsp/                modulation, demodulation, FFT — the CPU reference
  gpu/                the GPU twins, and the .wgsl they embed
  lora/               the coding chain: payload bytes to chirp symbols
  antenna/            patterns, orientation, polarisation
  terrain/            DEM tiles, profiles, diffraction
  environ/            buildings, heights, materials
  propagation/        path loss: heights in, decibels out, and the CPU twins

internal/mesh/      MeshCore itself: what a node is and what it says
  shim/               the Radio shim the emulated firmware links against
    repeater/           the C headers a host build of simple_repeater would
                        need, paused: nothing compiles or links them today
  companion/          TCP and PTY companion transports
  proto/              the companion protocol codec
  packet/             the MeshCore frame: what the bytes on the air mean
  energy/             battery, load, solar

internal/firmware/  running real firmware against the radio, native or emulated;
                    the native host build and the emulator runner, being split
                    into subdomains as the emulation domain lands
  board/              one board_<name>.go per real board, its wiring and parts
  native/             the native backend: MeshCore built for this host, run
                      as a child process
  fakenative/         a stand-in for that child process, so the lifecycle
                      around it can be tested without a MeshCore build
  emulated/           the emulator runner: the node, the flash images, and what
                      a published image boots against
    qemu/               the QEMU -machine argument, composed from a board's wiring
    renode/             the Renode script fragments for the nRF52 boards
    peripheral/         the models a board wires - display, buttons, keyboard,
                        GPS, serial - each addressed by pin
  console/            the operator's terminal onto a running node

internal/world/     what is being simulated, and where it came from
  scenario/           nodes, region, boundary - the seed belongs to the run,
                      and lives in sim/engine.Config
  provider/           CoreScope and Beacon feeds
  boundary/           named administrative areas
  basemap/            hillshaded terrain under the simulation
  sdr/                IQ export, SigMF, streaming

internal/sim/       running it, and recording what happened
  engine/             firmware nodes exchanging traffic over the channel
  boardcheck/         what a board demonstrates, measured one boot at a time
  capture/            pcapng, event log, the reception ledger
  replay/             observed traffic back into transmissions

internal/study/     the questions asked of a simulation
  coverage/           link budgets into rasters, combined and searched
  linkbudget/         path loss into a margin
  planning/           where the next node should go
  pathview/           why a link missed
  validate/           predicted against actually heard

internal/app/       orchestration, no toolkit
  version/            what this build is, stamped by the release pipeline
  state/              the store, and the snapshots the renderer reads
  session/            the workbench without a user interface
    study/              coverage and validation verbs, wired in by init
    provisioning/       the provisioning settings verbs, wired in by init
    resources/          the runtime-resource verbs and the first-run
                        readiness check, wired in by init
    board/              the board-probe and screenshot verbs, wired in by init
    boundary/           the study-area verbs, wired in by init
    capture/            the frame-capture verbs and Wireshark launch, wired in by init
    environ/            the environ.fetch verb, wired in by init
    experiment/         the A/B matrix: the arms, the seeds, the cells that run
                        real MeshCore under each, and the report that comes
                        back, wired in by init. The arm itself stays in core,
                        beside SweepArm, because provisioning describes the
                        same configuration under test
    sweep/              the sweep verbs and their offered-load plan, wired in by init
    firmwarelib/        the firmware library: what builds exist on disk and
                        upstream, what each node is pinned to, and the verbs
                        that scan, download, build and pin them, wired in by
                        init. The start gate stays in core, because playready
                        and runkind ask it whether a run may begin
    fleet/              the bulk node verbs, wired in by init
    inventory/          the node and event listing verbs, wired in by init
    links/              the link pair and profile verbs, wired in by init
    mapview/            the map's own verbs - where the camera looks, what is
                        drawn under it, what a click does and where a dragged
                        node lands, wired in by init
    nodeantenna/        the antenna verbs - what a node stands under and which
                        way it points, wired in by init
    nodelookup/         the node-finding verbs (search, near), wired in by init
    nodewipe/           node.wipe - one node back to factory, wired in by init
    sdr/                the rtl_tcp observer server, state off Sim via the seam
    updates/            the update verbs - whether the release feed may be
                        asked, asking it, and fetching what it names, wired
                        in by init
    domains/            blank-imports the split-out domains so their init runs
  fixture/            the on-disk form of a whole setup
  resource/           what is downloaded at runtime, and what it cost the disk
  update/             whether a newer release exists, and getting it onto the
                      disk beside this build rather than over it
  control/            the unix socket another process drives it by

internal/ui/        Gio — the only layer permitted a toolkit
  theme/              every colour, size and face, named once and nowhere else
    brandfont/          the three faces the identity is set in, embedded
    brandmark/          the wordmark, in the two grounds it has to sit on
  comp/               the widgets every view is built from, the map view among
                      them - there is no separate mapview package
  shell/              the window, the view switcher, the menus, the status bar
  desktop/            what the host desktop decided: cursor theme and size
  float/              keeping a window above the others, where the platform lets
                      a client ask
  pick/               the file dialog the platform already has
  uitest/             the harness a panel test drives one panel through: real
                      frames, clicks and typing, with no window. Its own
                      package because a _test.go cannot cross a package
                      boundary and the panels no longer live in one
  workbench/          the workbench itself: panels, state, wiring
    boardview/          one board in full: its panel live at a whole-number
                        scale, the controls for everything it has wired, and
                        what its profile declares beside what the firmware left
                        in the chip, with a verdict on each row. A window rather
                        than a tab, and the firmware developer's half of what
                        the Hardware tab shows
    nodeview/           one node: the list, the node window and all its tabs -
                        radio, antenna, hardware, console, output, companion -
                        the boards panel and the build picker
    packetview/         the packet inspector: the panel, the journey graph, the
                        dissection and the hex. Named for the view rather than
                        the packet, because internal/mesh/packet is the frame
                        itself and two packages called packet is one too many
    licences/           the licence inventory Help > Licences shows, generated
                        by tools/licgen and committed

pkg/                the public surface, for a fork or an app to import
  client-go/          the Go client and its runnable examples
  client-python/      the Python client, its pytest plugin and examples
  client-js/          the Node client, the same shape with no build step

tools/              what builds, generates and drives it, none of it shipped
  clientgen/          the closed enum sets all three clients need, from the one
                      place that defines them
  verbdoc/            docs/scripting-verbs.md and the verb counts in the prose,
                      regenerated from the verbs the tree registers
  flagdoc/            docs/cli-reference.md, built from the binary's own help,
                      plus the authored note of what each flag is for
  licgen/             the licence inventory the workbench embeds
  skillmirror/        the standalone skill repository, a plugin marketplace
                      rendered from .claude/skills so it cannot drift from it
    readme/             each mirror's front page, the one thing it carries that
                        the canonical tree has no place for
  envgen/             building footprints into environment tiles
  mockup/             the UX wireframes in docs/ux, rendered
  render/             the figures in docs/output, from the real engine
  goldencap/          a wideband capture down to chip-rate baseband, and the
                      analysis on it
  native/             builds and runs the native host firmware backend
  armfw/              the SoftDevice-free nRF52840 image the Renode backend boots
  renode/             MeshCore's published nRF52 binaries under Renode, the
                      cross-check on the native build
    peripherals/        the C# models Renode's own nRF52840 platform is missing:
                        clock calibration, CryptoCell, SAADC, TWIM, the USB
                        device controller and the host at the other end of it,
                        the radio
  esp32/              the ESP32 half of the emulated backend: Espressif's QEMU
                      fork, which Renode has no platform for
  platformio/         a post-build hook, copied into a MeshCore checkout, that
                      hands a fresh image to a running workbench
  firmware-ab/        two firmware builds against the same traffic
  ble/                a simulated node as a real BLE peripheral, so an
                      unmodified companion app connects to it
  headless/           a harness for driving a headless session
  soak/               drives a running workbench and judges what it heard
  dissector/          the Wireshark Lua dissector, ours and the vendored one
  internal/           what only the tools use, said structurally
    mockup/             the drawing toolkit tools/mockup renders the UX
                        wireframes with - shapes, text and a canvas, and
                        nothing that runs at application runtime
```

The WGSL lives in `internal/rf/gpu/` because `//go:embed` cannot reach outside
its own package directory. There is no top-level `shaders/`; a second copy there
went stale, which is how we found out.

This table is the map. A new package updates it in the same commit — the map
being wrong is worse than the map being short. `internal/layoutmap_test.go`
reads it in both directions, so a package with no row and a row naming a
package that has gone both fail on the machine that moved it. Every directory
under `internal/` and `tools/`, at any depth, needs a row; `pkg/` is mapped one
level down, at the client rather than inside it.

`internal/` for everything private - the nine layers - and `pkg/` for the small
public surface a fork imports (the clients). Not
`golang-standards/project-layout` wholesale — it is unofficial, disclaims
itself, and Go maintainers have criticised it — but `pkg/` earns its place as
the one boundary between what outsiders may import and what they may not.

## Limits

Mechanical, because taste does not survive scale — and this codebase will be big.

| Rule | Limit |
|---|---|
| File length | 300 lines soft, **500 hard** |
| Function length | 50 lines soft |
| Nesting depth | 4 |
| Dead code | none — git remembers |
| Filename | says what the file holds — never a plan phase, never a `2`/`b`/`c` suffix |
| Panel & widget files | one type per file, named after the type |
| Duplicated asset | none — one copy, and the code points at it |
| Build artifacts | never tracked |
| Speculative abstraction | none — write the interface at the *second* implementation |
| New dependency | justify it in the PR, one line |
| Comments | explain *why*, never *what* — and never cite a plan phase or ticket number, which the reader will not have |

## Domain rules that are easy to get wrong

- **The channel does not decide anything.** It sums waveforms and adds noise.
  Whether a packet decodes is the demodulator's business. Never add a rule like
  "if two transmissions overlap, both fail" — capture effect must emerge, or the
  simulator is just a packet model with extra steps.
- **Every GPU kernel has a CPU twin, and they are tested against each other.**
  A wrong FFT does not crash; it produces a plausible waterfall and slightly
  wrong sensitivity, and nobody notices for months.
- **Reachability is asymmetric.** Compute and present both directions. A result
  that does not say *which direction* is wrong even when the arithmetic is right.
- **Antenna gain is directional.** Evaluate the pattern towards the far end,
  per direction: A to B and B to A are different bearings and the two answers
  may differ. A scalar "gain" field is a bug. Azimuth is never optional. The
  bearing between two known positions is exact, and a yagi or a sector is 20 dB
  or more down off its boresight, so peak azimuth gain is not an approximation
  but a confident wrong answer in the optimistic direction. Elevation is
  decided by what the caller actually knows. Where both ends' altitudes are in
  hand, the engine and the coverage raster use the real look angle, because a
  repeater on a hill is not on its boresight to a node 5 km below it. Where
  they are not - the link budget and the validation residual, which are handed
  positions and decibels - the far end is taken as on the boresight in
  elevation: on a terrestrial path that angle is a fraction of a degree, and
  inventing one would claim a precision the geometry cannot support.
- **Position uncertainty propagates.** A node imported at ±5 km does not get a
  confident answer. Inherited from hamreach HAM-34, learned the hard way.
- **Airtime must match the firmware's own `getEstAirtimeFor()`.** The firmware's
  CSMA timing is built on it; if our channel disagrees, the two desynchronise
  silently.
- **The simulator is kinder than the air.** No multipath, no body loss, no
  oscillator error. Say so in the UI — never let a user assume otherwise, and
  keep `docs/shortcomings.md` honest as the model changes. The measured biases
  are nearly all in one direction, which is what makes a result usable: treat it
  as a best case.
- **Determinism is a feature.** Same seed, same scenario, same result. Use
  counter-based RNG, never a stateful stream shared across goroutines. **With
  one stated exception: a node running in an emulator.** Its firmware is a
  published image with nothing in it that could receive the engine's tick - what
  answers the tick is the chip model on our side of the socket - so the guest
  runs against QEMU's or Renode's clock, which runs on the host's, and the same
  seed does not put its traffic at the same instants twice. That is a property
  of running an unmodified image, not a bug to be filed, and it is why native is
  the backend for anything being compared. What is not permitted is leaving it
  implicit: `scenario.NotReproducible` is the one sentence saying so, `sim.state`
  and `experiment.start` answer it, and the sweep says it before the machine
  time is spent as well as over the results.
- **A board profile is a transcription of a real board — read its documentation
  first.** When adding or fixing a `board_<name>.go`, work from the
  manufacturer's own pinout, not from another board or from memory: the LilyGo
  T-Deck / T-Deck Plus wiki (`wiki.lilygo.cc`), the Heltec docs, the vendor
  schematic. Every pin the profile declares — radio SPI/NSS/BUSY/DIO1, the
  display CS/DC/backlight and controller, the card CS, the trackball and
  keyboard, the peripheral-power enable (e.g. the T-Deck's GPIO10) — has a
  right answer on that page, and a wrong one is silent: the board boots and a
  peripheral reads as absent. Cross-check against the firmware's own variant
  (MeshCore `variants/<board>/`, Meshtastic `variants/esp32s3/<board>/`) for
  which controller and I2C bus it actually drives, because two firmwares for one
  board can differ and both be right. Cite the source in the profile's comment.

## It is an application, and it runs standalone

A native desktop application — Go and Gio. **Not a web
application**, not a browser tab, not a local server with a front end pointed at
it. If a design starts describing endpoints, sessions or a front end, it has
drifted.

One binary on the operator's machine. **No service to deploy, no remote worker,
no compute backend.** "On the GPU" always means the GPU in the machine
that is running it, and every GPU path has a CPU one that produces the same
answer more slowly — a machine without a usable GPU loses time, not features.

The only thing that crosses the network is *data*: terrain tiles, and the
optional CoreScope and Beacon feeds. Terrain caches permanently and has an
offline mode that fails loudly; the feeds are all optional. Nothing in the
simulation depends on anything we run.

## Where development happens

Machine-specific: which host, what it can run, and what the lab runners are for
are in `docs/development-machines.md`. It is kept separate because it goes
stale on its own schedule and none of it is a rule.

Two things from it are worth knowing before reading anything above: **the
emulated boards are run one at a time**, because several at once will take a
twelve-core machine down, and **`golangci-lint` must match the version CI
pins**, or it will disagree with CI about whether the tree is clean.
