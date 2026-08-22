#!/usr/bin/env python3
"""Two builds, two nodes, one scenario - the A/B #192 was filed from.

    ./05_two_builds_in_one_scenario.py <stock version> <path to a local build>

The most common real use of this API, and the reason the node window grew a
firmware control: comparing a stock build against one with a single changed
constant, on the same mesh, at the same seed.

Costs: firmware on a whole fixture, so minutes.
"""

import sys
from datetime import timedelta

from meshbench import Workbench

SEED = 9001


def main() -> None:
    if len(sys.argv) < 3:
        sys.exit(
            "usage: 05_two_builds_in_one_scenario.py <stock version> <local build path>"
        )
    stock_version, local_path = sys.argv[1], sys.argv[2]

    with Workbench.headless(fixture="fife-strict", seed=SEED) as wb:
        stock = wb.firmware.find(stock_version)
        changed = wb.firmware.import_(local_path, role="repeater")

        # Two nodes far enough apart to be independently interesting, one on
        # each build. Applied, which restarts each of them.
        a, b = list(wb.nodes)[0], list(wb.nodes)[1]
        a.firmware = stock
        b.firmware = changed

        wb.sim.start()
        wb.firmware.wait_started(timedelta(minutes=15))
        wb.sim.run(timedelta(minutes=5), wait=timedelta(minutes=60))

        # Per node, because the whole point is which of the two behaved
        # differently - a total would hide it.
        print(wb.provenance())
        for node, build in ((a, stock), (b, changed)):
            s = node.stat
            print(f"{node.name:24} {str(build):32} sent {s.sent:4}  heard {s.heard:4}")

        # One run of one seed is one draw. A difference here is a hypothesis,
        # not a result: run it across seeds with experiment.* before believing
        # anything.
        print("\none seed, one draw - vary the seed before calling this a difference")


if __name__ == "__main__":
    main()
