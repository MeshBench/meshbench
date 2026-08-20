# Development machines

Where this project is actually worked on, and what each machine can and cannot
do. **This file is expected to go stale**, which is why it is separate from
`CLAUDE.md`: the rules there apply to anyone on any machine, and none of this
does.

## elite

`alex@10.100.72.98`, at `~/Documents/projects/meshcoresim`. Twelve cores, a
real GPU, and the Renode and QEMU toolchains for the firmware work.

- The full suite with `-race` runs in about three minutes.
- `golangci-lint` is pinned at `~/go/bin` to the version CI uses, **v2.1.6**.
  Matching it matters: v2.1.6 is built with go1.24 and refuses a config
  targeting a higher Go version, so a newer local binary disagrees with CI
  about whether the tree is clean.
- Clone with `gh repo clone MeshBench/meshbench`.

**One emulated board at a time.** The board check across several emulated
boards at once will take this machine down — an emulated node runs at roughly
1× real time and each carries a whole emulator process.

## VM 114

**MeshBench will not run here.** Virtual VGA, no display: it needs a GPU and a
display to open its window. Develop here if you like; run it on elite or a Mac.

The CPU path is what CI exercises, so a machine without a usable GPU loses
time, not features — but it still needs somewhere to draw.

## The lab runners

Three Proxmox LXC containers serve as self-hosted GitHub Actions runners,
labelled `lab-linux` and `lab-2204`. CI's Linux jobs land on them for pushes
and for pull requests from this repository; **a fork's pull request falls back
to `ubuntu-latest`**, deliberately, because the lab runners sit on a home LAN
with a NAS mounted and a stranger's code must never land on one.

`package.yml` also uses a self-hosted `macos-m4` runner for the macOS bundle.

## What runs where

| | elite | VM 114 | lab runners | hosted |
|---|:-:|:-:|:-:|:-:|
| `go test ./...` | yes | yes, slower | yes | fork PRs only |
| `-race` | ~3 min | several times that | on request | — |
| GPU kernels | yes | no | no | Windows check only |
| Emulated boards | yes, one at a time | no toolchain | no | no |
| Opening the window | yes | **no display** | no | no |
