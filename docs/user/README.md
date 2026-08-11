# Using MeshBench

MeshBench runs **real MeshCore firmware** against a sample-accurate LoRa
channel. When a repeater relays something here, it is because MeshCore decided
to, on an SNR that came out of a demodulator — not because a packet model said
so.

That is the whole point, and it is also the thing to keep in mind: everything
below is about arranging for the firmware to make a decision, and then reading
what it did.

## The guides

| | |
|---|---|
| [Building a network](building-a-network.md) | Import a real mesh, give it a boundary, work out its transport regions, run firmware on it |
| [Running an experiment](running-an-experiment.md) | Compare configurations properly: arms, repeats, and why the bench argues with you |
| [Watching the traffic](watching-the-traffic.md) | Wireshark, live, with every receiver's view of every frame |
| [Talking to a companion](talking-to-a-companion.md) | Send a real message from a real companion build |

## The four views

**Plan** — build and site. Import, place, drag, boundary, coverage.
**Run** — exercise and watch. Play, schedule traffic, consoles, live feed.
**Debug** — why did that happen. Packets, waterfall, link budgets.
**Verify** — is it still true. Baselines, A/B, residuals against reality.
**Bench** — compare configurations. Sweep a parameter, repeat it, read what
differed.

## Two things worth knowing before anything else

**The simulator is kinder than the air.** No multipath, no body loss, no
oscillator error. Every result is a *best case*, which is what makes it usable:
if it does not work here, it will not work outdoors. The status bar says so
permanently, on purpose.

**Nothing is saved unless you save it.** The scenario lives in the running
process. `File → Save project` before anything that might restart it — and a
restart loses the network, the inferred regions and the firmware assignments
together.
