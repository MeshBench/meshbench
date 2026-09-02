# Development machines

Where this project is actually worked on, and what each machine can and cannot
do. **This file is expected to go stale**, which is why it is separate from
`CLAUDE.md`: the rules there apply to anyone on any machine, and none of this
does.

## elite

`alex@10.100.72.98`, at `~/Documents/projects/meshbench`. Twelve cores, a
real GPU, and the Renode and QEMU toolchains for the firmware work.

- The full suite with `-race` runs in about three minutes.
- `golangci-lint` must match **the version CI runs, v2.12.2** — the pin is in
  `ci.yml`, which is the authority. Matching it is not pedantry: v2.1.6 and
  v2.12.2 disagree about this tree by 29 findings (34 `gosec` against 17, 12
  `noctx` against 1), so a local run on the wrong version says the tree is
  clean when CI will not. `tools/lint-ratchet.sh` takes `GOLANGCI_LINT` if the
  right binary is not first on your path.
- Clone with `gh repo clone MeshBench/meshbench`.

**One emulated board at a time.** The board check across several emulated
boards at once will take this machine down — an emulated node runs at roughly
1× real time and each carries a whole emulator process.

### SonarQube

A Community Build container runs on elite at **http://localhost:9000**, project
`meshbench`. It is not part of CI and deliberately so: a second gate to satisfy
before a merge is a second thing to work around. This one is for reading.

```bash
# the server, once
docker run -d --name meshbench-sonar -p 9000:9000 \
  -v meshbench-sonar-data:/opt/sonarqube/data \
  -v meshbench-sonar-ext:/opt/sonarqube/extensions \
  --restart unless-stopped sonarqube:community

# a scan, whenever
go test ./... -coverprofile=coverage.out -covermode=atomic
docker run --rm --network host -v "$PWD:/usr/src" \
  -e SONAR_TOKEN=... sonarsource/sonar-scanner-cli
```

The scanner does not run the suite — a scanner that does is a scanner nobody
waits for — so `coverage.out` has to exist first. `sonar-project.properties` in
the repo root carries the rest.

**What it is for, given golangci-lint already runs 20 linters.** The things that
are not a single-file rule: cognitive complexity per function, duplication
measured across the whole tree, and coverage readable per package rather than as
one number. It found a duplicated `switch` case in `tools/licgen` that no Go
linter had.

Stop it with `docker stop meshbench-sonar`; the volumes keep the history.

## VM 114

**MeshBench will not run here.** Virtual VGA, no display: it needs a GPU and a
display to open its window. Develop here if you like; run it on elite or a Mac.

The CPU path is what CI exercises, so a machine without a usable GPU loses
time, not features — but it still needs somewhere to draw.

## The lab runners

Three Proxmox LXC containers serve as self-hosted GitHub Actions runners,
labelled `lab-linux`, `lab-2204` and `lab-2404`. All three answer to
`lab-linux`. The other two say which glibc a job gets, and both are load
bearing in opposite directions: `lab-2204` is gha-lab-3 alone, and
`package.yml` asks for it to keep the release binary's floor at 2.35;
`lab-2404` is gha-lab-1 and gha-lab-2, and `firmware-live.yml` asks for it
because MeshCore's published native builds need 2.38, which gha-lab-3 cannot
provide. A job that needs either floor must name it: `lab-linux` alone is a
lottery between them.

CI's Linux jobs land on them for pushes
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
