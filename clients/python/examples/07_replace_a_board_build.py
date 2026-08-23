#!/usr/bin/env python3
"""Example 7: build a board image somewhere else, and put the new one on.

    WADAMESH=~/src/wadamesh ./07_replace_a_board_build.py

Point ``WADAMESH`` at the repository, run it, change something, run it again.
The second run does not start over: it finds the session still up, pauses it,
imports the new image beside the old, moves the node onto it, deletes the one
it replaced and carries on from where the clock was. Then it puts the node's
Hardware tab on screen, which is where you watch the thing you just built
actually boot.

Idempotent because a script you run twenty times in an afternoon is a script
that must not clear everything down each time. Repositioning the node, waiting
out the link warm and re-importing the terrain on every edit is minutes each
time, and it is the reason people stop using a tool like this.

The three things it gets right that are easy to get wrong:

**Every import is labelled with when it happened.** Without a label they were
all called "imported", each one overwrote the last, and there was no way to say
which of two builds a node was running - or to delete the older one, since both
were the same file.

**The old build goes only after the node is on the new one.** Deleting a build
a node is still pinned to leaves the pin in place, and a pin nothing can honour
does not fail until the node next starts.

**Replacing firmware restarts the companion.** ``set_firmware`` applies by
default: stop, provision, start. Firmware is chosen when a node launches, so
recording it and leaving the node on the old image is the control you press
twice and then stop trusting.

Costs: whatever your own build takes, plus a boot. The board is emulated one at
a time on purpose - a full fixture of them will take a twelve-core machine
down - so this example is one node and says so.
"""

import os
import subprocess
import sys
from datetime import datetime, timedelta
from pathlib import Path

from meshbench import Board, Kind, Workbench

#: The repository to build, and where its output lands. Override the directory
#: with WADAMESH; the rest follows PlatformIO's own layout.
WADAMESH = Path(os.environ.get("WADAMESH", "~/src/wadamesh")).expanduser()
PIO_ENV = os.environ.get("WADAMESH_ENV", "LilyGo_TDeck_companion_radio")
BOARD = Board.LILYGO_TDECK
ROLE = "companion_radio"

#: Its own address, so this example and whatever else you have open do not
#: fight over one session.
SOCKET = "/tmp/meshbench-wadamesh.sock"
NODE = "Bench"


def build() -> Path:
    """Build the repository and hand back the image it produced.

    The build is the repository's business, not ours - it owns the toolchain,
    the board definitions and the flags. All this needs is the artefact, and
    if that has moved, this is the one function to change.
    """
    out = WADAMESH / ".pio" / "build" / PIO_ENV / "firmware.bin"
    if not WADAMESH.is_dir():
        raise SystemExit(f"{WADAMESH} is not there; set WADAMESH to the repository")

    print(f"building {PIO_ENV} in {WADAMESH}")
    r = subprocess.run(["pio", "run", "-e", PIO_ENV], cwd=WADAMESH, check=False)
    if r.returncode != 0:
        raise SystemExit(f"the build failed ({r.returncode}); nothing was changed")
    if not out.is_file():
        raise SystemExit(
            f"the build reported success but {out} is not there; "
            f"set WADAMESH_ENV if this repository names its environment differently"
        )
    return out


def main() -> int:
    image = build()

    wb = Workbench.attach_or_launch(socket=SOCKET)
    with wb:
        attached = not wb.owns_process
        print("attached to the running session" if attached else "started a session")

        # Pause before touching anything. Swapping firmware under a running
        # clock means the node is stopped and restarted while its neighbours
        # carry on transmitting, and what it misses in that window is a gap
        # nothing accounts for later.
        was_playing = wb.sim.playing
        wb.sim.pause()

        if NODE not in wb.nodes:
            wb.project.new(place="Fife")
            wb.nodes.place(NODE, kind=Kind.COMPANION, lat=56.20, lon=-3.20, board=BOARD)
            wb.wait_idle()

        node = wb.nodes[NODE]

        # On screen before the swap rather than after it, so what you watch is
        # the board going down and coming back up on the image you just built.
        # The tab that shows it as itself: its screen, its buttons, and what
        # its radio is really doing.
        wb.window(NODE, "Hardware")

        # What it is on now, before anything replaces it. Read first: after the
        # import the library has two and telling them apart is guesswork.
        before = [
            b
            for b in wb.firmware.on_disk()
            if b.board == BOARD and b.version == node.info.firmware
        ]

        # Stamped with the minute, so a second run is a second build in the
        # library rather than a silent overwrite of the first.
        label = "wadamesh-" + datetime.now().strftime("%Y%m%d-%H%M%S")
        built = wb.firmware.import_(str(image), ROLE, board=BOARD, label=label)
        print(f"imported {built} ({built.bytes:,} bytes)")

        # Applied, which stops the node, provisions it again and starts it -
        # the companion comes back on the new image.
        node.set_firmware(built)
        node.wait_running()
        print(f"{NODE} is running {node.info.firmware}")

        # Only now, and only what this run replaced. A build somebody else is
        # using keeps its own pin and is none of this script's business.
        for old in before:
            if old.version != built.version and old.in_use == 0:
                print(f"deleted {old} at {wb.firmware.delete(old)}")

        # Carry on from where the clock was rather than from zero: playing
        # again only if it was playing, or if this run is the one that built
        # the session in the first place.
        if was_playing or not attached:
            wb.sim.play()

        wb.sim.run(timedelta(seconds=30))
        print(f"advert: {node.console.ask('advert')!r}")
        print(wb.provenance())

        if not wb.owns_process:
            print(f"the session stays up at {SOCKET}; run this again to replace it")
    return 0


if __name__ == "__main__":
    sys.exit(main())
