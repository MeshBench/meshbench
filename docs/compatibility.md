# Versioning and compatibility

What a MeshBench version number promises, what it does not, and how you find
out when something has moved under you.

MeshBench is **0.x, deliberately**. There is no 1.0 scheduled, and there will
not be one until the list at the end of this page is worked through. The
decision is the owner's and it is the honest one: 1.0 is a claim about
stability, and this project is still learning what it is.

## What 0.x means here

The usual reading of a leading zero is that anything may break at any time. That
is a licence, and taken literally it would make this page unnecessary. It also
would not be true: three client libraries are published to two package indexes,
a control socket is documented verb by verb, and files people save are opened
again months later. Somebody is depending on all of it, and saying "0.x, so
caveat emptor" while quietly expecting them not to is dishonest in the other
direction.

So the promise is narrower than 1.0 and wider than nothing:

**Three things are checked at runtime, and a mismatch is refused rather than
guessed at.** The control protocol number, the client and workbench pairing, and
the fixture format. Each is described below. What they have in common is that
the failure they exist to prevent is not a crash: it is a run that completes and
answers a question about something other than what you asked. This simulator's
worst failure mode is a plausible wrong number, and every one of these three
rules is bought to avoid one.

**Everything else may change between any two releases**, including the verb set,
the shape of a result, the command-line flags, the panels, the file layout of an
installation, and the numbers a study produces when the model improves. The
`CHANGELOG.md` says what changed. It is the only notice you get, and while the
major version is 0 there is no deprecation period behind it.

**A release is a plain `X.Y.Z`, spelled that way everywhere.** The git tag
carries a leading `v`, the linker stamp carries the tag, and PyPI and npm carry
the number without it. Nothing else counts as a release: a build from a working
copy stamps nothing at all, on purpose, because "not a release" is a thing the
pairing rule below needs to be able to tell.

## The control protocol number

`control.Protocol` is the wire version, and it is about **frames**, not
features. It moves when a client written against the previous number would
misread this one: a verb changing what it means, a field changing type, the
framing changing. It does not move when a verb is added, or when a result grows
a field, because a client reads the fields it knows and ignores the rest.

It has been `1` since the socket existed, and has never moved.

A client declares the number it speaks on the frame it was already sending: the
token line on loopback TCP, the first request on a unix socket. There is no
handshake of its own, because adding one would have broken every script written
against this socket before it existed, which is the wire the check exists to
protect.

**A number this build does not speak is refused before any verb runs**, with an
error carrying both numbers and which end to change. The rule is an exact match
in both directions, not "a newer workbench serves an older client": the number
moves only when something an older client relied on has changed, so any
difference at all is that break by construction.

**A client that declares nothing is served.** Every script written against this
socket before the field existed is one of those, and it finds out from
`session.hello`, which is what the shipped clients have always read.

The refusal carries the code `protocol_mismatch`. `docs/scripting-api.md` has
the wire detail and what each client raises.

## A client and the workbench must be the same release

The protocol number answers whether two ends can understand each other's frames.
It cannot answer the question a script actually has, which is whether the client
in the virtualenv is the one that came with the workbench on the PATH. Two
releases apart with no protocol bump between them will connect happily and then
disagree about a verb's parameters, and that failure surfaces forty calls later
looking like the simulation misbehaving.

So the release travels on the wire beside the protocol number, and **a released
client driving a workbench from a different release is refused at connect**,
with both release numbers and the instruction to make them the same one. The
code is `version_mismatch`.

Refused in the workbench rather than in the clients, because a third-party
script speaking the raw socket is entitled to the same answer as one using a
client we ship.

**A pair where either end is not a release is allowed through**, and this is
what keeps the tree usable by the people working on it. A build from a working
copy has no release stamped in it, and neither has a client run out of the same
checkout, so a rule of "they must be equal" would refuse every pair a developer
has, every run, for a disagreement that does not exist. Where one end is
unstamped there is no second version to disagree with. The skipped check is
logged rather than silent: `MESHBENCH_LOG=control` says which end was not a
release.

The practical consequence for a script: **pin the client to the workbench.**
`pip install meshbench==0.1.0` beside MeshBench 0.1.0, and the same for
`@meshbench/client`. Upgrading one without the other is caught at connect rather
than in the middle of a run, which is the whole point.

## The fixture format

A fixture is the input to a simulation: the nodes, the boundary, the traffic and
the assertions. It carries a `format` number, and this build reads up to
`fixture.Format`.

**Older is read. Newer is refused.** A file written by a later MeshBench is
refused by name, with both format numbers and the instruction to install the
release that wrote it. Nothing migrates it and nothing reads the parts it
recognises.

Refusing is the right answer while the format is still moving. Reading three
quarters of a file does not fail: it produces a complete, plausible result about
a network nobody described, and there is no point in the run at which anybody
would be told. Refusing costs one upgrade. Migration machinery would be the
other answer, and it is not built, because building it now would be guessing at
migrations for changes nobody has designed.

A file written before the number existed reads as format 0 and is still read.
The shape did not change on the day it started being declared, and backward
compatibility is the half of this rule that has to keep working: the fixtures
this release ships are the example everybody opens first.

`format` moves on the same rule the protocol number does: when a file this build
writes would be *misread* by the build before it. A new field that an older
build ignores costs it nothing and does not move the number.

## Version stamping, and what each build path produces

The version a build reports is a linker stamp, and the linker stamp only ever
reaches a binary the release pipeline builds. Three platforms are built by three
different jobs, so the only thing keeping the three in agreement is that
somebody remembered, and once they disagree the difference is invisible until a
release is already out: each build works perfectly and says a different thing
about itself. That has happened once, and the two platforms that disagreed with
the tag were the two that moved.

All three now stamp `internal/app/version.Version=v$VERSION`, where `$VERSION`
is the tag without its `v`, or the dispatched version, or
`0.0.0-dev-<short sha>` for a build from something that is not a tag. So a
release binary reports its tag, `v0.1.0`, and a build from anything else reports
a version that is visibly not a release and is treated as one by the pairing
rule above.

`internal/app/version`'s own test reads the three build commands out of
`.github/workflows/package.yml` and `packaging/macos-app.sh` and fails if any of
them builds `cmd/meshbench` without that stamp, or drops the `v`, or if a fourth
build path appears without one. All three also refuse a version that is not a
plain `X.Y.Z`, because a workbench stamped `vv0.1.0` is not a release as far as
the pairing rule can tell, and would pair silently with every client of every
version.

## Before 1.0 is worth cutting

Not aspirations. Each of these is something a person could check off, and each
is a reason a 1.0 today would be a claim the project cannot support.

1. **A predicted margin has been compared with a real reception.** The
   validation harness exists, counts its exclusions and refuses to treat a
   silent receiver as a negative observation. What it has never had is data. Run
   it against a real CoreScope or Beacon export and publish the bias and the
   spread. Until then every propagation number is correct according to the
   textbook, which is not the same as true on the hill.
2. **The board in the reader's hand works.** Ten boards are described and, as
   last measured, one passes every capability the board check asks of it. Three
   nRF52 boards advert once and go quiet, two ESP32 boards assert inside
   ESP-IDF startup, and two have never been attempted. A 1.0 whose odds against
   an arbitrary board are seven in ten is a version number doing marketing.
3. **A downloaded build runs without the operating system objecting.** macOS is
   ad-hoc signed and not notarised; Windows is unsigned and SmartScreen warns.
   Both need a signing identity, which needs an account, which is the actual
   task.
4. **Every platform can fetch what it needs.** Windows still cannot fetch an
   emulator, because the resource downloader reads ELF and Mach-O headers and
   not PE.
5. **A release has been rebuilt from its own source archive**, by somebody who
   did not build it, on a machine that has never seen this repository. The
   archive is published with every release and the obligation is met on paper;
   nobody has demonstrated it.
6. **The verb surface has gone two consecutive releases without a breaking
   change.** `docs/verb-reference.md` is generated, so the diff between two
   releases is the evidence: additions are fine, and a verb changing what it
   means or what it takes is not. A wire that has never been stable for two
   releases cannot be promised to be stable for a major version.
7. **The fixture format has a way forward, or a stated reason it will not need
   one.** Refusing a later file is the right answer while the format is moving.
   At 1.0 it stops being an answer, because the whole promise of a major version
   is that the file you saved keeps opening.

When those are done, 1.0 means: the control protocol and the fixture format do
not break within the major version, the clients and the workbench keep their
pairing rule, and anything that would break either gets a 2.0. Not before.

## The source archive, and the forks

MeshBench is GPL-3.0-or-later and the repository is still private, so every
release carries a `meshbench-source.tar.gz` beside the binaries. That
is section 6's obligation met directly rather than by an offer.

An archive is only worth what a recipient can build from it, and this tree
carries a `replace` directive pointing at **`MeshBench/gio`**, our Gio fork with
Wayland layer-shell windows. **That fork is public.** A rebuilder can fetch it
with no credentials, no `GOPRIVATE` and no `insteadOf`, which is what makes the
archive a real remedy rather than a gesture. Every other fork this project
carries is public too, and `docs/repositories.md` states each one's status in a
table for exactly this reason.

MeshCore itself is not linked into the binary. It is built separately in
`MeshBench/meshcore-native` and downloaded at runtime, which is what freed the
licence choice; do not reintroduce a direct dependency on it without an ADR.

The Nordic SoftDevice is not ours and cannot be redistributed. Anyone running
published nRF52 firmware supplies their own copy.
