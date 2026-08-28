#!/usr/bin/env python3
"""Example 1 from #209: a blank setup, one companion, and its screen on show.

    ./01_blank_setup_with_a_board.py

Costs: a minute or two. wadamesh is imported, not downloaded, so it must
already be in the library or reachable through WADAMESH_IMAGE (see below).
Needs a display. It opens the node's own window on the Hardware tab at the
end, which is the point of it.

"""

import os
import sys
from datetime import timedelta

from meshbench import Board, Kind, NotFound, Role, Workbench

WADAMESH = "wadamesh"
# wadamesh is imported, not in the download catalogue: a built image to
# import if it is not already in the library. Point this at one.
WADAMESH_IMAGE = os.environ.get("WADAMESH_IMAGE")
BOARD = Board.LILYGO_TDECK


def main() -> None:
    with Workbench.launch() as wb:
        wb.project.new(place="Fife")

        deck = wb.nodes.place(
            "Deck",
            kind=Kind.COMPANION,
            lat=56.19,
            lon=-3.17,
            board=BOARD,
        )

        # Whatever the catalogue has, so this does not go stale against a
        # version number typed here.
        wb.firmware.scan()
        try:
            build = wb.firmware.find(WADAMESH, board=BOARD)
        except NotFound:
            # wadamesh is imported, not downloaded - import a built image.
            if not WADAMESH_IMAGE:
                sys.exit(
                    f"{WADAMESH} is not in the library; set WADAMESH_IMAGE "
                    "to a built image, or import one in the workbench first"
                )
            build = wb.firmware.import_(
                WADAMESH_IMAGE,
                Role.COMPANION_RADIO_USB,
                board=BOARD,
                label=WADAMESH,
            )

        # Applied: stop, provision, start. On a board that means an emulator,
        # which is why the wait below is generous.
        deck.firmware = build

        wb.sim.start()
        deck.wait_running(timedelta(minutes=5))

        # The Hardware tab is where the board draws its own screen, which is
        # the whole reason for making this node a T-Deck.
        tab = wb.window(deck, tab="Hardware")

        print(f"{deck.name} is up on {build}; its window is open on {tab}")
        print(wb.provenance())
        # Held open for somebody looking at it, and only then. Piped or run
        # from CI there is nobody to press enter, and input() raises EOFError
        # there - so an example that had done everything right ended in a
        # traceback and a non-zero-looking failure.
        if sys.stdin.isatty():
            input("press enter to close the workbench ")


if __name__ == "__main__":
    main()
