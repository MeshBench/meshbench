# MeshBench

`MeshBench/meshbench` — an RF-accurate MeshCore network simulator. It runs **real MeshCore firmware**
against a **sample-accurate LoRa baseband channel** with real noise, so the
question it answers is not "would a packet get through" but "what actually
arrived at the antenna, and why".

**GPL-3.0-or-later** — see `docs/licence.md`. The repository is still
private; publishing binaries from a private repository means each release
carries a source archive, which the pipeline does. MeshCore is *not* linked into
our binary — it is built in `MeshBench/meshcore-native` and downloaded at
runtime, which is what freed the choice — so do not reintroduce a direct
dependency on it without a new ADR.

## Tracking

Plane project **MSIM** at http://plane.lab. Work items are `MSIM-<n>`; that ID
goes in the branch name and the PR title. ADRs live in the project's Pages.

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

Seven layers. **A package may import its own layer and everything below it,
never anything above.** `internal/layers_test.go` fails the build otherwise, so
this is a check rather than a description.

```
cmd/meshbench/    the binary

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
  firmware/           host builds, per-node runtime
  shim/               the Radio shim the emulated firmware links against
  companion/          TCP and PTY companion transports
  proto/              the companion protocol codec
  packet/             the MeshCore frame: what the bytes on the air mean
  console/            the operator's terminal onto a running node
  energy/             battery, load, solar

internal/world/     what is being simulated, and where it came from
  scenario/           nodes, region, seed; one board_<name>.go per board
  provider/           CoreScope, Beacon and MQTT feeds
  mqtt/               the paho client behind provider.Subscriber
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
  fixture/            the on-disk form of a whole setup
  resource/           what is downloaded at runtime, and what it cost the disk
  control/            the unix socket another process drives it by

internal/ui/        Gio — the only layer permitted a toolkit
  theme/ comp/ mapview-in-comp/ shell/ desktop/ float/ pick/
  theme/brandfont/    the three faces the identity is set in, embedded
  workbench/          the workbench itself: panels, state, wiring

pkg/                the public surface, for a fork or an app to import
  meshtest/           a MeshCore network inside somebody else's test - it exists
                      to be imported, which is why it is not internal/
  client-go/          the Go client and its runnable examples
  client-python/      the Python client, its pytest plugin and examples

tools/dissector/    Wireshark Lua dissector
tools/soak/         drives a running workbench and judges what it heard
tools/internal/     what only the tools use, said structurally
```

The WGSL lives in `internal/rf/gpu/` because `//go:embed` cannot reach outside
its own package directory. There is no top-level `shaders/`; a second copy there
went stale, which is how we found out.

This table is the map. A new package updates it in the same commit — the map
being wrong is worse than the map being short.

`internal/` for everything private - the seven layers - and `pkg/` for the small
public surface a fork imports (`meshtest`, the clients). Not
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
- **Antenna gain is directional.** Evaluate the pattern in the true direction to
  the far end, per direction. A scalar "gain" field is a bug.
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
  counter-based RNG, never a stateful stream shared across goroutines.
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
optional CoreScope, Beacon and MQTT feeds. Terrain caches permanently and has an
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
