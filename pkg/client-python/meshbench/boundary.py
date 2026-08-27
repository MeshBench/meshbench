"""The study area: which nodes are in the question being asked.

Not the firmware's region concept. A boundary decides what is *studied*; a
region decides what is *forwarded*. Both words are in this application and
confusing them is how somebody concludes the RF model is broken.

Set it before importing. The import filters at fetch time, so a boundary set
afterwards prunes what has already been paid for rather than never fetching it.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import TYPE_CHECKING

from . import errors

if TYPE_CHECKING:  # pragma: no cover - import for typing only
    from .workbench import Workbench


class Boundary:
    """The study area, however you have it. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def use(self, area: str | Path, name: str = "") -> list[str]:
        """Take a study area from a place name or from GeoJSON.

        The one to call. A path to a .geojson file is loaded; anything else is
        searched for by name and the best match accepted. Both end with the
        area in the study, which is the only thing the caller wanted to say.

        ``name`` renames a single loaded polygon, so a file called
        ``export(3).geojson`` can still join the study as "Tay catchment".
        """
        if _is_a_file(area):
            return self.load(area, name=name)
        return [self.accept(self.search(str(area))[0])]

    def search(self, query: str) -> list[str]:
        """Places matching a name, best first. Needs the network.

        Returns names rather than geometry: the geometry stays at the
        workbench, and the name is what accept takes.
        """
        got = self._wb.call("boundary.set", {"query": query}) or {}
        found = got.get("names") or []
        if not found:
            raise errors.NotFound(
                "boundary.set", f"nothing is called {query!r}", "not_found"
            )
        return found

    def accept(self, name: str) -> str:
        """Take one of the search results into the study area.

        Areas union rather than replace: a study is often two council areas
        rather than one.
        """
        got = self._wb.call("boundary.accept", {"name": name}) or {}
        return got.get("accepted", name)

    def load(self, source: str | Path | dict, name: str = "") -> list[str]:
        """Take a study area from GeoJSON: a path, a document, or a dict.

        A Polygon, a MultiPolygon, a Feature or a FeatureCollection. Each
        polygon becomes an area named from its ``name`` property, or from
        ``name``, or from the file.

        The one way to study an area nothing has an administrative name for -
        a catchment, a valley, the bit north of the river - and the only one
        that works with no network at all.
        """
        params: dict = {}
        if isinstance(source, dict):
            params["geojson"] = json.dumps(source)
        elif _is_a_file(source):
            params["path"] = str(source)
        else:
            params["geojson"] = str(source)
        if name:
            params["name"] = name
        got = self._wb.call("boundary.load", params) or {}
        return got.get("loaded") or []

    def list(self) -> list[str]:
        """What the study area is made of."""
        return (self._wb.call("boundary.list") or {}).get("names") or []

    def remove(self, name: str) -> None:
        """Take one area back out.

        Changes what is measured, never what is loaded: the nodes stay until
        something prunes them.
        """
        self._wb.call("boundary.remove", {"name": name})

    def prune(self, margin_km: float | None = None) -> int:
        """Delete the nodes outside the study area, and say how many went.

        For a mesh that was imported before the boundary was set. The margin is
        kept on purpose: a node just outside still interferes with one just
        inside, and dropping it makes the inside look quieter than it is.
        """
        params = {} if margin_km is None else {"margin_km": margin_km}
        return (self._wb.call("boundary.prune", params) or {}).get("removed", 0)


def _is_a_file(x: object) -> bool:
    """A path, rather than a place name or a GeoJSON document.

    Judged by extension as well as by existence, so a mistyped path is reported
    as a missing file rather than searched for as a place - which answers
    "nothing is called ./bounds/fife.geojson" and sends the reader looking in
    entirely the wrong direction.
    """
    if isinstance(x, Path):
        return True
    if not isinstance(x, str) or x.lstrip().startswith("{"):
        return False
    return x.endswith((".geojson", ".json")) or Path(x).is_file()
