# Provenance

Binaries released from this repository are built from
[meshcore-dev/MeshCore](https://github.com/meshcore-dev/MeshCore), which is MIT
licensed, copyright Scott Powell / rippleradios.com. Their applications are
compiled **unmodified**; everything this repository adds is the host variant in
`variants/host/` and the bridge in `bridge/`, also MIT.

The commit each release was built from is recorded in that release's body.

Third-party code compiled in:

| what | source | licence |
|---|---|---|
| MeshCore | meshcore-dev/MeshCore | MIT |
| Crypto | rweather/arduinolibs | MIT |
| ed25519 | vendored in MeshCore under `lib/ed25519` | as MeshCore ships it |

The Cayenne LPP constants in `variants/host/CayenneLPP.h` and the base64
implementation in `variants/host/base64.hpp` are ours, written against published
formats rather than copied, so that a build does not need the network to resolve
values that have not changed in a decade.

## Why this is a separate repository

The simulator these builds run under has not chosen a licence. Linking MeshCore
constrains that choice, so the linking happens here, under MeshCore's own terms,
and the simulator downloads the result.
