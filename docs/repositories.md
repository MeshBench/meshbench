# Repositories

Everything MeshBench depends on that we control, why each exists, and what we
changed. Written for the move into its own organisation: the awkward part of
that move is not the repositories, it is the references *between* them, and
those are listed here.

## Where things live

Everything except MeshBench itself is in the **MeshBench** organisation.

Whether each is public matters, and is stated here because it has to be
checked rather than assumed: a release publishes binaries built from these, and
GPL-3.0 section 6 means whoever receives one can have the source that made it.
A fork nobody outside the organisation can fetch cannot satisfy that.

| repository | what it is | licence | public |
|---|---|---|---|
| `MeshBench/meshbench` | MeshBench itself | GPL-3.0-or-later, `docs/licence.md` | yes, since 1 September 2026 |
| `MeshBench/meshcore-native` | host builds of MeshCore, the bridge and `radioserver` | see its NOTICE | yes |
| `MeshBench/virtual-sx1262` | the virtual SX1262 itself, with a C ABI, vendored by everything that needs a chip | MIT, and it has to stay permissive | yes |
| `MeshBench/meshbench-reports` | the published reports site | — | yes |
| `MeshBench/docs` | the documentation site, six of whose pages are generated from here | not stated | yes |
| `MeshBench/qemu` | QEMU with our SX1262 | GPLv2, upstream's | yes |
| `MeshBench/tlib` | the CPU library, with the SEVONPEND fix | upstream's | yes |
| `MeshBench/renode-infrastructure` | the C# half of that fix | upstream's | yes |
| `MeshBench/renode` | ties them together and builds the package | upstream's | yes |
| `MeshBench/gio` | Gio with Wayland layer-shell windows | upstream's | yes |

`virtual-sx1262`'s licence is the one in that table chosen by arithmetic rather
than by preference. Four things link that model: MeshCore built for this host,
our QEMU fork, Renode's C# peripheral, and eventually MeshBench itself. QEMU is
GPLv2 and MeshBench is GPL-3.0-or-later, and those two copylefts are not
compatible with each other, so no single copyleft licence lets both link one
library. Permissive is what is left. It is also why that repository forbids
dependencies beyond the C++ standard library: a GPL library pulled in there
would put the licence back where it cannot be.

One more in the organisation is **private**, and nothing a release ships is
built from it, which is why it is not in the table above: `brand`, where the
identity is generated. `tools/icon.sh` explains why the result is committed
here rather than fetched.

## The chip model, and the pins that do not follow it

`virtual-sx1262` is the one repository here that is consumed as a **submodule**
rather than fetched as a release or pinned in a manifest, and a submodule is a
commit, not a branch. Pushing to its `main` therefore changes nothing anywhere
else. Each consumer keeps the commit it recorded until somebody moves it, and
until then it builds, passes its tests, and runs the old chip.

Nothing detects that. There is no version to compare, no dependency resolver to
complain, and the symptom of a stale pin is a board behaving the way it did
before the fix - which is exactly what somebody chasing the fix expects to have
to look at. The DIO1 routing mask is the worked example: the model was correct
in its own repository for a day while every consumer still had the commit before
it.

So the rule is that **a change to `virtual-sx1262` is not done when it merges
there**. It is done when every pin has moved:

| consumer | pin |
|---|---|
| `MeshBench/meshcore-native` | `vendor/virtual-sx1262` |

That list is short today and will not stay short: QEMU and Renode both need the
same chip. It is duplicated in two places that are checked rather than
remembered - `virtual-sx1262`'s own CI asks GitHub what each consumer pins and
reports the ones that lag, and `.claude/skills/meshcoresim/SKILL.md` carries the
same table for anyone working here. Adding a consumer means adding a row to all
three in one commit.

Moving a pin is also not the same as proving one. The model is what every
emulated board's firmware talks to, so a bump is followed by booting a board and
watching it forward, not by a green build.

## What the documentation site takes from here

Six of its pages are written by tools rather than by hand: what the simulator
cannot do, from `docs/shortcomings.md`; the control verb reference, from
`docs/verb-reference.md`; the CLI reference, from `docs/cli-reference.md`; and
the three client APIs, read out of `pkg/client-go`, `pkg/client-python` and
`pkg/client-js`. The generators live in that repository, beside the pages they
write, and each takes a checkout of this one.

`.github/workflows/publish-docs.yml` runs them from the tag that cuts a
release, builds the site and pushes the result. It is the only thing that
does: the site's own build refuses a page that has fallen behind its source,
but only when that source is reachable, and on its runner this repository is
not there at all. Every one of the six had gone stale before the workflow
existed.

Between releases the site is behind this repository on purpose. It describes
the released version, so a reader following the CLI reference is given flags
their binary has, and the shortcomings page describes a build they can
actually download.

## Forks, and what we changed in each

Every one is a fork we carry a patch on, not a vendored copy. Upstream is listed
so the patch can be rebased, and so anyone can see how small each change is.

### `MeshBench/gio`, branch `meshbench-main`

`meshbench-main` is the integration branch and the default, so a clone gets the
patch rather than upstream. It was `msim-210-layer-shell`, which meant `go.mod`
pinned an unmerged feature branch and a reader landing on the repository saw
stock Gio with no sign of why the fork exists.

**Public, and checked rather than assumed.** It is the sharpest of these,
because it is the only fork the Go build itself needs: a `replace` directive in
`go.mod` points at it, so a recipient of the source archive cannot build
MeshBench at all without fetching it. If it were private the archive would meet
GPL-3.0 section 6 on paper and not in fact. It can be fetched with no
credentials, no `GOPRIVATE` and no `insteadOf` anywhere in CI, and `go.sum`
carries an entry verified against `sum.golang.org`, which a private module never
gets. `docs/compatibility.md` states the same thing where a reader looking for
the licence obligation will meet it.

Forked from `gioui/gio` (v0.10.2). Adds the `wlr-layer-shell` protocol to the
Wayland backend: the `app.LayerShell` option gives a window the
`zwlr_layer_surface_v1` role on a chosen layer instead of an `xdg_toplevel`,
with anchors, margins, exclusive zone and keyboard interactivity. A surface on
the overlay layer is stacked above every normal window by the compositor —
under Wayland the only mechanism a client has for that, which is why every
window that is not the main one asks for it (`internal/ui/float`). Compositors
without the protocol fall back to a normal window, which Gio reports in
`ConfigEvent.LayerShell`. The protocol XML is vendored in the fork because
`wlr-protocols` is not part of `wayland-protocols` and cannot be expected at a
system path.

Pinned in `go.mod` by a `replace` directive, the way a Go fork is carried: the
branch is upstream-plus-one-feature so a rebase is a tag away.

### `MeshBench/qemu` — branch `meshbench-main`

Forked from Espressif's QEMU fork (`esp-develop`, QEMU 9.2.2). Adds an SX1262
SPI device, a working GPIO implementation, and machine properties for the radio
wiring (`radio-path`, `radio-spi`, `radio-nss`, `radio-busy`).

Upstream's GPIO write handler is empty, and RadioLib drives NSS as an ordinary
GPIO rather than through the SPI controller's chip select — so without that
implementation the chip sees an unframed byte stream and the driver reports no
chip present.

Build with `--enable-gcrypt` or the `esp32` machine will not instantiate.

### `MeshBench/tlib` — branch `meshbench-main`

Forked from `antmicro/tlib`. One clause: SEVONPEND generates an event for *any*
exception entering the pending state, not only for ones the CPU would accept.

DDI0403E B1.5.17 does not qualify the event by whether the exception is enabled,
and the comment above the line already said "any exception" — but the code asked
`tlib_nvic_get_pending_masked_irq()`, which answers the narrower question. So
firmware that sets SEVONPEND, sleeps on `WFE` and then reads ISPR — handling the
source in thread mode with the interrupt deliberately disabled — never woke.
MeshCore's published nRF52 builds do exactly that.

### `MeshBench/renode-infrastructure` — branch `meshbench-main`

The C# half of the same fix: `AnyInterruptPending`, exported as `PendingIRQ()`
for the tlib callback. Its own `tlib` submodule points at our fork.

### `MeshBench/renode` — branch `meshbench-main`

Points `src/Infrastructure` at our fork, and carries a GitHub Actions workflow
that builds the **portable** package — the one that bundles the .NET runtime, so
a machine needs no dotnet installed. That is deliberate: setting MeshBench up
should be a download, not a toolchain.

The workflow also asserts both halves of the fix are present in the tree it
built, because a submodule quietly resolving to upstream would produce a Renode
that looks correct and hangs in exactly the same place.

## Taking a newer upstream

There is no calendar for this, and inventing one would be a promise nobody
keeps. Upstream is taken when there is a reason to take it: a security fix, a
board or a peripheral we need, or a bug we have hit and upstream has already
cured. The cost of waiting is real, though, so the rule is that a fork is never
more than one upstream release behind without somebody having decided it may be.

**Rebase, never merge.** Each `meshbench-main` is rebased onto the upstream tag
so the patch stays a short series of our own commits sitting on top of a named
upstream. A merge would bury it: the answer to "what did MeshBench change" has
to stay `git log upstream/<tag>..meshbench-main`, readable in one screen. If
that series ever stops being readable, the fix is to squash our own commits,
not to accept a fork nobody can audit. That readability is also what the
licences are built around, so it is a courtesy as well as a convenience.

**A release names both commits.** A tag on a fork records our commit *and* the
upstream tag it sits on, because "which upstream is this" is the first question
anybody debugging an emulator asks, and a bare SHA does not answer it.

**Cut releases from `meshbench-main`.** Not from a detached commit that happened
to build. The Renode release `meshbench-20260814-a784d99` is the example of why:
it targets `a784d994`, while `meshbench-main` is at `339f4df4`, and nothing
records how the two relate, so what shipped cannot be reproduced from a branch
name. The next Renode release should be cut from the branch tip and say which
upstream it carries.

**After any rebase, the emulated boards are the test.** The build asserting both
halves of the SEVONPEND fix are present catches a submodule that has quietly
resolved to upstream, but it does not catch a fix that still applies and no
longer works. Boot one emulated board and watch it relay before believing a
rebase.

## Not ours

`meshcore-dev/MeshCore` is upstream and unmodified. **Nothing in MeshCore is
patched** — the build points at a checkout and compiles it as it stands, which
is the whole basis of the claim that MeshBench runs real firmware. Board images
and the OTAFIX bootloader packages are downloaded from their releases and from
`flasher.meshcore.io` at runtime.

The Nordic SoftDevice is not ours and cannot be redistributed. Anyone running
published nRF52 firmware supplies their own copy.

## The move, and what is left of it

The four forks are moved. Done at the same time, because the references between
them are the awkward part rather than the repositories:

- **Submodule URLs**, both of them: `MeshBench/renode` → `src/Infrastructure`,
  and `MeshBench/renode-infrastructure` → `src/Emulator/Cores/tlib`. A submodule
  pointing at a moved repository keeps working through GitHub's redirect until
  it does not, and the failure is a build that silently resolves to upstream -
  an emulator that looks correct and hangs exactly where the fix was written to
  cure. The workflow asserts both halves of the SEVONPEND fix are in the tree it
  built, which is what catches that; keep it.
- **The release notes** the workflow writes, which link to the two branches.
- **Documentation**, here and in `docs/packaging-emulation.md`, `README.md` and
  the notes under `tools/renode/`.

`meshcore-native` and `meshbench-reports` followed. Two things there did not
follow a redirect:

1. **`NativeReleasesURL`** in `internal/firmware` names the releases repository
   in code rather than configuration, so the move was a code change.
2. **Published report URLs.** The reports site serves from a Pages domain
   derived from the account, so every link that was
   `a13xb0.github.io/meshbench-reports/...` is now
   `meshbench.github.io/meshbench-reports/...`. Anything already shared - the
   listen-before-talk report was given out as a keynote link - points at the old
   one.

Still to move: **`MeshBench/meshbench`** itself.
