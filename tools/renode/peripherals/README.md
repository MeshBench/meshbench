# SX1262 peripheral

**The single blocker for the emulated backend on both architectures.**

MeshCore's `radio_init()` drives an SX1262 over SPI. Neither Renode's nRF52840
platform nor Espressif's QEMU models one, so RadioLib waits on a chip that never
answers:

- **nRF52 / Renode** — firmware stalls
- **ESP32 / QEMU** — task watchdog fires, seven resets in 120 s

One peripheral unblocks both.

## Status

Compiles, loads, attaches, and speaks the protocol:

```
GetStatus            -> 0x20   chip mode STBY_RC (2)
SetStandby, status   -> 0x26   mode 2, command status 3
GetDeviceErrors      -> 0x0000 no errors
WriteBuffer AA BB CC
ReadBuffer           -> 0xAA 0xBB 0xCC
```

```bash
renode tools/renode/peripherals/sx1262_test.resc
```

## Why it is tested in isolation first

This is the piece where being subtly wrong produces a model that *looks* like it
works and silently corrupts every timing result downstream. So it gets driven
directly with a RadioLib-style command sequence before any firmware depends on
it.

That paid immediately: the first version declared `WriteBuffer` and `ReadBuffer`
as fixed-length. They are **variable-length** — they run until chip select
deasserts — so the model parsed payload bytes as opcodes. `ReadBuffer` returned
`0xAA` and then garbage. The test caught it; firmware would have shown it as an
inexplicable radio failure much later.

## Modelled

Command protocol, status byte (chip mode in bits 6:4, command status in 3:1),
IRQ flags, the 256-byte data buffer, register space, and the argument count for
every opcode RadioLib issues. Unknown opcodes are logged rather than guessed at,
because a wrong argument count desynchronises the entire SPI stream.

## Not modelled

Modulation. `internal/rf` does that. This peripheral's job is to make the
firmware believe it has a radio and to hand transmitted frames to the simulator —
the same division as the host backend's `SimRadio`.

## Next

Wire `SetTx` / `SetRx` / `WriteBuffer` through to the RF engine so an emulated
node participates in the same channel as native ones, and drive the BUSY line so
RadioLib's `waitForBusy` sees realistic timing rather than instant completion.
