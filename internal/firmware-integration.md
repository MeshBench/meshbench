# Firmware integration

How real MeshCore code runs inside MeshBench. This is the highest-risk part of
the project — MSIM-1 exists to prove it before anything else is built on it.

## What we know, verified against upstream

Checked at `meshcore-dev/MeshCore` HEAD, not assumed:

1. **`platformio.ini` has `[env:native]` and `[env:native_kiss_modem]`**
   (`platform = native`, C++17, googletest) and a `test/mocks/` directory
   shimming `Arduino.h`, `Stream.h`, `AES.h`, `SHA256.h`, `Identity.h`,
   `Mesh.h`.
   **Be precise:** that env compiles only `Utils.cpp`, `Packet.cpp` and
   `ConfigSerializer.cpp`. It is a unit-test harness, *not* a firmware build.
   What it proves is that the code is not hardware-entangled at the language
   level and that host mocking is an established upstream pattern.
2. **`src/Dispatcher.h:22` declares an abstract `class Radio`** with pure
   virtuals — the seam the whole design rests on.
3. **`src/Dispatcher.h:159–162` declares `logRxRaw`, `logRx`, `logTx`** as empty
   virtuals commented "hooks for custom logging". Overriding them is *not* a
   fork.
4. **`MESH_DEBUG`** gates `MESH_DEBUG_PRINT` to serial — the instrumented build.
5. **Licence: MIT.** Attribution only; no constraint on our own choice.
6. **Prebuilt releases exist per board**, in three families — Companion,
   Repeater, Room Server — at 158 companion targets over 87 board variants
   (36 nRF52, 43 ESP32).

## Verified by MSIM-1 (2026-08-09)

**The mesh stack builds and runs on the host, unmodified.** `Utils`, `Packet`,
`Identity`, `Mesh` and `Dispatcher` compile clean; a `Mesh` subclass ran 250
loop iterations and transmitted through `SimRadio`:

```
identity generated, pub_key[0..3]: 33 0a c2 1f
[SimRadio] startSendRaw len=10 bytes: 11 00 a0 a1 a2 a3 a4 a5
after sendFlood -> txCount=1 logTx=1
```

Every gap was in *our* shims, not in MeshCore: `Stream` needed `println`,
`readBytes` and buffered `write`, and rweather's Ed25519 needed a global RNG.
See `internal/firmware/shim/`.

## Previously unknown, now answered

**Whether the mesh stack itself builds off-target.** `Dispatcher`, `Mesh` and
`MyMesh` have never been compiled for the host. MSIM-1 is timeboxed at 8 h to
find out; if it turns out to be hardware-entangled in a way the `Radio`
abstraction hides, ADR-0002 needs revisiting rather than patching.

## The shim contract

```
┌─────────────────────────── one simulated node ───────────────────────────┐
│  MeshCore C++ (unmodified)                                              │
│    MyMesh / Mesh / Dispatcher                                           │
│      │            │              │            │                        │
│      ▼            ▼              ▼            ▼                        │
│  SimRadio     SimClock      SimStorage    SimSerial                     │
│      │            │              │            │                        │
└──────┼────────────┼──────────────┼────────────┼────────────────────────┘
       │            │              │            │
   RF engine   scheduler      virtual FS   console + control plane
```

### SimRadio

| Method | Implementation notes |
|---|---|
| `startSendRaw` | Enqueue frame to the RF engine with node id and sim time. Return false if already transmitting. |
| `isSendComplete` | True once sim time ≥ start + airtime. |
| `recvRaw` | Pop from this node's delivered queue. **Only CRC-passing frames reach here** — everything else is recorded but withheld, exactly like real hardware. |
| `getEstAirtimeFor` | Must equal our own airtime formula. Test this first. |
| `getNoiseFloor` | Real value from the channel, including external emitters. |
| `setCADEnabled` / CAD result | Answer from actual channel energy, not a guess. This is where firmware CSMA becomes observable. |
| `packetScore` | Defer to the firmware's own formula; we supply true SNR. |
| `resetAGC` | Model or no-op; record that it happened. |
| `isInRecvMode` | **Seventh pure virtual, found in MSIM-1.** True while listening; in the real engine this comes from channel state, and it is part of what makes CAD truthful. |

### SimClock

Simulation time, not wall time. Firmware calls `millis()` constantly. Faster
than real time by default; pinned to 1× when a companion client attaches.

### SimStorage

Per-node virtual filesystem so each node keeps its own identity, contacts and
channels. Firmware swap offers **wipe or keep** explicitly — "did it keep its
identity across the upgrade" is itself worth testing.

### SimSerial

Carries MeshCore's own CLI (`advert`, `set radio`, `advert.interval`, `cad`,
`bridge.*`). Two consumers: the console UI, and the control plane.

**No management traffic is ever transmitted over the simulated air.** Commands
over RF would add airtime, trigger backoffs and change collision statistics —
you would be measuring a network that only exists because you were measuring it.

## Observation layers

| Layer | Source | Adds | Modification |
|---|---|---|---|
| 1 | RF engine | Every frame offered, incl. sub-threshold; true RSSI/SNR; demod and CRC outcome | none — both backends |
| 2 | `SimDispatcher` overrides | Dedup decisions, scores, relay choices | none — upstream's own hooks |
| 3 | `MESH_DEBUG=1` build | Routing state, why-not-relayed | build flag; **alters timing** |

Layer 1 answers "what did this node receive" in a stronger sense than the
firmware could — it includes frames the firmware never saw.

Layer 3 is the observer effect at firmware level: serial printing costs time
inside the loops driving CSMA. Nodes running one are marked in the UI, and
MSIM-26 requires *measuring* the distortion rather than assuming it is small.

## Build management

A firmware build is `(git ref, variant, board, flags)`, compiled and cached by
that tuple, recorded with its commit hash. Different nodes may run different
builds in one scenario — that is the feature, not a side effect.

First run must handle a missing C++ toolchain with a clear message, not a
compiler error dump.

## Emulated backend

Renode, running the published binary with a modelled SX1262 on SPI. Sized by
MSIM-17 before commitment. Its value is not fidelity for its own sake — it is
the **cross-check** that tells us whether the native build behaves like what
people flash. A clear "too expensive" is a good outcome; the native backend
stands on its own.
