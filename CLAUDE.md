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
  oscillator error. Say so in the UI — never let a user assume otherwise, and
  keep `docs/shortcomings.md` honest as the model changes. The measured biases
  are nearly all in one direction, which is what makes a result usable: treat it
  as a best case.
- **Determinism is a feature.** Same seed, same scenario, same result. Use
  counter-based RNG, never a stateful stream shared across goroutines.

## It is an application, and it runs standalone

A native desktop application — Go and imgui (ADR-0005). **Not a web
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

**elite** (`alex@10.100.72.98`), at `~/Documents/projects/meshcoresim`. Twelve
cores, a real GPU, and the Renode and QEMU toolchains for the firmware work.
The full suite with `-race` runs there in about three minutes; on the VM it is
several times that and the GPU path cannot run at all.

Clone with `gh repo clone MeshBench/meshbench`. `golangci-lint` is pinned to the
version CI uses (v2.1.6) at `~/go/bin`.

## Running

Needs a GPU and a display: **this does not run on VM 114** (virtual VGA, no
display). Develop here, run on a Mac. The CPU path is what CI exercises.
