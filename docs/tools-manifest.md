# The tool manifest

What MeshBench needs beside itself, which version, and where it looks. Wave 1B
of `docs/plan-2026-08.md`, and the contract that packaging and both test
harnesses build against.

## Where things are looked for

Already implemented, in this order, and the order is the point:

1. the environment variable, if set
2. **beside the MeshBench binary**
3. `~/.cache/meshcoresim/tools/`
4. `PATH`

`PATH` is last and nearly useless on a desktop. **A desktop application is not
launched from a shell**, so it inherits neither a useful `PATH` nor any
environment variables. That was a real fault, not a hypothetical: emulation
worked from a terminal and failed from the desktop, reporting what read as a
missing package.

Beside-the-binary is what an installed bundle should use. The cache directory is
what a developer should use, and is where the release installer puts things it
downloads after the fact.

## The tools

| tool | env | needed for | source |
|---|---|---|---|
| `qemu-system-xtensa` | `MESHCORESIM_QEMU` | emulated ESP32 nodes | `MeshBench/qemu`, branch `meshbench-sx1262` |
| `renode` | `MESHCORESIM_RENODE` | emulated nRF52 nodes | `MeshBench/renode`, branch `meshbench` |
| `radioserver` | `MESHCORESIM_RADIO_SERVER` | both, and nothing else | `MeshBench/meshcore-native`, `bridge/` |
| native firmware | `MESHCORESIM_NATIVE` | every native node | `MeshBench/meshcore-native` releases |

**`MESHCORESIM_NATIVE` may name a directory** holding one build per role, which
is what a scenario mixing roles needs. Naming a single binary overrides every
node regardless of role, so a mesh of repeaters and room servers quietly becomes
a mesh of one application.

## Versions, and why they are pinned rather than "latest"

**QEMU** must carry our SX1262 device and a working GPIO implementation.
Upstream's GPIO write handler is empty, and RadioLib drives chip select as an
ordinary GPIO rather than through the SPI controller, so a distribution build
produces a driver that reports no chip present. It must be configured with
`--enable-gcrypt` or the `esp32` machine will not instantiate at all.

**Renode** must carry the SEVONPEND fix. Stock Renode asks whether a pending
interrupt could be *taken*, which a disabled one never can, so published nRF52
firmware sleeps for ever with its wake condition already true. A stock Renode
will start, load, run, and hang in exactly the place the fix was written to
cure, which is why the fork's CI asserts both halves of the fix are present in
the tree it built.

**Native firmware versions are per-role release tags** and not bare versions:
`repeater-v1.17.0`, `companion-v1.17.0`, `room-server-v1.17.0`. Asking for
`v1.17.0` resolves nothing and reports "no native builds published", which points
at the release rather than at the string.

**Board images are not in this manifest.** They are fetched from MeshCore's own
releases at run time and cached, because they change every release and bundling
them dates the installer.

## What a released bundle ships

All three platforms, as of the packaging work: a Linux tarball, an arm64 dmg
and a Windows zip.

    meshbench                     the application
    qemu-system-xtensa            beside it, or symlinked
    renode                        beside it, or symlinked
    radioserver                   beside it

A **symlink** is correct for QEMU and wrong for a copy: it resolves its own path
to find its data files, so a bare copy of the binary will not run. The Windows
zip cannot carry a symlink at all, so nothing is linked there and `lookupTool`
searches the emulators' own unpacked layouts — `qemu/bin/` and the versioned
`renode_*-portable/` — instead.

**radioserver is the one every emulated node needs**, ESP32 or nRF52, and it is
looked up before either emulator. Nothing built it until `build.sh` grew a
`radioserver` target, which is how 0.0.1 through 0.0.3 shipped both emulators
and no radio.

The AppImage and the `.deb` carry the application and radioserver but not the
emulators: those are 110 MB against a 26 MB AppImage, and the tarball is the
batteries-included download for people who want them.

## What is checked, and when

A tool that is missing must say so before a run rather than during one. The
error names all four search locations, because "qemu-system-xtensa not found"
sends people to their package manager for a build that will not do.

The firmware library only offers boards with verified emulation wiring, so a
board that appears in the picker is one whose image has been watched booting.
That list is `scenario.EmulationVerified`, and it is deliberately short.
