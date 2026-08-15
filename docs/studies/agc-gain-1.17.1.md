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

## It needs no operator action beyond enabling the reset

The struct default in `CommonCLI.h:66` is 0, and that is the figure an earlier
draft of this study leaned on. It is not what the repeater runs.
`examples/simple_repeater/MyMesh.cpp` overrides it:

```
#ifdef SX126X_RX_BOOSTED_GAIN
  _prefs.rx_boosted_gain = SX126X_RX_BOOSTED_GAIN;
#else
  _prefs.rx_boosted_gain = 1; // enabled by default;
#endif
```

and applies it a few lines later through `radio_driver.setRxBoostedGainMode`,
which is not guarded. So on precisely the variants where the restore is compiled
out, boosted gain is **on from boot with no operator involvement at all** -
verified: a node on fresh storage reports `get radio.rxgain -> on` and holds
0x96 before anything has been typed at it.

The only precondition is therefore `agc.reset.interval > 0`, which does default
to 0 and does guard the reset (`Dispatcher.cpp:133`). An operator who enables
AGC resets on such a board loses boosted gain permanently, without ever having
chosen to use it.

**Why the earlier sweep measured nothing is not established.** The explanation
given at the time - that boosted gain was never on - is wrong. Candidates that
changed between then and now, none of them confirmed: the sweep had been sending
unscoped, adverts were not sent before the flood, and the native binaries were
rebuilt. It is left open rather than replaced with a second guess.

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
