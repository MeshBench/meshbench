#!/usr/bin/env python3
"""Example 4: the one CI runs.

    ./04_headless_regression.py [fixture] [junit.xml]

No display, no GPU, no toolkit. Opens a fixture, runs it, checks its
assertions, writes JUnit, and exits non-zero if the mesh stopped delivering.
This is the shape a MeshCore pull request would use.

Costs: as long as the fixture asks for. fife-strict at five simulated minutes
is a couple of minutes of wall clock with no firmware, and considerably more
with it.
"""

import sys
from datetime import timedelta

from meshbench import Workbench

SEED = 9001


def main() -> int:
    fixture = sys.argv[1] if len(sys.argv) > 1 else "fife-strict"
    junit = sys.argv[2] if len(sys.argv) > 2 else ""

    with Workbench.headless(fixture=fixture, seed=SEED) as wb:
        wb.sim.run(timedelta(minutes=5), wait=timedelta(minutes=60))

        report = wb.assertions.check()

        # The report prints the caveats above the numbers itself, because this
        # is the output somebody pastes into a pull request and the caveats are
        # the half that gets dropped.
        print(report)
        print(f"{wb.events.total()} events")

        if junit:
            report.write_junit(junit)

        if report.total == 0:
            # Not a pass. A fixture with no assertions can report but cannot
            # pass or fail, and a green tick that checked nothing is the worst
            # outcome available here.
            return 2
        return 0 if report.ok else 1


if __name__ == "__main__":
    sys.exit(main())
