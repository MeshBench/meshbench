# Repositories

Everything MeshBench depends on that we control, why each exists, and what we
changed. Written for the move into its own organisation: the awkward part of
that move is not the repositories, it is the references *between* them, and
those are listed here.

## Where things live

Everything except MeshBench itself is in the **MeshBench** organisation.

| repository | what it is | licence |
|---|---|---|
| `MeshBench/meshbench` | MeshBench itself | none chosen yet — ADR-0001 |
| `MeshBench/meshcore-native` | host builds of MeshCore, `VirtualSX1262`, the bridge and `radioserver` | see its NOTICE |
| `MeshBench/meshbench-reports` | the published reports site | — |
| `MeshBench/qemu` | QEMU with our SX1262 | GPLv2, upstream's |
| `MeshBench/tlib` | the CPU library, with the SEVONPEND fix | upstream's |
| `MeshBench/renode-infrastructure` | the C# half of that fix | upstream's |
| `MeshBench/renode` | ties them together and builds the package | upstream's |

## Forks, and what we changed in each

Every one is a fork we carry a patch on, not a vendored copy. Upstream is listed
so the patch can be rebased, and so anyone can see how small each change is.

### `MeshBench/qemu` — branch `meshbench-sx1262`

Forked from Espressif's QEMU fork (`esp-develop`, QEMU 9.2.2). Adds an SX1262
SPI device, a working GPIO implementation, and machine properties for the radio
wiring (`radio-path`, `radio-spi`, `radio-nss`, `radio-busy`).

Upstream's GPIO write handler is empty, and RadioLib drives NSS as an ordinary
GPIO rather than through the SPI controller's chip select — so without that
implementation the chip sees an unframed byte stream and the driver reports no
chip present.

Build with `--enable-gcrypt` or the `esp32` machine will not instantiate.

### `MeshBench/tlib` — branch `sevonpend-any-pending`

Forked from `antmicro/tlib`. One clause: SEVONPEND generates an event for *any*
exception entering the pending state, not only for ones the CPU would accept.

DDI0403E B1.5.17 does not qualify the event by whether the exception is enabled,
and the comment above the line already said "any exception" — but the code asked
`tlib_nvic_get_pending_masked_irq()`, which answers the narrower question. So
firmware that sets SEVONPEND, sleeps on `WFE` and then reads ISPR — handling the
source in thread mode with the interrupt deliberately disabled — never woke.
MeshCore's published nRF52 builds do exactly that.

### `MeshBench/renode-infrastructure` — branch `sevonpend-any-pending`

The C# half of the same fix: `AnyInterruptPending`, exported as `PendingIRQ()`
for the tlib callback. Its own `tlib` submodule points at our fork.

### `MeshBench/renode` — branch `meshbench`

Points `src/Infrastructure` at our fork, and carries a GitHub Actions workflow
that builds the **portable** package — the one that bundles the .NET runtime, so
a machine needs no dotnet installed. That is deliberate: setting MeshBench up
should be a download, not a toolchain.

The workflow also asserts both halves of the fix are present in the tree it
built, because a submodule quietly resolving to upstream would produce a Renode
that looks correct and hangs in exactly the same place.

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
