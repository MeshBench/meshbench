> **Working note, last true on 12 August 2026.** Kept for the thinking in it, not maintained as a description of the code. **2 of the 2 package paths it names no longer exist**, the seven-layer restructure of 19 August having moved them. Where this disagrees with the tree, the tree is right; the authority is the board matrix in `README.md`.

# Download the published firmware and run it

The goal: pick a board and a version in the workbench, fetch the same binary the
flasher would give you, and run it as a node. No build step, no shell.

Status: **plan.** What already works is marked.

## The one thing in the way

ESP32 firmware already boots under Espressif's QEMU fork. Real, unmodified,
straight from a release (`tools/esp32/`). The ROM bootloader runs, the app
loads, ESP-IDF starts:

```
ets Jul 29 2019 12:21:46
rst:0x1 (POWERON_RESET),boot:0x12 (SPI_FAST_FLASH_BOOT)
entry 0x400805e4
```

Then `main.cpp` calls `radio_init()`, RadioLib waits on an SX1262 that isn't
there, and the watchdog resets the board. That is the whole blocker. Everything
else on this page is small by comparison.

**And we already have the missing piece.** `VirtualSX1262` in
[meshcore-native](https://github.com/MeshBench/meshcore-native) satisfies RadioLib
7.6.0's unmodified driver: opcodes, status bytes, register reads, buffer
addressing, IRQ semantics, TX and RX-done timing. It has carried 154 nodes
through hundreds of runs. It just needs to be reachable from inside QEMU instead
of only from the native process.

So the work is: **make the existing radio model answer SPI from QEMU.**

## Step 1: spike the QEMU SPI attachment

Before writing anything, find out how Espressif's fork lets a device sit on SPI2,
and whether it can be done without patching QEMU. If it needs a patched QEMU
that is a maintenance cost worth knowing on day one. Timebox it.

This is the assumption the whole path rests on, so it goes first.

## Step 2: the radio model, reachable

Expose `VirtualSX1262` over a socket: chip-select asserted, N bytes out, N bytes
back, plus the `BUSY` and `DIO1` lines. The native path keeps calling it
in-process and does not change.

The QEMU device is then a forwarder, not a second implementation. Same for
`tools/renode/peripherals/VirtualSX1262.cs`, which already assumes a socket and can
become a forwarder too, deleting about 380 lines of C#.

Its link to the RF engine already exists and stays exactly as it is.

**Done when** the published `Heltec_v3` repeater binary gets past `radio_init()`
and prints its banner with no watchdog reset.

## Step 3: download and import

A node's firmware is already a role and a version resolved by `NativeCatalogue`
against GitHub releases. Published board firmware is the same shape from the same
API.

The flasher at `meshcore.io/flasher` is a browser client over MeshCore's release
assets; there is no separate manifest to consume. Asset names carry everything:

```
Heltec_v3_repeater-v1.17.0-727fc05-merged.bin
└─ board ─┘└─ role ─┘└ version ┘└ sha ┘└ full flash image
```

So: list assets for a tag, parse board, role and format, cache by
`(board, role, version)`, verify the digest against the release.

Two ways in, both landing in the same cache:

- **Download** from the release, which is the common case.
- **Import** a local `.bin` or `.uf2`, for builds that were never released.

`-merged.bin` is a full flash image, which is what QEMU wants handed to it. Two
traps already paid for in `tools/esp32/README.md`: flash size must match the
image header, and QEMU accepts only 2/4/8/16 MB images, so pad with `0xFF`.

## Step 4: only show hardware that will actually run

`internal/scenario/boards.go` already records `MCU`, `Radio` and `Vendor` per
board. A board is offered when all three hold:

- its MCU has a working emulator: **ESP32 and ESP32-S3**,
- its radio is one we model: **SX1262**, and note that **SX1268 is a different
  part** which several published boards use,
- an asset exists in the right format.

Of 89 boards in a release, 53 are ESP32-family and 45 ship a `-merged` image.

Show the incompatible ones greyed with the reason rather than hiding them. A
board missing for an unstated reason reads as a bug; "SX1268 radio, not
modelled" reads as a fact. Per-board pin maps are a long tail, so support is
declared and verified per board, never inferred from the MCU family.

## Step 5: prove it, small

Two nodes, one message, one seed. Node A native, node B running the downloaded
binary, same MeshCore version and radio parameters. Then swap and repeat.

| Assertion | Tolerance |
|---|---|
| Transmitted frames, byte for byte | none. MeshCore builds the same packet whatever carries it |
| Delivery outcome per node | none. A two-node link with this margin has no reason to differ |
| `getEstAirtimeFor()` against our channel's airtime | exact, per CLAUDE.md. If they disagree, CSMA desynchronises silently |
| Event timing | ordering exact, drift quoted |

If frames and delivery match, every native result transfers to the flashed bytes.
If they do not, that is worth more than the backend cost.

## Timing: enough to know before mixing nodes

A native node runs in lockstep, the engine supplies the clock, ten simulated
seconds take three milliseconds and the same seed always gives the same answer
(`internal/firmware/lockstep.go`). QEMU runs on wall clock by default, and
MeshCore's CSMA is built on `millis()`.

For "download it and watch it run" that is fine, and it is a reasonable first
milestone. The moment an emulated node shares a scenario with native ones it is
not, because the order two nodes reach the channel would depend on host
scheduling. The fix is QEMU `-icount` with the engine driving virtual time,
which is the contract the native bridge already implements.

If `-icount` does not hold up under a real ESP-IDF image, emulated nodes stay
single-node conformance tools. That is a reduced outcome, not a failure.

## nRF52 published binaries: not in this plan

Worth saying plainly, because it looks like an omission and it means **RAK4631
and the other 40 `.uf2` boards are not covered**.

Those images are linked above a Nordic SoftDevice and make 119 SVC calls into it.
Renode boots them as far as the stack-fill pattern without one, and the
SoftDevice is a proprietary binary Renode does not claim to emulate faithfully.
`tools/renode/README.md` has the measurements, including two wrong diagnoses
disproved by identical instruction counts.

Our own SoftDevice-free ARM build already runs there (`tools/armfw/`) and still
cross-checks compiler, word size and ARM codegen. But it is not the flashed
bytes, and it should not be described as if it were.

## Risks worth stating

- **QEMU may not take an SPI device without patching.** Step 1 exists to find
  out early.
- **Emulated nodes are slow.** Native runs about 3300× real time. A 300-node
  emulated scenario is not the idea; a handful of emulated nodes in an otherwise
  native mesh is.
- **The long tail is boards, not architectures.** One board working does not
  make fifty work.

## The flood column was the chip model, twice (2026-09-03)

Every `✗` in the **flood** column above, on both MCU families, came from two
faults in `VirtualSX1262` rather than from anything about the boards. Both are
fixed, in
[MeshBench/virtual-sx1262](https://github.com/MeshBench/virtual-sx1262) and
released as `radioserver-v3`, which was the last of them: the chip moved into
the emulators and that process was retired.

**The carrier-detect flags outlived the carrier.** `PREAMBLE_DETECTED` and
`HEADER_VALID` were latched and never cleared when the air went quiet.
MeshCore's `CustomSX1262::isReceiving()` reads `HEADER_VALID` to mean "the
channel is busy now", and its stale-flag timeout is about four seconds, so on a
mesh whose adverts arrive every few seconds that window never closed. A repeater
with a packet to forward never saw a clear channel. That is exactly the
"channel busy for essentially the whole run" measured on the nRF52 boards.

**DIO1 was gated on the wrong mask.** `SetDioIrqParams` carries an IRQ enable
mask and then a DIO1 routing mask; the model read the first and used it for the
pin. RadioLib enables `RxDone`, `Timeout`, `CrcErr`, `HeaderValid` and
`HeaderErr` in the status register but routes only `RxDone` to DIO1, so the pin
went high part-way through the carrier and was still high when `RxDone` arrived.
DIO1 is a level, and RadioLib attaches it on the rising edge, so there was no
edge for the packet: MeshCore's `recvRaw` is gated on the flag that interrupt
sets, and the frame decoded perfectly and was never read out of the chip.

Measured on `Ebyte_EoRa-S3` under QEMU, eight interleaved rounds per arm on an
otherwise idle machine: **2 of 8 before, 8 of 8 after** (Fisher exact
p = 0.007). Interleaved and on an idle machine deliberately, because this
measurement is biased by whatever else the host is doing - the same binary
scored 5 of 5 and then 1 of 4 while a compile was running.

## And the nRF52 boards were failing for two more reasons (2026-09-03)

Fixing the mask made three ESP32-S3 boards forward and left every nRF52840 board
exactly where it was. That group turned out to be failing for two further
faults, in different layers, each of which alone was enough.

**CryptoCell read modular operands too narrow.** MeshCore builds every nRF52
board with `-D USE_CC310_HW_CRYPTO=1`, so `Identity::verify` is
`CRYS_ECEDW_Verify` running Ed25519 in hardware. Our CC310 model loaded each
operand at the operation's own width, but the firmware keeps field elements
*unreduced* in the 320-bit registers - 19,087 plain `ADD` against 4 `MODADD` in
one verification - and hands them to a 256-bit `MODMULN` expecting the machine
to take the whole value and reduce it. That headroom is what Arm's `A=64` in
`Np = floor(2^(N+A+X-1)/n)` exists for. Reading only the operation's width threw
away the top of every accumulated operand, so the signature never verified, the
advert was dropped as forged, and the packet never reached the forwarding
decision at all.

It hid because the arithmetic was never wrong: all 16,626 PKA operations in a
verification check exactly against arbitrary-precision integers, and the decode,
opcode numbers and status bits all match Arm's own `pka_hw_defs.h`. A correct
operation on a truncated operand looks perfect in isolation. ESP32 boards have
no CryptoCell and verify in software, which is why they were never affected.

**An undriven input reads low, and a low on a button pin is a button held
down.** `heltec_t114`, `heltec_t096` and `heltec_mesh_solar` configure
`PIN_USER_BTN` as a plain `INPUT` and rely on the board's external pull-up, which
Renode has no notion of. MeshCore saw a long press within seconds of boot,
printed "Powering Off" and shut the node down. `rak4631` and `xiao_nrf52` use
`INPUT_PULLUP` and were never affected, which is exactly why fixing the crypto
made those two forward while the other three still failed. The board profiles
carry the pins that are held high in copper, the same way they carry the radio's.

All five now forward 2 of 2 on their published images. Two of them pass on the
CryptoCell fix alone, and the three that needed the button fix are exactly those
whose variants use plain `INPUT`, so the two are independently evidenced rather
than a pair that happen to work together.

**What is still open.** `LilyGo_TDeck` forwards nothing, on the same emulator and
model as the ESP32-S3 boards that do. Re-measured on 4 September against the
chip model loaded inside QEMU, which is the arrangement the other three S3
boards pass under, and it reproduced: adverts unprompted at 34.0 s, is heard,
answers its console after a 15 s idle, and forwards 0 of 2. So this is the
board's own row rather than something the old radio server was doing to it,
and the columns either side of the failure are the useful part - it transmits
and it is heard, and only the forward is missing.

Two traps sit in front of anybody re-measuring this. A stale toolchain reads as
a board fault: a tools directory without `virtual-sx1262` fails the boot
outright, and one carrying a QEMU from before the chip moved inside it gets as
far as "firmware started but never connected", neither of which is the board.
`resource.list` is the check - the pinned build reads `on disk` and anything
else reads `available`. And `rx` passing proves less than it appears, because
the engine records a delivery before the firmware reads it; `flood` is the
column that shows the firmware actually got the packet.

And no nRF52 board has a working filesystem: `IdentityStore::save()` fails
every boot, so nothing a repeater stores survives a restart - tracked
separately.

## Where the board matrix's failures are (last true: 2026-08-29)

The README's compatibility matrix links here for what each ✗ turned out to
be, and why it is not the board's fault.

- The three nRF52 boards that failed **flood** reported their channel busy for
  essentially the whole run, 241 seconds of 250 on one measurement against zero
  on a board that relays. That diagnosis was wrong: `busyreads` is 0 on those
  boards, so the firmware never once read the channel as busy. They were failing
  for the two reasons above, and all five forward now.
- The two ESP32-S3 boards now boot and reach their application. They used to
  restart for ever without finishing startup, 360 times in one probe,
  asserting in ESP-IDF's `do_core_init` on `esp_flash_init_default_chip()`.
  The flash driver's `set_io_mode()` was returning
  `ESP_ERR_FLASH_NO_RESPONSE`, which ESP-IDF forgives only when the flash is
  not in quad mode: the ESP32 boards are built for DIO and were forgiven,
  the S3 boards are built for QIO and were not, which is why it looked like
  an S3 fault rather than a flash one. The emulator's flash model knew
  GigaDevice parts by name and handled their quad-enable bit nowhere, so the
  bit could be written and never took.
- Both ESP32-S3 boards now reach the air. Two things were wrong and neither
  was where it looked. Their radios are not on the controller the profiles
  named: Arduino's default `SPIClass` is HSPI, and HSPI is controller 2 on
  an ESP32 but controller 3 on an ESP32-S3, and the machine modelled only
  the flash controller's registers, using that layout for the
  general-purpose one too, where a transfer starts on a different bit and
  the data sits at a different offset. And every input pin read low out of
  reset, GPIO0 included; it is a strapping pin whose pull-up holds it high,
  and reading it low is a program button held down, so MeshCore powered the
  board off after two minutes, every time, before it had adverted once.
- What those two failed then was **flood**, which turned out not to be a board
  property at all: see the two notes above.
- Three boards put the application's `Serial` on USB Serial/JTAG rather than
  UART0 (`ARDUINO_USB_CDC_ON_BOOT`): the T-Deck, the RAK3112 and the Heltec
  Wireless Tracker. Until that peripheral carried bytes they read as boards
  that started and then fell silent; the T-Deck's row is measured with it
  carrying bytes.
- `Station_G2` has no emulation wiring recorded yet. `Heltec_v2` carries an
  SX1276, which is not modelled: the chip here is an SX1262.
- **power** was untested rather than failed on the nRF52 boards, because Renode
  modelled two UARTs and no USB device while the Adafruit core puts `Serial` on
  USB CDC. `NRF52840_USBD` is that device, and those boards answer their console
  now; the rows here predate it and have not been measured again.
