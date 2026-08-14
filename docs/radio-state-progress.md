# Radio state from firmware — where this got to

Working notes for the branch `msim-radio-state-from-firmware`. The plan is in
`docs/radio-state-from-firmware.md`; this is what has actually been built, what
was learned doing it, and what is left. Written down because the Bench UX work
interrupted it.

## Built and committed

Branch `msim-radio-state-from-firmware`, three commits:

- `4423f63` docs: the plan
- `9a7867d` W1 — the engine reads what the firmware set the radio to
- `183a5c9` W3 — the front-end module gates transmit power

### W1 — radio state crosses the bridge

`kRadioStats` (0x09) grew a tail: 16 bytes of counters as before, then the whole
configured radio. Read on length, so an older peer that stops at 16 still parses.

| Offset | Field |
|---|---|
| 0–15 | irqReads, busyReads, busyMs, spuriousRaises (unchanged) |
| 16 | RX gain register 0x08AC |
| 17 | transmit power, int8, −128 = never set |
| 18 | FEM line live level |
| 19 | mode: 0 standby, 1 rx, 2 tx, 3 cad |
| 20–21 | SF, CR |
| 22–25 | frequency Hz |
| 26–29 | bandwidth Hz |
| 30–31 | preamble symbols |
| 32–33 | IRQ mask |
| 34–35 | IRQ flags |
| 36 | FEM at transmit: 0 never transmitted, 1 module out, 2 module in |

Go side: `firmware.RadioStats` carries all of it plus `Configured`.
`engine.ApplyRadioState` is called per node per tick from `runFirmware`, after
`WaitAdvance`. `effectiveRF` in `internal/engine/radiostate.go` turns board
baseline plus reported state into effective transmit power and noise figure.

Pre-existing bug fixed on the way: **noise figure was never per-node.**
`scenario.Node.NoiseFigureDB` has been populated from the board profile since
import and the engine only ever used the run-wide `Config.NoiseFigDB`. Four call
sites now use `noiseFigOf`. The path-loss cull takes the *quieter* of a pair,
because culling on the worse receiver discards links the better one closes.

`geometryFingerprint` now hashes `NoiseFigureDB`. It feeds the cull, so without
it a stale link matrix loads from disk and looks authoritative.

### W2 — gain register drives noise figure

`VirtualSX1262` handles `kCalibrate` (0x89) and returns 0x08AC to its reset
default. That is the mechanism behind the fault. `RxBoostedGainImprovementDB = 2.0`
in `internal/engine/radiostate.go` — **a chosen figure, not a cited one**, see
unknowns below.

### W3 — FEM gates transmit power

`scenario.FEM{TxGainDB, TxLossDB}` on `Board` and `Node`. The enable line reaches
the chip from the emulator as a GPIO: new `radio-fem` machine property on the
QEMU fork, `sx1262-fem` named GPIO input on the device, `kSetFem` (0x06) tag to
radioserver.

Judged at the instant of transmit, not the live level — see the live-run finding
below.

Boards: `Heltec_t096` added (13 dB KCT8103L, from upstream's own
"9dBm + ~13dB" comment), `Generic_E22_sx1262` gained `FEM: 13` wiring and a
switch-isolation model.

### Tests

`internal/engine/radiostate_test.go` — 8 unit tests, no emulator needed.
`internal/engine/radiostate_live_test.go` — observes a real image's radio.
`internal/engine/firmware_ab_live_test.go` — A/B harness, writes JSON.

## What running it actually taught

**The live run caught a modelling error.** Reading the FEM line's *current* level
docked an idle E22 25 dB for the ordinary state of listening: RadioLib holds the
line low while receiving and only raises it just before `SetTx`. The state that
decides anything is the one at the moment of transmission, latched in `startTx()`.
Three states, not a bool, because a node that has not transmitted has not
answered the question.

**Observed timeline, published 1.17.0 image, Generic_E22 under QEMU:**

```
   500 ms  gain=0x94 tx=-128 femAtTx=0 mode=0 sf=10 bw=250000   (reset defaults)
  8500 ms  gain=0x94 tx=22   femAtTx=0 mode=0 sf=8  bw=62500    (modem configured)
 18500 ms  gain=0x96 tx=22   femAtTx=0 mode=1                   (boosted applied)
 38500 ms  gain=0x96 tx=22   femAtTx=2 mode=2                   (TXEN high, transmit)
 40000 ms  gain=0x96 tx=22   femAtTx=2 mode=1                   (back to receive)
effective: tx=22.0 dBm, noise figure=4.0 dB
```

**The A/B came out identical, and the reason matters.**
`CommonCLI.h:49` — `uint8_t agc_reset_interval = 0` — and `Dispatcher.cpp:133`
guards on `getAGCResetInterval() > 0`. **AGC resets are off by default, so the
1.17.1 gain fault never fires on a stock node.** It only ever bit operators who
had explicitly set `agc.reset.interval`. To provoke it the bench must send
`set agc.reset.interval 4` first — the CLI takes seconds and stores seconds/4,
so 4 gives a reset every 4 s.

## Findings from the source, with citations

Receive gain, RadioLib `SX126x_registers.h:71,119-122`:
`RADIOLIB_SX126X_REG_RX_GAIN = 0x08AC`, boosted `0x96`, power saving `0x94`,
retention list at `0x029F-0x02A1`.

**The bug is MeshCore's, not RadioLib's.** RadioLib 7.7.1 has a correct
`SX126x::resetAGC()` that restores from cached runtime state
(`this->rxBoostedGainMode`). MeshCore does not call it — it reimplements the
sequence in `src/helpers/radiolib/SX126xReset.h:28-30` and re-applies the
**compile-time** `SX126X_RX_BOOSTED_GAIN` macro, discarding whatever the operator
set. Reached from `Dispatcher.cpp:133-136`.

Two failure modes: a variant with the macro undoes a runtime change; a variant
*without* it (generic-e22 among them) re-applies nothing at all and boosted gain
is gone until reboot. In both cases `_prefs.rx_boosted_gain` and the CLI `get`
keep reporting the operator's value — **firmware state and chip state
desynchronise with no log line anywhere.**

Runtime control is `set radio.rxgain on|off`, `CommonCLI.cpp:534-542`.
69 variants define the macro; `station_g2` and `station_g3_esp32` set it to 0.

T096 front end: `variants/heltec_t096/LoRaFEMControl.cpp`, three GPIOs —
`P_LORA_PA_POWER` 30 (LDO), `CSD` 12 (shutdown), `CTX` 41 (path select).
Driven from `T096Board::onBeforeTransmit/onAfterTransmit`.
`platformio.ini:24-25`: `LORA_TX_POWER=9`, `MAX_LORA_TX_POWER=22`,
"9dBm + ~13dB KCT8103L gain".

The T096 `-1` is not unique: `lilygo_techo_lite/variant.h:124` and
`mesh_pocket/variant.h:111` carry the same `PIN_SPI1_MISO (-1)` against a
48-entry `g_ADigitalPinMap`.

E22 RF switch: `platformio.ini:17-18` `SX126X_TXEN=13`, `SX126X_RXEN=14`, driven
by `setRfSwitchPins(RXEN, TXEN)` from `CustomSX1262.h:80-88`. RadioLib raises it
in `launchMode()` before `SetTx`. **Upstream also sets
`SX126X_DIO2_AS_RF_SWITCH=true`, which its own `variant.h` warns against** — so
on this board DIO2 may switch the path regardless of the MCU pins.

Board assets are `.uf2` for T096, T-Echo, T-Echo Lite, T-Echo Card, Minewsemi and
R1Neo — all nRF52, so Renode rather than QEMU. Only T-Beam Supreme S3 is ESP32-S3.

## Unknowns still open

1. **`RxBoostedGainImprovementDB = 2.0` is invented.** Neither RadioLib nor
   MeshCore states a dB figure anywhere. SX126x datasheet section 9.6 is the
   authority and this must be reconciled before any sensitivity number derived
   from it is published.
2. **Whether `CALIBRATE_ALL` really clears 0x08AC.** MeshCore's own comment says
   "RX settings that calibration *may* reset". We model the chip behaving the way
   the firmware's authors assumed. Same datasheet section.
3. The E22's 25 dB switch isolation is plausible, not measured.
4. T096 antenna and sleep figures are borrowed from comparable nRF52840 boards.

## Environment, so this is not rediscovered

- Development is on **elite**, 12 cores, display `:1`, Wayland.
- `~/msim/MeshCore` — firmware source, **1.17.0-era**, on a local study branch.
  No 1.17.1 tag, so the upstream fix diffs are not available locally.
- `~/msim/meshcore-native` — `VirtualSX1262`, bridge, radioserver.
  **Not a git repository.** Changes are backed up in this session's scratchpad
  under `meshcore-native-changes/`; they must be ported to a real clone.
- `~/msim/espqemu-src` — the QEMU fork, `hw/ssi/sx1262.c` and `hw/xtensa/esp32.c`.
  Also unversioned here. `~/.cache/meshcoresim/tools/qemu-system-xtensa` is a
  **symlink** into its build dir, so `ninja qemu-system-xtensa` goes live at once.
- `~/.cache/meshcoresim/tools/radioserver` is a real binary; rebuild with
  `g++ -std=c++17 -O2 -I variants/host bridge/radioserver.cpp variants/host/VirtualSX1262.cpp`.
- Native node binaries: `MESHCORE=~/msim/MeshCore CRYPTO=~/msim/arduinolibs/libraries/Crypto ./build.sh simple_repeater`.

Running the live tests:

```
MESHCORESIM_LIVE=1 \
MESHCORESIM_QEMU=~/msim/espqemu-src/build/qemu-system-xtensa \
MESHCORESIM_RADIO_SERVER=~/.cache/meshcoresim/tools/radioserver \
MESHCORESIM_NATIVE=~/msim/meshcore-native/build \
go test ./internal/engine/ -run TestTheRadioReports... -v -timeout 400s
```

## Driving the running app

Not MCP — the workbench's own control socket, per `.claude/skills/meshcoresim`.
`$XDG_RUNTIME_DIR/meshcoresim.sock`, newline-delimited JSON,
`{"id":1,"method":"<verb>","params":{}}`. `session.verbs` returns all 174.
The scratchpad has a `msim.py` helper.

Screenshots: `spectacle -a -b -n -o file.png` after
`kdotool windowactivate` on the window named "MeshBench workbench".

## The bench, where it was interrupted

Set up and ready to run:

- Fixture `fixture-scotland-ireland-strict` opened, 311 nodes.
- **Native `repeater-v1.17.1` downloaded** via `firmware.download` — it is
  published at `MeshBench/meshcore-native` (released 2026-08-14 15:57), so a
  311-node native A/B is possible. `repeater-v1.17.0` was already installed.
- Emulated E22 images for both versions are cached, for the emulated arm.

The gestures for the A/B, from `internal/session/experiment.go`:

```
experiment.vary   {"parameter":"repeater_version",
                   "values":["repeater-v1.17.0","repeater-v1.17.1"]}
experiment.seeds  {"seeds":[9001, 4417]}
experiment.senders{"senders":["<a companion name>"]}
experiment.base   {"run_for_ms":..., "send_at_ms":...}
experiment.start
```

**Provisioning `extra` is the hook for enabling AGC resets** —
`internal/session/provisioning_settings.go:231` splits it on newlines and sends
each line to every node. `set agc.reset.interval 4` there is what makes the
1.17.1 fault reachable at all.

## Also outstanding

- A node-window **Radio tab** showing everything the chip reports. W1 already
  carries the data; this is a rendering job. Alex's idea, and arguably a better
  bug-catcher than either test, because it catches configuration faults nobody
  thought to assert.
- The Bench view UX rebuild, which is what interrupted this.
