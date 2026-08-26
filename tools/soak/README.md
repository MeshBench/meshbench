# Soak

Drives a running workbench through sustained companion traffic and judges the
receptions it produces against the real ScotMesh network.

This exists because green tests twice let a broken demodulator lock through.
The lock's failure mode is not a crash or a wrong number - it is a mesh that
quietly delivers a fraction of what it should, which no unit test noticed and
an operator spotted in seconds because a companion stopped hearing its own
messages. A soak is what catches that class of thing: run the mesh, count what
arrives, compare the shape against reality.

    pip install -e pkg/client-python  # the client it drives the workbench through
    ./soak.py 18 /tmp/soakdump     # 18 floods, one at a time, dumped per round
    ./check.py /tmp/soakdump       # verdicts against the real distribution

`soak.py` sends one message at a time and lets each flood finish. Driving
several companions at once produces a storm no mesh experiences, and reading
only `events.recent` samples the most congested moment of it - both were
first attempts here, and both flattered or maligned the model for reasons
that had nothing to do with the model.

What `check.py` asserts, and why each one is worth asserting:

- **one demodulator** - no receiver decodes two packets in the same instant.
  This is the invariant the lock exists to keep, and the one that broke.
- **reporting ceiling** - nothing above the +15 dB an SX126x can express.
- **the mesh is alive** - a broken lock shows up as too few receptions, not
  as an error.
- **median plausible** - against the real network's +5.0 dB.
- **floods thin out** - receptions per transmission stays bounded.

The real figures it compares against are 1,992 receptions carrying SNR from
the live ScotMesh CoreScope: median +5.0 dB, 90th percentile +13.0 dB, a hard
wall at +15.0, and a whole-network relay rate of 0.76/s.

A soak is busier per node than a real network by design - it sends a message
every twenty seconds into a mesh of fifty-eight - so its distribution sits
harder against the ceiling than reality's. The pair-by-pair fidelity test is
`validate.fetch`, which compares predictions against real observations one
link at a time.
