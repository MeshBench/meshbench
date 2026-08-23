#!/usr/bin/env python3
"""Build a board image from its own repository and put the new one on a node.

    WADAMESH=~/src/wadamesh ./07_replace_a_board_build.py

Run it again after a change and it reuses the session already open: pause, swap
the firmware, delete the build it replaced, carry on. Needs a display, and
leaves the window up on the node's Hardware tab so you can watch it boot.
"""

import os
import subprocess
from datetime import timedelta
from pathlib import Path

from meshbench import Board, Kind, Role, Tab, Workbench

WADAMESH = Path(os.environ.get("WADAMESH", "~/src/wadamesh")).expanduser()
#: The PlatformIO environment to build. Every one wadamesh defines ends in
#: _touch; override it for a different board.
PIO_ENV = os.environ.get("WADAMESH_ENV", "LilyGo_TDeck_companion_radio_touch")
IMAGE = WADAMESH / ".pio" / "build" / PIO_ENV / "firmware.bin"

BOARD, ROLE, NODE = Board.LILYGO_TDECK, Role.COMPANION_RADIO, "Bench"


def main() -> None:
    subprocess.run(["pio", "run", "-e", PIO_ENV], cwd=WADAMESH, check=True)

    # Not a context manager: closing owns the process it started, and the point
    # here is to leave the window up.
    wb = Workbench.attach_or_launch()

    # Pause first. Swapping firmware under a running clock stops the node while
    # its neighbours carry on transmitting, and nothing accounts for the gap.
    wb.sim.pause()

    if NODE not in wb.nodes:
        wb.project.new(place="Fife")
        wb.nodes.place(NODE, kind=Kind.COMPANION, lat=56.20, lon=-3.20, board=BOARD)

    node = wb.nodes[NODE]
    old = node.build

    # No label, so it is stamped with the time: two runs are two builds rather
    # than one quietly overwriting the other.
    new = wb.firmware.import_(str(IMAGE), ROLE, board=BOARD)
    node.firmware = new  # stops the node, provisions it again, starts it
    node.wait_running()

    # Only now it is on the new one. A pin nothing can honour does not fail
    # until the node next starts.
    if old and old != new:
        wb.firmware.delete(old)

    wb.window(NODE, Tab.HARDWARE)
    wb.sim.play()
    wb.sim.run(timedelta(seconds=30))
    print(f"{node} is running {node.firmware}")


if __name__ == "__main__":
    main()
