# Third-party notices

MeshBench is **GPL-3.0-or-later** — see [`LICENSE`](LICENSE), and
[`docs/licence.md`](docs/licence.md) for why that licence and what it obliges.

This file is a pointer, not the inventory. **The inventory is generated**, so
that it cannot fall behind the build:

- `go run ./tools/licgen` walks the build graph of `./cmd/meshcoresim`, reads
  every linked module's licence out of the module cache, classifies it, and
  **fails** on a module whose licence it cannot name. A dependency arriving
  without a licence breaks the build, not somebody's review.
- The curated half — the forks, the bundled third parties, what is downloaded
  at runtime and the data attributions — lives in
  [`docs/licences.json`](docs/licences.json) with its texts checked in under
  `docs/licences/`, so generation needs no network.
- The result is written to `internal/ui/workbench/licences/licences.json`,
  embedded in the application as its **Licences** window, and unpacked into a
  `LICENCES` directory inside every release bundle.
- CI regenerates it on every pull request and fails if the checked-in copy has
  drifted. The release pipeline additionally runs
  `licgen -require-project-licence`, which refuses to publish a build of a
  project that has no licence of its own.

## What is in it

Six sections, 26 entries besides MeshBench itself, as of this writing:

| Section | Count | What it covers |
|---|--:|---|
| Modified forks | 5 | `MeshBench/qemu` (GPL-2.0), `MeshBench/tlib` (LGPL), `MeshBench/renode` and `renode-infrastructure` (MIT, MIT + LGPL), `MeshBench/meshcore-native` (MIT) |
| Bundled third parties | 4 | .NET runtime (MIT), wgpu-native (MIT or Apache-2.0), Noto Color Emoji (OFL-1.1), the Wireshark dissector (GPL-2.0-only) |
| Go libraries | 10 | Discovered from the build graph — Gio and its shader package, cogentcore/webgpu, go-text/typesetting, zenity, and five `golang.org/x` modules |
| Downloaded at runtime | 3 | MeshCore (MIT), the published board firmware images, the Nordic SoftDevice under Nordic's own agreement |
| Map and terrain data | 4 | OpenStreetMap (ODbL 1.0), CARTO basemaps, Esri World Imagery, Terrain Tiles on AWS |

Counts change with the build graph; the generated inventory is always the
authority, and this table is a description of it rather than a second copy.

## Two obligations worth naming here

**Nothing of MeshCore is linked into this binary.** MeshCore is built in
[`MeshBench/meshcore-native`](https://github.com/MeshBench/meshcore-native)
under its own MIT terms and downloaded at runtime, and the emulator forks are
separate processes spoken to over sockets. That is what left MeshBench free to
choose GPL-3.0 — an aggregation beside the binary, not a combined work with it.

**Map attribution must appear wherever the map does.** Each basemap layer
carries its own attribution string and the application draws it on the map.
That is an ODbL and CARTO requirement rather than a courtesy, which is why the
field is not optional on a layer. Any map you publish from a MeshBench
simulation carries the same attribution.

## Reading the terms without building anything

Open the application and choose **Help → Licences**, or unpack any release
bundle and read the `LICENCES` directory. Both are the same generated set.
