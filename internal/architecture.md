# Architecture

How MeshBench fits together, and why each seam is where it is. Read this
before writing code; the ADRs in Plane project **MSIM** carry the reasoning,
this carries the shape.

## The one-paragraph version

Real MeshCore firmware runs in-process, one instance per node, driven by a
scheduler. Each node's `Radio` implementation hands transmitted frames to an RF
engine that converts them to complex baseband, applies terrain path loss and
antenna gain, sums every concurrent transmission at every receiver, adds thermal
noise, and demodulates. Nothing decides whether a packet "arrives" — the
demodulator finds out.

## Data flow, one transmission

```
firmware (MeshCore C++)
    │  startSendRaw(bytes, len)
    ▼
SimRadio ──────────────────────────────────────────────► scheduler
    │  frame + node id + sim time
    ▼
modulator          bytes → LoRa symbols → complex baseband (2^SF samples/symbol)
    │
    ▼
for each receiver j:
    ├── link budget    TX power − feedline + antenna gain(direction)
    │                  − path loss(terrain, diffraction) + RX gain − feedline
    ├── delay          distance / c, in samples (sub-sample → phase)
    └── accumulate     y_j[n] += a_ij · x_i[n − d_ij] · e^{jφ}
    ▼
channel sum + AWGN     w[n] ~ CN(0, N)   N = −174 + 10log10(BW) + NF
    ▼
demodulator            dechirp → FFT → argmax bin → symbols → bytes → CRC
    │
    ├──► reception ledger   (every receiver, including failures)
    ├──► pcapng capture     (per receiver, with pseudo-header)
    ├──► waterfall / IQ     (already computed; presentation only)
    ▼
SimRadio.recvRaw() ──► firmware (only if CRC passed)
```

The branch after the demodulator is the point of the design: **observation
happens at the RF layer**, so we record frames the firmware never sees.

## Modules

| Package | Owns | Must not |
|---|---|---|
| `internal/dsp` | Modulation, demodulation, FFT, link maths. CPU reference **and** GPU kernels. | Know about nodes, scenarios or terrain. |
| `internal/rf` | The channel: summation, delay, noise, per-receiver accumulation. | Decide whether a packet arrived. |
| `internal/terrain` | DEM tiles, profiles, diffraction, path loss. | Cache policy leaking into callers. |
| `internal/antenna` | Patterns, orientation, polarisation, feedline. | Return a scalar gain. |
| `internal/firmware` | Host builds of MeshCore, the shims, per-node runtime, build cache. | Contain propagation maths. |
| `internal/scenario` | Nodes, boards, region, seeds, persistence, providers. | Talk to the GPU. |
| `internal/capture` | pcapng, event log, reception ledger. | Alter what it records. |
| `internal/sdr` | IQ export, SigMF, streaming. | Recompute anything. |
| `internal/companion` | TCP and PTY companion transports. | Touch the RF path. |
| `internal/ui` | ImGui panels. | Contain physics. |
| `internal/mockup` + `tools/mockup` | Documentation figures only. | Be imported by anything at runtime. |

## The firmware boundary

MeshCore declares `class Radio` in `src/Dispatcher.h` with pure virtuals. We
implement it. The full boundary we must supply:

| Boundary | Our implementation | Why it matters |
|---|---|---|
| `Radio` | `SimRadio` → RF engine | The seam the whole design rests on |
| `millis()` / clock | Simulation time | Determinism, and faster-than-real-time |
| RNG | Per-node counter-based, seeded | MeshCore's CSMA is randomised; reproducibility requires owning it |
| Storage | Per-node virtual filesystem | Each node keeps its own identity and contacts |
| Serial | Virtual UART → console + control plane | Commands never go over the air (ADR-0014) |
| `logRx` / `logTx` / `logRxRaw` | Overridden in `SimDispatcher` | Upstream's own hooks — **not a fork** |

### Radio interface, verbatim from upstream

```cpp
virtual int      recvRaw(uint8_t* bytes, int sz) = 0;
virtual bool     startSendRaw(const uint8_t* bytes, int len) = 0;
virtual bool     isSendComplete() = 0;
virtual void     onSendFinished() = 0;
virtual uint32_t getEstAirtimeFor(int len_bytes) = 0;
virtual float    packetScore(float snr, int packet_len) = 0;
virtual int      getNoiseFloor() const;
virtual void     setCADEnabled(bool enable);
virtual void     resetAGC();
```

`getNoiseFloor` and `setCADEnabled` are why a real RF model is worth the cost:
MeshCore's own carrier-sense behaviour reads from them, so with a sampled
channel the firmware's CSMA becomes *observable* rather than asserted.

**`getEstAirtimeFor()` must agree with our own airtime calculation.** The
firmware's retransmit timing is built on it. If the two disagree the simulation
desynchronises from the firmware's expectations silently, and every collision
statistic is quietly wrong.

## Time

Two clocks, deliberately separate:

- **Sample time** — 1/BW, e.g. 8 µs at 125 kHz. The RF engine's unit.
- **Event time** — nanoseconds. The scheduler's unit; firmware sees this.

Energy simulation (ADR-0011) runs on a third, coarse scale (minutes over
months) and must **never** be driven by sample-level simulation. A year of
samples is not a thing anyone should compute.

Real-time coupling: attaching a companion client pins the clock to 1× (ADR-0008),
and that is surfaced in the UI, not applied silently.

## Determinism

A run that cannot be reproduced is not evidence. Rules:

1. Counter-based RNG (Philox), keyed by `(seed, node, stream, counter)`. Never a
   stateful stream shared across goroutines.
2. Fixed reduction order in the FFT and in channel accumulation.
3. Scheduler ties broken by node id, never by map iteration order.
4. **Contract:** identical results for the same seed on the same backend;
   agreement within tolerance across CPU and GPU. Bit-identity across backends
   is *not* promised — floating-point associativity differs.

## Concurrency

- One goroutine per node for firmware stepping; nodes never share mutable state.
- The RF engine is a single owner of the channel per time slice, fed by a queue.
  Fan-out to receivers is data-parallel and is the GPU's job.
- Capture and UI read snapshots, never live structures.

Firmware code will call our shims from its own execution context and expects
Arduino semantics. The shim layer must be small, explicit, and unit-tested in
isolation from the RF engine.

## Backends

| | Native | Emulated |
|---|---|---|
| Runs | MeshCore sources compiled for host | The published `.uf2` / `.bin` |
| Scale | Hundreds of nodes | A handful |
| Introspection | Layers 1 **and** 2 (see ADR-0014) | Layer 1 only |
| Use | Everything | Fidelity spot-checks, release verification |

Running one scenario on both and diffing is how we learn whether the host build
behaves like the firmware people actually flash. Divergence is a *finding*.

## Non-negotiables

- The channel decides nothing.
- Every GPU kernel has a CPU twin, and a test asserts they agree.
- Reachability is asymmetric; present both directions.
- Position uncertainty propagates; too-uncertain nodes get no verdict.
- Nothing degrades silently — say it where the affected number is shown.
