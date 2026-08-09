# SoftDevice-free nRF52840 firmware

The build that makes the Renode backend work.

```bash
tools/armfw/build.sh
renode  # with tools/renode/armfw.resc
```

## Result

```
   text    data     bss     dec filename
 108963    1824   38432  149219 fw.elf
```

Booted under Renode, the firmware's own UART output:

```
MSIM bare-metal nRF52840, no SoftDevice
SX1262 GetStatus: 20
Mesh::begin() ok
.....
loop x200 ok
sendFlood issued
TX OK — mesh stack ran on ARM
```

That is **real MeshCore code — `Mesh`, `Dispatcher`, `Packet`, `Identity`,
`Utils`, plus Ed25519 and AES — cross-compiled for Cortex-M4, executing on an
emulated nRF52840, driving an emulated SX1262 to a completed transmission**,
with MeshCore itself unmodified. `20` is `STBY_RC`: the radio answered.

## Why this was necessary

The published `.uf2` is linked above a SoftDevice (`0x26000` for s140 v6.1.1)
and contains **119 SVC call sites** into it. Without a matching SoftDevice it
executes the stack-fill pattern; with one it idles in `WFE` waiting on
proprietary Nordic behaviour Renode does not model.

But `examples/simple_repeater/main.cpp` has **no BLE code at all** — its only
conditionals are `ETHERNET_ENABLED` and `ENABLE_ADVERT_ON_BOOT`. The SoftDevice
is imposed by the Adafruit nRF52 Arduino core's linker layout, not by anything
MeshCore calls. Building it ourselves drops SVC sites from 119 to 5 (residual
data patterns, not instructions) and the image boots directly.

## A bug worth recording: UB deleted the firmware

Adding the SPI driver silently reduced the image from 30 KB to **346 bytes**,
with all 84 mesh symbols garbage-collected out. `main` was still present — at
72 bytes.

The cause was one line:

```c
uint8_t st = sx_cmd(0xC0, 0, 1);   // args = nullptr, n = 1
```

`sx_cmd` dereferences `args[0]`, so this is a null dereference. GCC is entitled
to assume undefined behaviour never happens, and deleted everything after the
call. No warning, no error — a clean build producing an image with no firmware
in it.

Fixed by passing a real NOP byte. Worth knowing because the symptom (a tiny
binary) points nowhere near the cause, and `-Os` with UB will do this again to
anyone who repeats the pattern.

## Two more silent traps, both of which looked like a hang

Neither produced a diagnostic. In both cases the firmware printed some output,
then stopped, with `Mesh` somewhere in the call stack and nothing to point at.

### 1. The FPU is off at reset

The Cortex-M4F powers up with CP10 and CP11 disabled. Compiling with
`-mfpu=fpv4-sp-d16` enables the *code generation*; it does not enable the
*hardware*. The first floating-point instruction takes a UsageFault.

`Dispatcher::begin()` computes

```cpp
float duty_cycle = 1.0f / (1.0f + getAirtimeBudgetFactor());
```

so MeshCore faults on the very first thing it does. `Reset_Handler` now sets
CPACR before touching anything else.

### 2. Static constructors were never run

`__libc_init_array()` was not called and `link.ld` had no `.init_array`, so
every file-scope C++ object kept the **zeroed vtable pointer it got from
`.bss`**. This does not fail to link and does not crash at startup; the program
dies much later, at the first virtual call through one of those objects —
here `_mgr->getNextInbound()`, most of the way into `Dispatcher::loop()`.

Fixing it needs four things together, and each one is its own link error:

| Symbol | Where it comes from |
|---|---|
| `.init_array` / `.fini_array` / `.preinit_array` | added to `link.ld` |
| `__dso_handle` | `PROVIDE(__dso_handle = 0)` — libstdc++'s `__cxa_atexit` wants it |
| `__exidx_start` / `__exidx_end` | bracket `.ARM.exidx`; the unwinder comes in with libstdc++ |
| `_init` / `_fini` | normally in `crti.o`/`crtn.o`, which `-nostartfiles` omits — stub them |

The image grows 29 KB → 109 KB, which is the libstdc++ startup machinery
arriving. That is the cost of correct C++ semantics on bare metal, not bloat to
be optimised away.

## Make a fault say so

Both of the above presented identically — output stops, nothing else happens —
because `Default_Handler` was `for (;;) {}`. Replacing it with a handler that
prints IPSR, the stacked PC and CFSR turned the second one from a guess into
an address:

```
FAULT vec=00000006 pc=00001238 cfsr=00020000
```

`vec=6` is UsageFault; CFSR bit 17 is **INVSTATE** — a branch to an address with
the Thumb bit clear, which for a C++ program means a bad vtable. `addr2line`
on `0x1238` gave `mesh::Dispatcher::loop()`, and the disassembly showed
`blx r6` loaded from `_mgr`'s vtable.

The handler pokes UART0 TXD directly rather than calling the C++ `uart_putc`: a
fault handler must not depend on the rest of the program still being sane.

**Renode's log deduplicates identical consecutive lines**, appending `(N)`. A
naive decoder reads `cfsr=00200` — dropping exactly the repeated zeros that
carry the meaning. The decoder must expand the repeat count or every hex value
it recovers is wrong.

## Notes

- The UART writes appear in Renode's log as *unhandled writes to offset
  `0x51C`*. Renode's `NRF52840_UART` models the UARTE (EasyDMA) variant, so the
  legacy TXD register is unhandled — but the writes are still logged, and
  decoding them recovers the text exactly.
- `startup.c` carries newlib syscall stubs (`_sbrk` and friends). A bare-metal
  image has no OS to ask, and `_sbrk` in particular is needed because the crypto
  code allocates.
