# Host shims for MeshCore

Proof for MSIM-1, kept as the starting point for `internal/firmware`.

MeshCore's mesh stack compiles for the host **unmodified**. Everything needed
lives here; nothing upstream was patched.

## What upstream gives us

- `src/Dispatcher.h` declares abstract `Radio`, `MillisecondClock`,
  `PacketManager`; `src/MeshCore.h` declares `RTCClock`; `src/Utils.h`
  declares `RNG`; `src/Mesh.h` declares `MeshTables`.
- `Dispatcher.h:159-162` declares `logRxRaw` / `logRx` / `logTx` as empty
  virtuals — observation layer 2 needs no fork.

## What we supply

| Shim | Why |
|---|---|
| `Stream.h` | Upstream's `test/mocks/Stream.h` lacks `println()`, `readBytes()` and buffered `write()`, all of which `Identity.cpp` uses. Also carries `captured()` / `feed()` so the console and control plane can attach. |
| `HostRNG.cpp` | rweather's `Ed25519`/`Curve25519` reference a global `RNG`; theirs needs `micros()`. Ours is seeded, so key generation is reproducible. |
| `SimNode.h` | The shims themselves — `SimClock`, `SimRTC`, `SimRNG`, `SimTables`, `SimPacketMgr`. Shared by both builds below, because a proof that exercises a different stack from the one that ships proves nothing. `Radio` is deliberately not here: it is the one seam that differs. |
| `probe_main.cpp` | The MSIM-1 harness: a printing `SimRadio` and a `Mesh` subclass overriding the log hooks. |
| `native_main.cpp` | The native node (MSIM-41). `BridgeRadio` puts frames on a socket to the RF engine, and the clock is driven by the simulator in lockstep. Built by `tools/native/build.sh`. |

## Building

```
MC=path/to/MeshCore
CRY=path/to/arduinolibs/libraries/Crypto
INC="-I $MC/src -I shim -I $CRY -I $MC/lib/ed25519"
g++ -std=c++17 $INC -c $MC/src/{Utils,Packet,Identity,Mesh,Dispatcher}.cpp
g++ -std=c++17 $INC -c $CRY/{SHA256,AES128,AESCommon,BlockCipher,Crypto,Ed25519,BigNumberUtil,Curve25519,SHA512,Hash}.cpp
gcc -std=c11 -I $MC/lib/ed25519 -c $MC/lib/ed25519/*.c
```

Deps: MeshCore (MIT), rweather/arduinolibs Crypto (MIT), MeshCore's vendored
`lib/ed25519`. All permissive; attribution belongs in `THIRD_PARTY_NOTICES.md`
whenever we first distribute.

## Correction to docs/firmware-integration.md

`Radio` has **seven** pure virtuals, not six. The missed one is
`isInRecvMode() const` — and it matters: in the real engine it comes from
channel state, and it is part of what makes CAD truthful.
