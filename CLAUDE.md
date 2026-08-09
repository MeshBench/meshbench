# MeshcoreSim

An RF-accurate MeshCore network simulator. It runs **real MeshCore firmware**
against a **sample-accurate LoRa baseband channel** with real noise, so the
question it answers is not "would a packet get through" but "what actually
arrived at the antenna, and why".

**Private, A13xB0. No licence chosen yet** — see ADR-0001. Do not add a LICENSE
file or make this public without an explicit decision; MeshCore's own licence is
linked into our binary (ADR-0002) and constrains the choice.

## Tracking

Plane project **MSIM** at http://plane.lab. Work items are `MSIM-<n>`; that ID
goes in the branch name and the PR title. ADRs live in the project's Pages.

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

```
cmd/meshcoresim/   the binary
internal/rf/       channel: sum, delay, noise
internal/dsp/      modulation, demodulation, FFT — CPU reference and GPU
internal/antenna/  patterns, orientation, polarisation
internal/terrain/  DEM tiles, profiles, diffraction
internal/firmware/ host builds of MeshCore, the Radio shim, per-node runtime
internal/scenario/ nodes, import, persistence, seeds
internal/capture/  pcapng, event log
internal/sdr/      IQ export, SigMF, streaming
internal/companion/ TCP and PTY companion transports
internal/ui/       ImGui panels
shaders/           WGSL compute shaders
tools/dissector/   Wireshark Lua dissector
```

`internal/` by default. Deliberately not `golang-standards/project-layout` — it
is unofficial, disclaims itself, and Go maintainers have criticised it.

## Limits

Mechanical, because taste does not survive scale — and this codebase will be big.

| Rule | Limit |
|---|---|
| File length | 300 lines soft, **500 hard** |
| Function length | 50 lines soft |
| Nesting depth | 4 |
| Dead code | none — git remembers |
| Speculative abstraction | none — write the interface at the *second* implementation |
| New dependency | justify it in the PR, one line |
| Comments | explain *why*, never *what* |

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
  oscillator error. Say so in the UI — never let a user assume otherwise.
- **Determinism is a feature.** Same seed, same scenario, same result. Use
  counter-based RNG, never a stateful stream shared across goroutines.

## Running

Needs a GPU and a display: **this does not run on VM 114** (virtual VGA, no
display). Develop here, run on a Mac. The CPU path is what CI exercises.
