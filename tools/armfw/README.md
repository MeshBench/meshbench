# SoftDevice-free nRF52840 firmware

The build that makes the Renode backend work.

```bash
tools/armfw/build.sh
renode  # with tools/renode/armfw.resc
```

## Result

```
   text    data     bss     dec filename
  28896    1408   33560   63864 fw.elf

  fw.bin 30304 bytes  SP=0x20040000 reset=0x000045C9  SVC halfwords: 5
```

Booted under Renode, the firmware's own UART output:

```
MSIM bare-metal nRF52840, no SoftDevice
```

That is **real MeshCore code — `Mesh`, `Dispatcher`, `Packet`, `Identity`,
`Utils`, plus Ed25519 and AES — cross-compiled for Cortex-M4 and executing on an
emulated nRF52840**, with MeshCore itself unmodified.

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

## Where it currently stops

41 UART writes — exactly the first line — then no further output. So it reaches
`main()`, initialises the UART, prints, and stalls before `Mesh::begin()`
returns.

The likely cause is not a hang: **Ed25519 key generation on an emulated
Cortex-M4 is very slow**. `LocalIdentity` does a Curve25519 scalar
multiplication, which is expensive in software and far worse under emulation.
Worth measuring with a longer run before assuming a fault — put a UART print
either side of the keygen to tell "slow" from "stuck" apart.

## Notes

- The UART writes appear in Renode's log as *unhandled writes to offset
  `0x51C`*. Renode's `NRF52840_UART` models the UARTE (EasyDMA) variant, so the
  legacy TXD register is unhandled — but the writes are still logged, and
  decoding them recovers the text exactly.
- `startup.c` carries newlib syscall stubs (`_sbrk` and friends). A bare-metal
  image has no OS to ask, and `_sbrk` in particular is needed because the crypto
  code allocates.
