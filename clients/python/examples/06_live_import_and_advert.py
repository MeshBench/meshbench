#!/usr/bin/env python3
"""Example 6: a real deployment, pulled live, and one node made to speak.

    ./06_live_import_and_advert.py ["node name"]

Imports ScotMesh from its CoreScope feed - nodes, then a week of traffic to
work out which regions each one holds - trims it to a workable neighbourhood,
brings the firmware up and sends an advert from West Lomond.

Two things this exists to show.

**The import is four steps and the last two are the ones that get skipped.**
``live.pull`` does all four. Stopping after the commit gives you a mesh with
every node in the right place and no regions on any of them, which transmits,
relays nothing, and reports no error whatsoever. It reads as bad RF and it has
cost people days.

**You cannot type these names.** The real one is "🏔️ West Lomond 📡" with the
emoji varying by who last edited it, so the script asks the workbench to search
and takes the best answer, having first checked that the best answer is
actually good. Taking the top result unconditionally is how you end up
adverting from a node that merely shared a word with what you asked for.

Costs: the feed is real, so this needs the network. Reading a week of ScotMesh
traffic is around 150,000 packets and a few minutes. Firmware on the trimmed
neighbourhood is another few. The simulator is kinder than the air - no
multipath, no body loss, no oscillator error - so read what arrives as a best
case.
"""

import math
import sys
from datetime import timedelta

from meshbench import RECEIVED, Workbench

FEED = "https://scotmesh-corescope.mm7roq.compute.oarc.uk"

#: How many of its nearest neighbours to keep around the node we want. The
#: whole deployment is around 676 nodes; running firmware on all of them is
#: hours and a great deal of memory, and the question here is what one hill can
#: reach. Set it to 0 to keep the lot and mean it.
NEIGHBOURS = 12


def kilometres(a, b) -> float:
    """Great-circle distance, near enough for ranking neighbours.

    The workbench has the accurate one and uses it for every path loss; this is
    only deciding which dozen nodes to keep, where a few metres either way
    changes nothing.
    """
    r = 6371.0
    p1, p2 = math.radians(a.lat), math.radians(b.lat)
    dp, dl = p2 - p1, math.radians(b.lon - a.lon)
    h = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * r * math.asin(math.sqrt(h))


def main() -> int:
    want = sys.argv[1] if len(sys.argv) > 1 else "West Lomond"

    with Workbench.headless() as wb:
        print(f"pulling {FEED}")
        found = wb.live.pull(FEED)
        print(f"  {found}")

        # The commit measures every pair over real terrain and does it as a
        # job, so it is still going when pull returns.
        wb.wait_idle()

        # Search, then look at what came back. find() refuses rather than
        # guessing when the best answer is not convincing, and says what it did
        # find - which is the difference between "you spelt it wrong" and "that
        # node is not on this mesh today".
        for m in wb.nodes.search(want, limit=5):
            print(f"  {m.score:.2f}  {m.name}")
        node = wb.nodes.find(want)
        print(f"{want} is {node.name!r}")

        if NEIGHBOURS:
            here = node.info
            others = sorted(
                (n for n in wb.nodes.list() if n.name != node.name),
                key=lambda n: kilometres(here, n),
            )
            keep = [node.name] + [n.name for n in others[:NEIGHBOURS]]
            wb.nodes.keep(*keep)
            wb.wait_idle()
            furthest = kilometres(here, others[NEIGHBOURS - 1])
            print(f"kept {len(keep)} nodes, out to {furthest:.1f} km")

        # Whatever this machine holds for each role that the trimmed mesh
        # actually needs. Asking the workbench which roles are unanswered
        # beats guessing: an import brings whatever kinds the deployment has,
        # and a run refuses to start until every one of them is pinned.
        for role in {r["role"] for r in wb.firmware.needed()}:
            builds = [
                b for b in wb.firmware.on_disk() if b.role == role and not b.board
            ]
            if not builds:
                raise SystemExit(
                    f"no {role} build on this machine: "
                    f"meshcoresim firmware download {role}"
                )
            wb.firmware.use_for_role(role, builds[-1])

        wb.sim.start()
        wb.firmware.wait_started()

        # ask() rather than send() and then read(): a node reads its serial
        # input on its next loop and its loop only runs when the engine steps,
        # so reading straight after sending reads the moment before the command
        # went out. That mistake looks exactly like a console that does not
        # answer.
        print(f"advert from {node.name!r}: {node.console.ask('advert')!r}")

        wb.sim.run(timedelta(minutes=2))

        heard = {
            e.to
            for e in wb.events.recent(2000)
            if e.class_ == RECEIVED and e.from_ == node.name
        }
        print(wb.provenance())
        print(f"{len(heard)} of {len(wb.nodes) - 1} neighbours heard it directly")
        for name in sorted(heard):
            print(f"  {name}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
