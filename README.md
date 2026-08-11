# MeshcoreSim

An RF-accurate MeshCore network simulator.

It runs **real MeshCore firmware** — the actual C++ from `meshcore-dev/MeshCore`,
compiled for the host, driven through **its own radio driver over real
RadioLib** — against a **sample-accurate LoRa baseband channel** with real
noise.

The channel does not decide whether a packet arrives. It sums waveforms, applies
path loss over real terrain, adds thermal noise, and lets each receiver's
demodulator find out. Capture effect, partial collisions and sensitivity are
*emergent*, not rules someone wrote down.

## Where the real ends and the simulation begins

The claim "it runs real firmware" is worth being exact about, because the
interesting bugs live on the boundary.

| Layer | What runs |
|---|---|
| MeshCore application — repeater, companion, room server | **real**, unmodified |
| MeshCore mesh logic — `Mesh`, `Dispatcher`, forwarding policy, CLI, preferences | **real**, unmodified |
| MeshCore radio driver — `CustomSX1262`, `RadioLibWrapper` | **real**, unmodified |
| RadioLib 7.6.0 | **real**, vendored, unmodified |
| The SX1262 chip | **ours** — a virtual chip answering SPI |
| Arduino core, board, flash filesystem, RTC, sensors, RNG | **ours** — the platform |
| The air — modulation, path loss, noise, capture effect | **ours** — this is the simulator |

**Nothing in MeshCore is patched.** The build points at a MeshCore checkout and
compiles it as it stands. What is substituted is the platform beneath it, which
is hardware rather than firmware — and, at the bottom, the air, which is the
entire point of the exercise.

The line moved recently and it mattered. The radio driver used to be ours too: a
hand-written stand-in for the whole stack. That made every listen-before-talk
decision the simulator's rather than the firmware's, and two MeshCore versions
differing *only* in their radio driver produced bit-identical results across
twelve runs — which read as "the change does nothing" when it actually meant
"the changed code never ran". See `docs/virtual-sx1262.md`.

### What the boundary still costs

- **A virtual chip is a model of a chip.** Everything above the SPI pins is the
  software that runs on hardware. What answers those pins is our best
  understanding of an SX1262, so real errata and a real preamble detector's
  behaviour under interference remain invisible.
- **We compile as an nRF52.** Where MeshCore takes a platform-specific path, we
  take that one. A bug that only appears on ESP32 will not appear here.
- **The simulator is kinder than the air.** No multipath, no body loss, no
  oscillator error. Every result is a best case — which is what makes it usable:
  if it does not work here, it will not work outdoors.

## What it is for

Answering **why**, not just whether:

- Why did that packet miss? — terrain cut-through with the Fresnel zone and each
  diffracting edge's individual loss.
- What did the air actually look like? — waterfall, IQ, and a dechirped symbol
  view showing which of two colliding frames captured.
- Did that commit break relaying? — run half the repeaters on one firmware build
  and half on another, same traffic, and diff.
- Does my app work on a 40-node mesh? — attach a real companion client over TCP.

## Status

Early. Design settled, implementation starting. Decisions live in Plane project
**MSIM** as `ADR-0001` … `ADR-0009`, with a UX spec and the baseband technical
design alongside them.

## Building and running

Needs a GPU and a display. **It does not run on the dev VM** (virtual VGA, no
display) — develop there, run on a Mac. CI exercises the CPU reference path,
which per ADR-0004 is a maintained oracle rather than a fallback.

```bash
go test ./...        # CPU reference path
go run ./cmd/meshcoresim
```

## Honesty about the model

The simulator is **kinder than the air**. It does not model multipath or fading,
Doppler, oscillator ppm error, body loss, or non-LoRa interference beyond a flat
floor. Every one of those makes real links worse than simulated ones, so the bias
is one-directional — and it is stated in the UI, not buried here.

Sensitivity is validated against Semtech's published SX1262 figures: simulated
1% PER must land within ~2 dB across SF7–SF12. If that test fails, the chain is
wrong however convincing the waterfall looks.

## Wireshark

Captures carry every receiver's view of every frame, live over loopback UDP or
saved as pcapng. The MeshCore protocol itself is dissected by
[aaronb/wireshark-meshcore](https://github.com/aaronb/wireshark-meshcore)
(GPL-2.0-only), vendored in `tools/dissector/` beside our own metadata layer —
see `tools/dissector/README.md`.
