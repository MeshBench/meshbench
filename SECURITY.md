# Security

## What MeshBench is, in security terms

A **desktop application**. One binary on your own machine, with no service to
deploy, no remote worker and no compute backend. Nothing in the simulation
depends on anything we run.

The only things that cross the network are *data*:

- **Terrain tiles and basemap imagery**, fetched from public tile servers and
  cached locally. There is an offline mode that fails loudly rather than
  silently degrading.
- **Firmware images and the Nordic SoftDevice**, downloaded from their
  publishers on request, checksummed against digests pinned at build time.
- **The optional CoreScope and Beacon feeds**, which you configure and
  which are off until you do.

The control interface listens locally, and how depends on the operating
system:

- **Linux, macOS and the BSDs**: a **unix domain socket**, per user, mode
  `0600`, gone at reboot. On Linux it is `$XDG_RUNTIME_DIR/meshbench.sock`;
  elsewhere it is under the per-user cache directory. The kernel enforces who
  may connect.
- **Windows**: a **loopback TCP listener** on an ephemeral port, because
  Windows has no unix socket a Python client can reach. It is bound to
  `127.0.0.1` and never to an outward-facing address, and a connection must
  present a 128-bit token before it is served. The port and token are written
  to a `0600` file under the per-user cache directory.

The Windows arrangement is deliberately weaker than the unix one, and worth
saying plainly: **any local process can open a loopback port**, so the token in
that file is what stands between another program on the same machine and your
running session, where on Linux the kernel does it. If that matters to you,
pass `-control-socket` a unix socket path — Windows 10 and later do have
AF_UNIX for Go clients, it is only the Python one that cannot use it.

Either transport can be pointed somewhere else with `-control-socket` or
`MESHBENCH_CONTROL_SOCKET`. An address that is not loopback is refused rather
than bound.

No other port is opened unless you ask it to serve a companion transport or an
SDR source, which bind where you tell them to.

## Reporting a vulnerability

Report privately, not as a public issue. Either route works:

- **GitHub's private vulnerability reporting**, which is enabled on this
  repository: open the Security tab and choose Report a vulnerability. This is
  the preferred route, because the report, the discussion and the eventual
  advisory stay in one place.
- **Email** alex@hectospark.co.uk with `meshbench security` in the subject.

Please include what you did, what happened, and what you expected. A proof of
concept helps; a crash report with the input that caused it is already useful.

You will get an acknowledgement within a week. This is a small project with one
maintainer, so please do not expect a same-day response — and please do not
disclose publicly before we have had a chance to look.

## What we consider a vulnerability

- Anything that lets a **downloaded artefact** — a firmware image, a terrain
  tile, a fixture, a feed message — execute code, escape its cache directory,
  or overwrite a file outside it.
- Anything that lets a **process on the same machine** drive the control socket
  in a way its own user could not.
- Anything that causes MeshBench to send data somewhere the user did not
  configure.

## What we do not

- **A simulation result being wrong** is a correctness bug, not a security one.
  Open a normal issue. [`docs/shortcomings.md`](docs/shortcomings.md) records
  what the model does not do and in which direction it errs.
- **The emulators running unsanitised firmware.** Running a firmware image
  under QEMU or Renode is the entire point, and an image you chose to load is
  code you chose to run. Load images you trust.
- **Anything requiring an attacker who already has your user account.**
