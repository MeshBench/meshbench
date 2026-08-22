#!/usr/bin/env python3
"""Example 3 from #209: two repeaters, two companions, and a message to the
public channel every twenty seconds.

    ./03_small_mesh_with_traffic.py

Costs: about ten minutes of simulated time, and a few of yours.

The repeating traffic needed no new verb: schedule.add has taken every_ms all
along and nothing said so, which to somebody writing a script is the same as
it not existing.
"""

import meshbench
from meshbench import Workbench

MESH = [
    {"name": "R1", "kind": meshbench.SIMPLE_REPEATER, "lat": 56.20, "lon": -3.20},
    {"name": "R2", "kind": meshbench.SIMPLE_REPEATER, "lat": 56.12, "lon": -3.02},
    {"name": "C1", "kind": meshbench.COMPANION, "lat": 56.19, "lon": -3.17},
    {"name": "C2", "kind": meshbench.COMPANION, "lat": 56.09, "lon": -3.10},
]


def main() -> None:
    with Workbench.headless() as wb:
        wb.project.new(place="Fife")
        wb.nodes.place_many(MESH)
        wb.wait_idle("10m")

        # Whatever this machine holds for each role, rather than a version
        # typed here that goes stale.
        for role in ("repeater", "companion"):
            builds = [
                b for b in wb.firmware.on_disk() if b.role == role and not b.board
            ]
            if not builds:
                raise SystemExit(
                    f"no {role} build on this machine: "
                    f"meshcoresim firmware download {role}"
                )
            wb.firmware.use_for_role(role, builds[-1])

        # C1 is the T-Deck. The board goes on before the firmware is pinned,
        # because a host image is not a board image and setting the board
        # clears a pin that was made for different hardware.
        wb.nodes["C1"].board = "LilyGo_TDeck"

        # Every twenty seconds, from the plain companion to the public channel.
        # Simulated seconds: this is the mesh's own clock.
        wb.call(
            "schedule.add",
            {"node": "C2", "command": "send hello", "at_ms": 5_000, "every_ms": 20_000},
        )

        wb.sim.start()
        wb.firmware.wait_started("10m")
        wb.sim.run(minutes=10, wait="60m")

        received = [e for e in wb.events.recent(1000) if e.class_ == meshbench.RECEIVED]
        print(wb.provenance())
        print(f"{wb.events.total()} events, {len(received)} receptions in the tail")


if __name__ == "__main__":
    main()
