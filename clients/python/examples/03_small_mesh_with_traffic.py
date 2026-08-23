#!/usr/bin/env python3
"""Example 3 from #209: two repeaters, two companions, one of them a T-Deck,
and a message to the public channel every twenty seconds.

    ./03_small_mesh_with_traffic.py

Needs a display: it opens the workbench so you can watch the traffic move.

Costs: about ten minutes of simulated time, and a few of yours.

The repeating traffic needed no new verb: schedule.add has taken every_ms all
along and nothing said so, which to somebody writing a script is the same as
it not existing.
"""

from datetime import timedelta

from meshbench import Board, Class, Kind, Workbench

MESH = [
    {"name": "R1", "kind": Kind.SIMPLE_REPEATER, "lat": 56.20, "lon": -3.20},
    {"name": "R2", "kind": Kind.SIMPLE_REPEATER, "lat": 56.12, "lon": -3.02},
    {"name": "C1", "kind": Kind.COMPANION, "lat": 56.19, "lon": -3.17},
    {"name": "C2", "kind": Kind.COMPANION, "lat": 56.09, "lon": -3.10},
]


def main() -> None:
    with Workbench.launch() as wb:
        wb.project.new(place="Fife")
        wb.nodes.place_many(MESH)
        wb.wait_idle(timedelta(minutes=10))

        # C1 is the T-Deck. The board goes on before the firmware is pinned,
        # because a host image is not a board image and setting the board
        # clears a pin that was made for different hardware.
        wb.nodes["C1"].board = Board.LILYGO_TDECK

        # Whatever this machine holds for each role that needs one, rather
        # than a version typed here that goes stale.
        wb.firmware.use_what_is_here()

        # Every twenty seconds, from the plain companion to the public channel.
        # Simulated seconds - the mesh's own clock, not yours.
        wb.schedule.add(
            "C2",
            "send hello",
            at=timedelta(seconds=5),
            every=timedelta(seconds=20),
        )

        wb.sim.start()
        wb.firmware.wait_started(timedelta(minutes=10))
        wb.sim.run(timedelta(minutes=10), wait=timedelta(minutes=60))

        received = [e for e in wb.events.recent(1000) if e.class_ == Class.RECEIVED]
        print(wb.provenance())
        print(f"{wb.events.total()} events, {len(received)} receptions in the tail")


if __name__ == "__main__":
    main()
