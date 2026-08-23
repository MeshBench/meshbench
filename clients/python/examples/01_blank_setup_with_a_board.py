#!/usr/bin/env python3
"""Example 1 from #209: a blank setup, one companion, and its screen on show.

    ./01_blank_setup_with_a_board.py

Costs: a minute or two, plus whatever downloading a build takes the first time.
Needs a display. It opens the node's own window on the Hardware tab at the
end, which is the point of it.

"""

import sys
from datetime import timedelta

from meshbench import Board, Kind, NotFound, Workbench

WADAMESH = "wadamesh"
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
            wb.firmware.download("companion", WADAMESH, board=BOARD)
            wb.wait_idle(timedelta(minutes=10))
            build = wb.firmware.find(WADAMESH, board=BOARD)

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
