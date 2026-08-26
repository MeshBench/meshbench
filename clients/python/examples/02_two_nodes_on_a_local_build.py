#!/usr/bin/env python3
"""Example 2 from #209: a fixture trimmed to two, both on a build from a
MeshCore checkout - and re-runnable without clearing anything down.

    ./02_two_nodes_on_a_local_build.py ~/src/MeshCore

The interesting half is the second run. It attaches to the workbench the first
one left, stops the clock, rebuilds, repoints the nodes and starts again -
rather than opening a fresh session and paying for the fixture twice.

Costs: minutes, mostly firmware. Real firmware on two nodes, not fifty-eight,
because trimming is what this example is about.
"""

import os
import subprocess
import sys
from datetime import timedelta

from meshbench import BINARY_ENV, Build, Kind, Role, Workbench

# Outskirts of Glasgow, and Glenrothes.
KEEP = {
    "Glasgow-Outskirts": (55.8720, -4.3300),
    "Glenrothes": (56.1980, -3.1780),
}


def build_from_checkout(checkout: str, wb: Workbench) -> dict[str, Build]:
    """Build MeshCore and import what came out, one build per role.

    Both roles from one invocation, deliberately. A locally built repeater
    compiled against a stale shim once answered console output with 0x06 where
    the host expects 0x07: it connected, misbehaved and exited. Two arms of a
    comparison speaking different wire protocols measure the shim, not the
    firmware - so if either arm is built by hand, both are, the same way, at
    the same moment.
    """
    # The same binary the client is driving, not whatever is on PATH: a
    # checkout usually has one built and not installed, and building with one
    # while talking to another is how two arms end up on different code.
    exe = os.environ.get(BINARY_ENV) or "meshbench"

    out: dict[Role, Build] = {}
    for role in (Role.SIMPLE_REPEATER, Role.COMPANION_RADIO):
        # Named here rather than left to default to the git branch, because
        # the build has to be found again afterwards and a name you chose is
        # the only one you can look up.
        name = f"local-{role}"
        made = subprocess.run(
            [exe, "dev", "-from", checkout, "-role", role, "-name", name],
            capture_output=True,
            text=True,
            check=False,
        )
        if made.returncode != 0:
            sys.exit(
                f"building the {role}: {made.stderr.strip() or made.stdout.strip()}"
            )
        # meshbench dev puts the build in the cache; the library sees it.
        wb.firmware.scan()
        out[role] = wb.firmware.find(name)
    return out


def main() -> None:
    if len(sys.argv) < 2:
        sys.exit("usage: 02_two_nodes_on_a_local_build.py <path to MeshCore>")
    checkout = sys.argv[1]

    with Workbench.attach_or_launch() as wb:
        # Whether the mesh is already the one this example is about, not
        # whether the session is empty. A launched workbench is never empty:
        # it opens its own default fixture, which is 311 nodes - so "is it
        # empty" was always false, the trim below never ran, and this put a
        # local build on a national network and reported "311 nodes" as
        # though that had been the plan.
        already = {n.name for n in wb.nodes.list()} == set(KEEP)

        # Stop the clock before anything else. A no-op on a fresh session, and
        # the thing that makes the second run safe on a live one.
        wb.sim.pause()

        if not already:
            wb.project.open("fife-strict")
            # Put them where they belong first, then delete the rest. keep is
            # all-or-none by design, so naming a node that is not there yet
            # refuses and removes nothing - and one of these two is never in
            # the fixture, so the trim refused on every run that reached it.
            for name, (lat, lon) in KEEP.items():
                if name in wb.nodes:
                    wb.nodes[name].move(lat, lon)
                else:
                    wb.nodes.place(name, Kind.COMPANION, lat, lon)
            wb.nodes.keep(*KEEP)
            wb.wait_idle(timedelta(minutes=10))

        builds = build_from_checkout(checkout, wb)

        # Repoint every node, applied - which stops, provisions and starts it.
        for node in wb.nodes:
            role = (
                Role.COMPANION_RADIO
                if node.info.kind == Kind.COMPANION
                else Role.SIMPLE_REPEATER
            )
            node.firmware = builds[role]

        wb.sim.start()
        wb.firmware.wait_started(timedelta(minutes=10))
        print(f"{len(wb.nodes)} nodes on a build from {checkout}")
        print(wb.provenance())


if __name__ == "__main__":
    main()
