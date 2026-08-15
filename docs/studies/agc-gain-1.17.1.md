# The 1.17.1 receive-gain fix does not reach a variant without the macro

Report: https://claude.ai/code/artifact/73947dca-d786-4a66-935b-346322107b46
Data: `agc-gain-1.17.1-sweep.json`

## The finding

MeshCore 1.17.1 changed `sx126xResetAGC` to take the operator's runtime value
instead of the compile-time macro:

```
-inline void sx126xResetAGC(SX126x* radio) {
+inline void sx126xResetAGC(SX126x* radio, bool rx_boost_gain) {
 #ifdef SX126X_RX_BOOSTED_GAIN
-  radio->setRxBoostedGainMode(SX126X_RX_BOOSTED_GAIN);
+  radio->setRxBoostedGainMode(rx_boost_gain);
 #endif
```

The corrected line is still inside `#ifdef SX126X_RX_BOOSTED_GAIN`. On a variant
that never defines that macro the statement is removed by the preprocessor, so
the AGC reset re-applies nothing and boosted gain enabled at runtime is gone
until reboot - in 1.17.1 exactly as in 1.17.0. `_prefs.rx_boosted_gain` and the
CLI `get` keep reporting the operator's value throughout.

A variant that does define the macro is genuinely fixed.

## Three lines of evidence

**Source.** The diff above, `repeater-v1.17.0..repeater-v1.17.1`.

**Register.** `TestAnAGCResetLosesBoostedGain` boots each version, sends
`set radio.rxgain on` then `set agc.reset.interval 4`, and reads 0x08AC over the
bridge. Both take 0x96 and both drop to 0x94 two seconds after the first reset
becomes due, with no recovery over the following 22 seconds.

**Mesh.** 161 nodes, resets crossed against version, two seeds, eight cells.
With resets on, 1.17.1 receives 3337 against 1.17.0's 3332 - 0.2% apart - and
both deliver exactly 1182 messages. Enabling resets costs 6.5% of receptions and
2.9% of deliveries.

## Why the first attempt measured nothing

`rx_boosted_gain` defaults to 0 (`CommonCLI.h:66`). The fault destroys a runtime
setting; the first sweep enabled the AGC reset and never enabled boosted gain,
so the reset fired every four seconds against a register already at its reset
value. All four arms were identical and the bench said so accurately.

Both preconditions are needed: `set radio.rxgain on` and
`set agc.reset.interval > 0`. Resets are also off by default
(`Dispatcher.cpp:133`), so a stock node is unaffected.

## What a decibel is worth on this mesh

Share of a run's decodes that a less sensitive receiver would have cost,
measured rather than assumed:

| lost | decodes at risk |
|---|---|
| 1 dB | 5.9% |
| 2 dB | 12.4% |
| 3 dB | 19.3% |
| 6 dB | 36% |
| 10 dB | 57% |

12.4% at risk against 6.5% actually lost: about half the individual losses are
recovered by the mesh's redundancy, which is the fraction aggregate delivery
counts hide. It is also why the result does not rest on
`RxBoostedGainImprovementDB`, which is a placeholder.

## Caveats

Two seeds. One arm - 1.17.1 with resets off - spread 5% across its seeds, wider
than several differences discussed; nothing here rests on that row. The host
variant does not define the macro, which is the point, but it means this
measures the fault's reach rather than its severity where 1.17.1 does apply. And
the simulator is kinder than the air: no multipath, bare earth, ideal
demodulator, so treat it as a best case.
