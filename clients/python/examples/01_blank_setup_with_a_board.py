#!/usr/bin/env python3
"""Example 1 from #209: a blank setup, one companion, and its screen on show.

    ./01_blank_setup_with_a_board.py

Costs: a minute or two, plus whatever downloading a build takes the first time.
Needs a window - it opens the node's own window at the end, which is the point
of it. Run the headless examples instead if you have no display.

Two lines of this are marked NOT YET: giving a node a board at placement, and
opening a node window on a chosen tab, are both missing verbs (#216). They are
left in, commented, rather than worked around, so this file stops being a lie
the day that lands.
"""

import meshbench
from meshbench import Workbench

WADAMESH = "wadamesh"
BOARD = "LilyGo_TDeck"


def main() -> None:
    with Workbench.launch() as wb:
        wb.project.new(place="Fife")

        deck = wb.nodes.place(
            "Deck",
            kind=meshbench.COMPANION,
            lat=56.19,
            lon=-3.17,
            # board=BOARD,  # NOT YET: nodes.place takes no board (#216)
        )

        # Whatever the catalogue has, so this does not go stale against a
        # version number typed here.
        wb.firmware.scan()
        try:
            build = wb.firmware.find(WADAMESH, board=BOARD)
        except meshbench.NotFound:
            wb.firmware.download("companion", WADAMESH, board=BOARD)
            wb.wait_idle("10m")
            build = wb.firmware.find(WADAMESH, board=BOARD)

        # Applied: stop, provision, start. On a board that means an emulator,
        # which is why the wait below is generous.
        deck.firmware = build

        wb.sim.start()
        deck.wait_running("5m")

        # wb.call("node.window", {"node": deck.name, "tab": "Hardware"})
        #   NOT YET: node.window takes no tab (#216)
        wb.call("node.window", deck.name)

        print(f"{deck.name} is up on {build}; its window is open")
        print(wb.provenance())
        input("press enter to close the workbench ")


if __name__ == "__main__":
    main()
