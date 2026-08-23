#!/usr/bin/env python3
"""Import a real mesh from its live feed, find one node, and make it advert.

    ./06_live_import_and_advert.py [area] [node]

    ./06_live_import_and_advert.py Fife
    ./06_live_import_and_advert.py bounds/tay-catchment.geojson

The area is a place name or a path to GeoJSON, and it is set before the import
because the import filters at fetch time - the whole feed is around 676 nodes
and this is how you study a corner of it.

The node is searched for rather than typed: the names on a real mesh carry
emoji, so "West Lomond" is really "🏔️ West Lomond 📡".

Needs the network.
"""

import sys
from datetime import timedelta

from meshbench import Class, Workbench

FEED = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"


def main() -> None:
    area = sys.argv[1] if len(sys.argv) > 1 else "Fife"
    wanted = sys.argv[2] if len(sys.argv) > 2 else "West Lomond"

    with Workbench.headless() as wb:
        print("studying", ", ".join(wb.boundary.use(area)))

        # Fetch the nodes, commit them, read a week of traffic, and apply the
        # regions it implies. Skip that last step and nothing ever relays.
        print(wb.live.pull(FEED))
        wb.wait_idle()

        node = wb.nodes.find(wanted)
        print(f"{wanted} is {node}, one of {len(wb.nodes)}")

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
        print(f"{len(heard)} of {len(wb.nodes) - 1} others heard it directly")
        print(wb.provenance())


if __name__ == "__main__":
    main()
