# Repositories

Everything MeshBench depends on that we control, why each exists, and what we
changed. Written for the move into its own organisation: the awkward part of
that move is not the repositories, it is the references *between* them, and
those are listed here.

## Ours

| repository | what it is | licence |
|---|---|---|
| `A13xB0/meshcoresim` | MeshBench itself | none chosen yet — ADR-0001 |
| `A13xB0/meshcore-native` | host builds of MeshCore, `VirtualSX1262`, the bridge and `radioserver` | see its NOTICE |
| `A13xB0/meshbench-reports` | the published reports site | — |

## Forks, and what we changed in each

Every one is a fork we carry a patch on, not a vendored copy. Upstream is listed
so the patch can be rebased, and so anyone can see how small each change is.

### `A13xB0/qemu` — branch `meshbench-sx1262`

Forked from Espressif's QEMU fork (`esp-develop`, QEMU 9.2.2). Adds an SX1262
SPI device, a working GPIO implementation, and machine properties for the radio
wiring (`radio-path`, `radio-spi`, `radio-nss`, `radio-busy`).

Upstream's GPIO write handler is empty, and RadioLib drives NSS as an ordinary
GPIO rather than through the SPI controller's chip select — so without that
implementation the chip sees an unframed byte stream and the driver reports no
chip present.

Build with `--enable-gcrypt` or the `esp32` machine will not instantiate.

### `A13xB0/tlib` — branch `sevonpend-any-pending`

Forked from `antmicro/tlib`. One clause: SEVONPEND generates an event for *any*
exception entering the pending state, not only for ones the CPU would accept.

DDI0403E B1.5.17 does not qualify the event by whether the exception is enabled,
and the comment above the line already said "any exception" — but the code asked
`tlib_nvic_get_pending_masked_irq()`, which answers the narrower question. So
firmware that sets SEVONPEND, sleeps on `WFE` and then reads ISPR — handling the
source in thread mode with the interrupt deliberately disabled — never woke.
MeshCore's published nRF52 builds do exactly that.

### `A13xB0/renode-infrastructure` — branch `sevonpend-any-pending`

The C# half of the same fix: `AnyInterruptPending`, exported as `PendingIRQ()`
for the tlib callback. Its own `tlib` submodule points at our fork.

### `A13xB0/renode` — branch `meshbench`

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

## Moving to an organisation

The repositories are the easy part. These are the references that will break,
roughly in the order they will bite:

1. **Submodule URLs.** `A13xB0/renode` → `src/Infrastructure`, and
   `A13xB0/renode-infrastructure` → `src/Emulator/Cores/tlib`. A submodule
   pointing at a moved repository keeps working through GitHub's redirect until
   it does not, and the failure is a build that silently checks out upstream.
   The workflow's assertion is what catches that, so keep it.
2. **Release download URLs**, wherever packaging fetches the Renode and QEMU
   builds from.
3. **`internal/firmware`** — the native build catalogue resolves releases from
   `A13xB0/meshcore-native`. That name is in code, not configuration.
4. **Documentation links**, including `docs/packaging-emulation.md`, the Renode
   notes under `tools/renode/`, and this file.
5. **The reports site**, which publishes under a GitHub Pages domain derived
   from the account name.

Worth doing in one pass with the redirects still live, rather than discovering
them one build failure at a time.
