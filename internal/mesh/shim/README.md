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
| `tick.h` | `StepTick`: one firmware loop per simulated millisecond, and one loop for a tick that advances no time. Split out of the tick handler so the invariant can be tested without MeshCore's headers, by `tick_test.cpp`. |
| `probe_main.cpp` | The MSIM-1 harness: a printing `SimRadio` and a `Mesh` subclass overriding the log hooks. |
| `native_main.cpp` | The native node (MSIM-41). `BridgeRadio` puts frames on a socket to the RF engine, and the clock is driven by the simulator in lockstep. Built by `tools/native/build.sh`. |
| `repeater/` | **Paused, and does not build.** See below. |

## `repeater/` is paused work

Read this before reading anything in that directory: **nothing builds it,
nothing links it, and it would not link if something tried.** It is here for
what it records, not because it runs.

It was an attempt at a second host build, one that compiles MeshCore's own
`examples/simple_repeater` rather than a bare mesh node, so that the forwarding
policy, the CLI and the preferences a user reaches would be the firmware's own
rather than something reimplemented here. The headers are that build's platform
layer: `Arduino.h`, `Stream.h`, `HostArduino.h` (the console), `HostBoard.h`,
`HostRadio.h`, `HostSensors.h`, `RTClib.h`, `InternalFileSystem.h` with
`Adafruit_LittleFS.h` beside it, `CayenneLPP.h` for the telemetry advert, and
`target.h`, which is the same four-globals-and-two-functions contract every
MeshCore variant provides.

They compile. What is missing is the translation unit underneath them: nothing
defines the globals `target.h` and `HostRadio.h` declare (`board`,
`radio_driver`, `rtc_clock`, `sensors`, `Serial`, `InternalFS`, `g_sim_millis`,
`g_sf`, `g_bwKHz`, `g_cr`), nothing defines `radio_init()`,
`radio_new_identity()`, `HostRadio::getEstAirtimeFor()` or
`HostRadio::packetScore()`, and there is no `main()` and no build script.

It stayed unfinished because the need went away rather than because the work was
hard: host builds of `simple_repeater`, `companion_radio` and
`simple_room_server` are produced by `MeshBench/meshcore-native` and downloaded
at runtime, which is also what keeps MeshCore out of this binary's link line.
Reviving this would mean writing that missing translation unit, a build script
beside `tools/native/build.sh`, and a reason to prefer a local build over the
published one.

Three defects that were in it have been fixed rather than left for whoever picks
it up, because a reader cannot tell paused code from wrong code: `HostSerial`
answered `available()` from the consuming input callback, so the ordinary
`if (Serial.available()) c = Serial.read();` dropped every other byte;
`Adafruit_LittleFS::format()` returned success without erasing anything; and
`CayenneLPP` owned a raw `new[]` buffer with no copy or move operators, so
copying one double-freed it. None was reachable, all three are the kind that
would be blamed on something else when it was.

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
