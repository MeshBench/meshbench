<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/brand/meshbench-banner-1600x400.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/brand/meshbench-banner-1600x400-light.png">
  <img alt="MeshBench: an RF-accurate MeshCore network simulator" src="docs/brand/meshbench-banner-1600x400-light.png">
</picture>

# MeshBench

**An RF-accurate MeshCore network simulator: real firmware, modelled air.**

[![CI](https://github.com/MeshBench/meshbench/actions/workflows/ci.yml/badge.svg)](https://github.com/MeshBench/meshbench/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/MeshBench/meshbench?display_name=tag)](https://github.com/MeshBench/meshbench/releases/latest) [![Docs](https://img.shields.io/badge/docs-meshbench.github.io-E8500F)](https://meshbench.github.io/docs/) [![Licence: GPL-3.0-or-later](https://img.shields.io/badge/licence-GPL--3.0--or--later-blue)](LICENSE) [![Go](https://img.shields.io/badge/go-1.25-00ADD8)](go.mod)

> [!WARNING]
> **This is not ready for a release.** The repository was made public so that
> other work could be completed against it, not because the project is
> finished. Expect rough edges and expect things to move.
>
> There are also issues with QEMU on emulated boards such as IRQs getting stuck and receiving not working correctly (these do not happen in the native builds).
>
> **Linux is the platform it is developed and tested on.** The other two have
> known compatibility problems, and they are being worked through:
>
> - **Windows**: several open faults, tracked under the
>   [`windows`](https://github.com/MeshBench/meshbench/issues?q=is%3Aissue+is%3Aopen+label%3Awindows)
>   label.
> - **macOS**: the build is not notarised, so the first launch has to be
>   allowed by hand, and no emulator package is published for it, so emulated
>   boards need forks you build yourself.
>
> Builds for those two are worth helping to test. They are not yet worth
> depending on.

## Overview

**MeshBench runs real MeshCore firmware. MeshBench models the air.** Every
node is MeshCore's own code, compiled and running as its own process; what
MeshBench supplies is the radio spectrum, the terrain, the distances, the
noise, and a shared clock. When a packet is not relayed, it is because the
real firmware decided not to relay it.

Reception is judged one of two ways, and the choice is stamped into every
result:

- **Calculated RF** (the default): a link budget against the demodulator's
  floor, with overlapping transmissions summed into the noise. Fast enough
  to sweep a national network: a 300-node flood burst prices in about 46 ms.
- **Waveform RF**: the actual chirps are synthesised as IQ samples, overlaps
  sum coherently, and a real receive chain (preamble lock, frequency
  correction, per-symbol FFT, Gray, deinterleave, Hamming FEC, dewhitening,
  CRC) recovers the frame or does not. **Capture effect and partial
  collisions emerge from the physics**, not from a rule, at 20 to 50 times
  the cost. The chain is held to real silicon by
  [golden vectors](https://meshbench.github.io/docs/golden-vectors.html):
  frames captured off a real SX1262 decode end to end through it.

The channel itself decides nothing in either mode: it produces signal, and
the receiver finds out. [RF simulation](https://meshbench.github.io/docs/rf-simulation.html)
compares the two models; [the RF chain](https://meshbench.github.io/docs/rf-chain.html)
walks the physics they share.

It exists because a link-budget rule cannot answer the questions that
matter: a marginal link, two nodes transmitting at once, a hill in the way.
Network operators use it to plan sites, firmware developers to A/B their
branches on hundreds of nodes, and app developers to test against a mesh
that speaks the real companion protocol.

**Every result is a best case.** The model is kinder than the air: no
multipath, no oscillator error, no body loss. If it says a link will not
work, believe it; if it says a link works marginally, go and measure.
[`docs/shortcomings.md`](docs/shortcomings.md) is the maintained account of
what is not modelled, and it is stated in the interface on every result.

![The plan view: the shipped Scotland and Ireland network, 378 nodes running real MeshCore firmware, showing the 1787 strongest of 4873 links](docs/images/workbench-plan.png)

## Features

- **Two reception models**: calculated link budgets for scale, waveform
  synthesis with a real demodulator for collisions, capture and everything
  arithmetic cannot see. Switchable live; stamped into every saved run.
- **Real firmware on every node**: routing, flood suppression, duty-cycle
  policing and CSMA timing are MeshCore's own, native for hundreds of
  deterministic nodes or emulated from the published `.uf2`/`.bin` images
  under QEMU and Renode.
- **Real terrain**: elevation tiles, ITU-R P.526 diffraction, optional
  buildings; a terrain cut-through with the Fresnel zone explains every
  missed link, in both directions, because reachability is asymmetric.
- **Coverage and planning**: link budgets rasterised over terrain, combined
  across a fleet, and searched for where the next node should go.
- **Import real networks** live from CoreScope or Beacon, with
  transport regions inferred from a week of real traffic.
- **Firmware A/B on one seed**: half the repeaters on one build, half on
  another, same traffic, and diff. Deterministic: same seed, same scenario,
  same answer.
- **An endpoint for your app**: `meshbench serve` exposes a real companion
  over TCP, a serial pty, or Bluetooth. Your client cannot tell it from a
  radio on a desk.
- **Scriptable end to end**: a control socket and three clients (Go,
  Python, Node) in [`pkg/`](pkg/); the
  [cookbook](https://meshbench.github.io/docs/cookbook.html) shows the same
  seven programs in each.
- **Evidence out**: Wireshark live or as pcapng, IQ export and a served
  rtl_tcp stream an unmodified SDR client can tune, waterfall and dechirped
  symbol views, JUnit from the test runner.

## Installation

Download the build for your platform from the
[latest release](https://github.com/MeshBench/meshbench/releases/latest);
nothing else has to be installed first.

```console
# Linux
chmod +x meshbench-*.AppImage && ./meshbench-*.AppImage
```

macOS (`.dmg`) and Windows (`.msi`, or a `.zip` with no installer) builds are
in the same release;
per-platform detail, including the signing caveats, is in
[`docs/install.md`](docs/install.md).

To build from source instead (needs Go 1.25 and a C toolchain with GL and
X11 headers):

```console
git clone https://github.com/MeshBench/meshbench
cd meshbench
go build ./cmd/meshbench
```

## Usage

Open the workbench on a real network and press **Play**:

```console
meshbench workbench
```

Run your MeshCore branch on every repeater in the mesh:

```console
meshbench dev -from ~/src/MeshCore
```

Give your application a mesh and an endpoint that speaks the real companion
protocol:

```console
meshbench serve
```

Run a fixture on real firmware and check its assertions, for CI:

```console
meshbench test -fixture fixtures/fixture-fife-strict.json -junit results.xml
```

Or drive a session from code, in Go, Python or Node:

```python
from meshbench import Workbench

with Workbench.headless(fixture="fife-strict", seed=7) as wb:
    wb.sim.start()
    wb.firmware.wait_started()
    wb.sim.run(timedelta(minutes=5))
    print(wb.assertions.check())
```

The **[documentation site](https://meshbench.github.io/docs/)** takes it
from here: [your first simulation](https://meshbench.github.io/docs/first-simulation.html),
[debugging packet delivery](https://meshbench.github.io/docs/debugging.html),
[running experiments](https://meshbench.github.io/docs/experiments.html),
and the full CLI, control-socket and client references. This repository's
[`docs/`](docs/README.md) keeps the engineering notebook behind it.

MeshBench is **0.x deliberately**, and there is no 1.0 scheduled.
[`docs/compatibility.md`](docs/compatibility.md) says what that means for
anything depending on it: what may break, what is refused at connect rather
than guessed at, and what would have to be true before a 1.0 was worth cutting.
The short version for a script is that a client and the workbench it drives
must be the same release, and a mismatched pair is refused before any verb
runs.

## Board compatibility

Which published board images have actually been run under emulation, and
how far each one got. Every row is a measurement, not a claim: the firmware
is the released image from MeshCore's own releases, and a blank cell means
nobody has watched that board do that thing.

| Board | MCU | Emulator | build | boot | radio | tx | rx | flood | fem | power |
|---|---|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `Generic_E22_sx1262` | ESP32 | QEMU | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `Heltec_t114` | nRF52840 | Renode | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `Heltec_t096` | nRF52840 | Renode | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ? | ✓ |
| `RAK_4631` | nRF52840 | Renode | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `Xiao_nrf52` | nRF52840 | Renode | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `Heltec_mesh_solar` | nRF52840 | Renode | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `Xiao_S3_WIO` | ESP32-S3 | QEMU | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ? |
| `Heltec_v3` | ESP32-S3 | QEMU | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `LilyGo_TDeck` | ESP32-S3 | QEMU | ✓ | ✓ | ✓ | ✓ | ✓ | ✗ | – | ✓ |
| `Ebyte_EoRa-S3` | ESP32-S3 | QEMU | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | – | ✓ |
| `Station_G2` | ESP32-S3 | - | | | | | | | | |
| `Heltec_v2` | ESP32 | - | | | | | | | | |

✓ passed  ✗ failed  – not applicable  ? not measurable yet  blank not attempted

Measured one board at a time on an idle machine. Nine of these rows were
re-measured on 4 September 2026 against `virtual-sx1262` v1.3.0, loaded inside
the emulator, and every one reproduced what it had shown the day before through
the radio server that arrangement replaced. `LilyGo_TDeck` is the exception and
still carries its 3 September run; the two blanks have never been attempted.

What each board's row means in detail is in
[`docs/emulated-published-firmware.md`](docs/emulated-published-firmware.md).

The columns, briefly: **build** is a published image whose digest checks
out; **boot** means the emulator attached and the node did not spend the
run restarting; **radio**/**tx** mean it put its own unprompted advert on
the air; **rx** that it heard another node; **flood** that it *forwarded
somebody else's packet*, judged at the board itself; **fem** that a
front-end module was switched in; **power** that it still answered after
being left idle.

What each ✗ turned out to be, and why it is not the board's fault, is
recorded in [`docs/emulated-published-firmware.md`](docs/emulated-published-firmware.md):
the ESP32-S3 pair's history runs through a flash quad-enable bit, an SPI
controller numbering difference, and a strapping pin read low.

## Contributing

Read [`CONTRIBUTING.md`](CONTRIBUTING.md) first: the house rules are
mechanical and mostly enforced by CI, so knowing them beforehand is quicker
than finding out from a failed run. Bug reports, board reports and "this
number looks wrong" each have their own issue template, because they need
different information.

## License

**GPL-3.0-or-later**: see [`LICENSE`](LICENSE), and
[`docs/licence.md`](docs/licence.md) for the reasoning. Anyone who receives
a MeshBench binary can get its source, study the RF model, and check the
numbers against the code that produced them; for a simulator whose output
is used to argue about real deployments, that is the property that matters
most. Attribution for everything MeshBench links, bundles, downloads or
draws is generated from the build graph: see
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
