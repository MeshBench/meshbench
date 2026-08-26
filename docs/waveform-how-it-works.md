# Waveform mode: carrier-sense the firmware decides for itself

In the default "calculated" physics a link either closes or it does not, priced
from path loss and a required SNR. In **waveform mode** the channel instead sums
the transmitters' actual sample-level IQ, and reception is whatever the
demodulator makes of what arrived (see `internal/sim/engine/waveform.go`). That
is slower, and it buys one thing the calculated model cannot give: the firmware
gets to decide, from real channel energy, whether the air is busy.

## Where CSMA becomes observable

MeshCore's Dispatcher listens before it talks: it runs the SX1262's
channel-activity detection and defers while the air looks occupied. The chip's
CAD is a pure virtual in the radio shim, so *something* has to answer it.

- In calculated mode the answer is a model of who is on the air.
- In **waveform mode** the answer is `dsp.CADBusy` run on the same dechirped
  window the demodulator sees — the actual summed IQ
  (`internal/sim/engine/waveformbusy.go`). No rule in the engine says "these two
  overlap, so defer": the busy vector is a measurement of the air, and the
  firmware's carrier-sense is downstream of it, exactly as on hardware.

`TestWaveformCADTracksTheAir` holds that busy vector to the air at the sample
level. What follows is the other half — real MeshCore firmware carrier-sensing
it — which closes the MS4 acceptance from W3.

## The record: firmware in the loop

Two native `simple_repeater` builds (repeater-v1.17.1) about 3 km apart in
waveform mode, the listener well inside the talker's reach. The talker adverts
in a burst, then the air is left quiet. The listener's own driver counters — how
many times it read the interrupt register (`irqReads`), how many of those found
a detection flag set (`busyReads`), and for how long (`busyMs`) — are its account
of how busy the air looked. Reproduce with:

```
MESHBENCH_LIVE=1 go test ./internal/sim/boardcheck/ \
    -run TestWaveformCADIsWhatTheFirmwareCarrierSenses -v
```

Observed (seed 4417):

| moment                | irqReads | busyReads | busyMs |
|-----------------------|---------:|----------:|-------:|
| before any traffic    |      320 |         0 |      0 |
| after a burst of adverts | 3274 |      2568 |   3698 |
| after the air went quiet | 3467 |      2568 |   3698 |

Two things to read out of it:

- **`busyReads` climbed 0 → 2568 while the talker was on the air.** The firmware
  found the channel occupied — not because the engine told it a packet was
  present, but because CAD on the summed waveform said so.
- **Across the quiet that followed, `busyReads` did not move**, even though
  `irqReads` kept climbing (3274 → 3467): the driver went on polling and found
  nothing busy. The busy count tracks whether the air is *actually* occupied,
  and stops the moment it clears.

Because the busy answer is measured from channel energy rather than mediated by a
rule, it moves with the RF conditions on its own: a weaker or more distant talker
puts less energy in the listener's window, so fewer of its reads find the air
busy and it defers less — the same knobs (`set tx`, separation, the environment)
that change what decodes change what the firmware carrier-senses, with nothing in
the engine coupling the two. That is the whole point of paying for the waveform.
