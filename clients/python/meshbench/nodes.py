"""The network: what is in it, and what one node can be asked to do."""

from __future__ import annotations

from collections.abc import Iterator
from typing import TYPE_CHECKING

from . import errors
from .types import SIMPLE_REPEATER, Build, NodeInfo, NodeStat
from .wait import seconds, wait_for

if TYPE_CHECKING:  # pragma: no cover - import for typing only
    from .parts import Console
    from .workbench import Workbench


class Nodes:
    """The collection. Live: every call reads the session."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def list(self) -> list[NodeInfo]:
        got = self._wb.call("nodes.list") or {}
        return [NodeInfo.parse(n) for n in got.get("nodes") or []]

    def __iter__(self) -> Iterator[Node]:
        return (Node(self._wb, n.name) for n in self.list())

    def __len__(self) -> int:
        return len(self.list())

    def __getitem__(self, name: str) -> Node:
        self.info(name)  # so a typo fails here rather than three calls later
        return Node(self._wb, name)

    def __contains__(self, name: str) -> bool:
        return any(n.name == name for n in self.list())

    def info(self, name: str) -> NodeInfo:
        for n in self.list():
            if n.name == name:
                return n
        raise errors.NotFound("nodes.list", f"no node named {name!r}", "not_found")

    def of_kind(self, kind: str) -> list[Node]:
        return [Node(self._wb, n.name) for n in self.list() if n.kind == kind]

    def place(
        self,
        name: str,
        kind: str = SIMPLE_REPEATER,
        lat: float = 0.0,
        lon: float = 0.0,
        height_m: float | None = None,
        tx_dbm: float | None = None,
        board: str | None = None,
    ) -> Node:
        """Put one node down.

        It inherits its neighbours' regions and their firmware, because
        somebody dropping a repeater on a map is adding a repeater to this
        network, not choosing a firmware strategy.
        """
        params: dict = {"name": name, "kind": kind, "lat": lat, "lon": lon}
        if height_m is not None:
            params["height_m"] = height_m
        if tx_dbm is not None:
            params["tx_dbm"] = tx_dbm
        if board:
            # A name nothing matches is refused rather than ignored: the board
            # decides the transmit ceiling, the noise figure and the battery,
            # so a silent fallback would be a different node answering.
            params["board"] = board
        self._wb.call("nodes.place", params)
        return Node(self._wb, name)

    def place_many(self, placements: list[dict]) -> list[Node]:
        """Put several down, then measure the links once.

        One warm at the end rather than one per node: nodes.place re-measures
        the matrix each time, and on a national network that is minutes
        repeated.
        """
        out = [self.place(**p) for p in placements]
        self._wb.call("links.recompute")
        return out

    def delete(self, *names: str) -> None:
        """Remove them, in one rebuild.

        All or none: a name that is not there refuses and removes nothing,
        because half a deletion leaves a scenario nobody described and no way
        to tell which half survived without asking again.
        """
        if names:
            self._wb.call("nodes.delete_many", list(names))

    def keep(self, *names: str) -> None:
        """Delete everything these do not name.

        The complement is worked out at the workbench rather than here, so it
        cannot be computed against a list that changed in between.
        """
        self._wb.call("nodes.keep", list(names))

    def select(self, *names: str, add: bool = False) -> None:
        self._wb.call(
            "nodes.add_to_selection" if add else "nodes.select_many", list(names)
        )

    def selected(self) -> list[str]:
        return [n.name for n in self.list() if n.selected]

    def stats(self) -> list[NodeStat]:
        return self._wb.node_stats()


class Node:
    """One node. Live: a handle, not a copy - it holds a name and asks."""

    def __init__(self, wb: Workbench, name: str) -> None:
        self._wb = wb
        self.name = name

    def __repr__(self) -> str:  # pragma: no cover - for a REPL
        return f"<Node {self.name!r}>"

    def __str__(self) -> str:
        return self.name

    # ---- what it is ------------------------------------------------------

    @property
    def info(self) -> NodeInfo:
        return Nodes(self._wb).info(self.name)

    @property
    def stat(self) -> NodeStat | None:
        for s in self._wb.node_stats():
            if s.name == self.name:
                return s
        return None

    @property
    def running(self) -> bool:
        s = self.stat
        return bool(s and s.running)

    @property
    def state(self) -> str:
        s = self.stat
        return s.state if s else "unknown"

    # ---- what it does ----------------------------------------------------

    def start(self) -> None:
        self._wb.call("node.start", self.name)

    def stop(self) -> None:
        self._wb.call("node.stop", self.name)

    def delete(self) -> None:
        self._wb.call("nodes.delete", {"node": self.name})

    def move(self, lat: float, lon: float) -> None:
        """Put it somewhere else. The physics moves with it: cached losses for
        this node are forgotten."""
        self._wb.call("nodes.move", {"name": self.name, "lat": lat, "lon": lon})

    def set_regions(self, *regions: str) -> None:
        self._wb.call("nodes.regions", {"node": self.name, "regions": list(regions)})

    def set_firmware(self, build: Build | str, apply: bool = True) -> None:
        """Change what it runs.

        Applied by default, which means stop, provision, start: firmware is
        chosen when a node launches, so recording it and leaving the node on
        its old build is the control somebody presses twice and then distrusts.
        Pass apply=False to record it for the next start instead - and know
        that is what you have done.
        """
        b = Build(version=build) if isinstance(build, str) else build
        self._wb.call(
            "node.set_firmware" if apply else "node.set_firmware_only",
            {
                "node": self.name,
                "version": b.version,
                "board": b.board,
                "role": b.role,
            },
        )

    @property
    def firmware(self) -> str:
        return self.info.firmware

    @firmware.setter
    def firmware(self, build: Build | str) -> None:
        self.set_firmware(build)

    @property
    def board(self) -> str:
        """What this node is, by board profile name.

        From the network rather than from the statistics: a stopped node has
        hardware just as surely as a running one.
        """
        return self.info.board

    @board.setter
    def board(self, name: str) -> None:
        """What hardware this node is.

        A change to the physics rather than a label, so it rebuilds and
        re-warms - and it clears a firmware pin made for a different board,
        because that image cannot run on this one and a pin nobody can honour
        reads as a configured node right up until it refuses to start.
        """
        self._wb.call("node.set_board", {"node": self.name, "board": name})

    def set_true_rf(self, on: bool = True) -> None:
        """Take waveform verdicts whatever the run's mode - the hybrid flag,
        for measuring one node honestly inside a cheap run."""
        self._wb.call("node.truerf", {"node": self.name, "on": on})

    def inject(self) -> None:
        """Originate a packet without firmware.

        It exercises the radio model and the channel; what it does not exercise
        is relaying, which is a firmware behaviour and needs a firmware.
        """
        self._wb.call("sim.inject", self.name)

    def provisioning(self) -> list[str]:
        """What this node is told at boot, in the console's own words."""
        return (self._wb.call("node.provisioning", self.name) or {}).get("commands", [])

    def serve(self, kind: str = "tcp") -> str:
        """Hand this companion to a real client, and say where to point it."""
        got = self._wb.call("bench.serve", {"node": self.name, "kind": kind}) or {}
        return got.get("addr", "")

    def unserve(self) -> None:
        self._wb.call("bench.drop", {"node": self.name})

    @property
    def console(self) -> Console:
        from .parts import Console

        return Console(self._wb, self.name)

    def wait_running(self, timeout: float | str = "5m") -> None:
        """Wait for its firmware to come up."""
        secs = seconds(timeout)

        def check():
            s = self.stat
            if s and s.running:
                return True, ""
            return False, (s.state if s else "no stat row yet")

        wait_for(check, secs, f"firmware on {self.name}")
