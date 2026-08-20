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
- **The optional CoreScope, Beacon and MQTT feeds**, which you configure and
  which are off until you do.

MeshBench listens on a **unix domain socket** for the control interface
(`$XDG_RUNTIME_DIR/meshcoresim.sock`), which is per-user and does not survive a
reboot. It opens no TCP port unless you ask it to serve a companion transport,
which binds where you tell it to.

## Reporting a vulnerability

Report privately, not as a public issue: email **alex@hectospark.co.uk** with
`meshbench security` in the subject.

GitHub's private vulnerability reporting is not enabled on this repository yet —
it is still private, and that channel will be turned on when it is published.
Until then, email is the route, and it is read.

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
