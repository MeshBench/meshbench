#!/usr/bin/env python3
"""Import a real mesh from its live feed, find one node, and make it advert.

    ./06_live_import_and_advert.py ["node name"]

Needs the network. The names on a real mesh carry emoji - "West Lomond" is
really "🏔️ West Lomond 📡" - so the node is searched for rather than typed.
"""

import sys
from datetime import timedelta

from meshbench import Class, Workbench

FEED = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"

# ScotMesh is around 676 nodes and firmware on all of them is hours, so this
# keeps the one we want and its dozen nearest.
NEIGHBOURS = 12


def main() -> None:
    want = sys.argv[1] if len(sys.argv) > 1 else "West Lomond"

    with Workbench.headless() as wb:
        # Fetch the nodes, commit them, read a week of traffic, and apply the
        # regions it implies. Skip that last step and nothing ever relays.
        print(wb.live.pull(FEED))
        wb.wait_idle()

        node = wb.nodes.find(want)
        print(f"{want} is {node}")

        wb.nodes.keep(node, *wb.nodes.near(node, NEIGHBOURS))
        wb.wait_idle()

        wb.firmware.use_what_is_here()
        wb.sim.start()
        wb.firmware.wait_started()

        # ask() gives the mesh time to answer. read() straight after send()
        # reads the moment before the command went out.
        print(node.console.ask("advert"))
        wb.sim.run(timedelta(minutes=2))

        heard = {
            e.to
            for e in wb.events.recent(2000)
            if e.class_ == Class.RECEIVED and e.from_ == node.name
        }
        print(f"{len(heard)} of {len(wb.nodes) - 1} neighbours heard it directly")
        print(wb.provenance())


if __name__ == "__main__":
    main()
