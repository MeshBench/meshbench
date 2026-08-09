# MeshcoreSim

An RF-accurate MeshCore network simulator.

It runs **real MeshCore firmware** — the actual C++ from `meshcore-dev/MeshCore`,
compiled for the host and driven through its own `Radio` interface — against a
**sample-accurate LoRa baseband channel** with real noise.

The channel does not decide whether a packet arrives. It sums waveforms, applies
path loss over real terrain, adds thermal noise, and lets each receiver's
demodulator find out. Capture effect, partial collisions and sensitivity are
*emergent*, not rules someone wrote down.

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
