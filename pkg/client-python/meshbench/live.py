"""Bringing a real deployment in from a live feed.

Four steps in a fixed order, and every one of them has been skipped by somebody
at least once. The two that get missed are the last two, and missing them does
not fail: the mesh comes up with regions inferred but never applied, which
transmits everything, relays nothing, and reports no error at all. It reads as
bad RF.

So the steps are here individually, because sometimes you want to look at a
preview before committing - and ``pull`` runs all four, because the ordinary
case is wanting the whole deployment and the ordinary mistake is stopping
early.
"""

from __future__ import annotations

from datetime import timedelta
from typing import TYPE_CHECKING

from .sets import Strategy
from .types import ImportPreview
from .wait import JOB_WAIT

if TYPE_CHECKING:  # pragma: no cover - import for typing only
    from .workbench import Workbench

#: How far back to read traffic when working out what each node holds. A week,
#: because that is what it takes for the quiet regions to say anything at all:
#: on ScotMesh a small region is about sixty packets in seven days, and a
#: shorter window drops it entirely rather than reporting it as thin.
DEFAULT_WINDOW = timedelta(days=7)


class Live:
    """A live feed, and the deployment it describes. Live in both senses."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    # ---- the whole thing -------------------------------------------------

    def pull(
        self,
        url: str,
        strategy: Strategy = Strategy.REPLACE,
        window: timedelta = DEFAULT_WINDOW,
        wait: timedelta = JOB_WAIT,
    ) -> ImportPreview:
        """Fetch, commit, read the traffic, and apply what it implies.

        The whole chain, in the order that works. ``window`` is how far back
        into the feed's history to read - the mesh's own past, not your
        patience; ``wait`` is yours.

        Returns what the fetch found. Link measurement is still running when
        this returns on anything but a small mesh, so follow it with
        ``wb.wait_idle()`` before starting a run.
        """
        preview = self.fetch(url)
        if preview.nodes == 0:
            raise ValueError(f"{url} described {preview.records} nodes, none usable")
        self.commit(strategy)
        self.infer(window, wait=wait)
        self.apply_regions()
        return preview

    # ---- or one step at a time -------------------------------------------

    def set_source(self, url: str) -> str:
        """Point at a feed without reading it, and say how it was tidied.

        A method rather than a property, because a property implies something
        to read back and the session offers no way to ask what its source
        currently is. One that answered from a value this object happened to
        remember would be right until anything else set it.
        """
        return (self._wb.call("import.set_source", {"url": url}) or {}).get("url", url)

    def fetch(self, url: str = "") -> ImportPreview:
        """Read the deployment and say what would change, changing nothing."""
        if url:
            self.set_source(url)
        return ImportPreview.parse(self._wb.call("import.fetch") or {})

    def commit(self, strategy: Strategy = Strategy.REPLACE) -> int:
        """Apply the fetched nodes to the scenario.

        ``"replace-all"`` is what the shipped fixtures were built with;
        ``"add"`` keeps what is already here and skips names that clash.

        Measuring the links afterwards is a job rather than part of this call -
        676 nodes is 228,000 terrain paths over real ground - so this returns
        while that is still running.
        """
        got = self._wb.call("import.commit", {"strategy": strategy}) or {}
        return got.get("nodes", 0)

    def infer(
        self, window: timedelta = DEFAULT_WINDOW, wait: timedelta = JOB_WAIT
    ) -> None:
        """Read the feed's recent traffic to work out what each node holds.

        This is the step that decides whether anything relays. A node whose
        regions are unknown forwards nothing, and nothing says so.

        ``window`` is the feed's own past; ``wait`` is how long you will sit
        here for it. A week of ScotMesh is around 150,000 packets and several
        minutes of paging.
        """
        hours = window.total_seconds() / 3600.0
        if hours <= 0:
            raise ValueError("infer() needs a window, e.g. timedelta(days=7)")
        self._wb.call("infer.run", {"hours": hours})
        self._wb.job("infer").wait(wait)

    def apply_regions(self) -> int:
        """Put the inferred regions onto the nodes, and say how many took one.

        The forgotten step. Everything above can succeed and the mesh still be
        silent until this runs.
        """
        return (self._wb.call("infer.apply") or {}).get("applied", 0)
