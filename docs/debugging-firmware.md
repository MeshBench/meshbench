# Debugging a node's firmware

`node.debug` makes a running node debuggable and says how to attach. The
firmware is real, so the debugger is the ordinary one: gdb, lldb, or VS Code
with the C/C++ extension.

## Native nodes

A native node is an ordinary process. Ask for it:

```
node.debug {"node": "West Lomond"}
```

The answer has the process ID, the binary, whether the binary has symbols, a
gdb command line and a VS Code `attach` configuration:

```
gdb -p 2104750 ~/.cache/meshbench/firmware/native/.../meshcore-simple_repeater-linux-amd64
```

A native node normally has to answer every tick within 30 seconds or it is
marked down. Once `node.debug` has been called it is held instead. A
breakpoint stops it, the rest of the mesh carries on, and when the debugger
lets it go it catches up to the run's clock and rejoins. Its clock jumps by
however long it was stopped, so `sim.state` reports the run as not
reproducible from then on.

- `symbols` is `debug` when the binary carries DWARF, `symbols` when it has a
  symbol table only, and `stripped` when it has neither. For source-level
  debugging, build MeshCore from a checkout with `meshbench dev`.
- On Linux, attaching to a process that is not the debugger's own child needs
  ptrace permission: `kernel.yama.ptrace_scope` set to 0, or the debugger run
  as root.
- A native node runs `setup()` as soon as it starts, so a fault during boot
  cannot be caught by attaching afterwards.

`node.debug {"node": ..., "off": true}` puts the deadline back.

## Emulated boards

An emulated board's firmware runs inside QEMU or Renode, and the way in is the
emulator's GDB stub. `node.debug` restarts the board with one:

```
node.debug {"node": "Deck", "wait": true}
```

- `gdb_port` is where the stub listens. Pass `port` to choose it.
- `wait` holds the CPU at reset until a debugger continues, so boot can be
  stepped through.
- `debugger` is the gdb that speaks the board's architecture:
  `xtensa-esp32-elf-gdb` or `xtensa-esp32s3-elf-gdb` for the ESP32 boards
  under QEMU, `arm-none-eabi-gdb` (or `gdb-multiarch`) for the nRF52 boards
  under Renode.

The image a board runs is a flashable `.bin` or `.uf2` with no symbols. Point
the debugger at the `firmware.elf` the same build produced, for a PlatformIO
build `.pio/build/<env>/firmware.elf`:

```
arm-none-eabi-gdb -ex 'target remote 127.0.0.1:3333' .pio/build/RAK_4631_repeater/firmware.elf
```

The answer includes a VS Code configuration for this, with
`miDebuggerServerAddress` set to the stub.

QEMU's stub listens on loopback only. Renode's listens on every interface, so
only ask for one on a machine you trust the network around.

`node.debug {"node": ..., "off": true}` restarts the board without the stub.
