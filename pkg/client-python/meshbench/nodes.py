"""The network: what is in it, and what one node can be asked to do."""

from __future__ import annotations

from collections.abc import Iterator
from datetime import timedelta
from typing import TYPE_CHECKING, Any

from . import errors
from .device import Device
from .sets import Board, Kind, Transport
from .types import Build, CardSlot, NameMatch, NodeInfo, NodeStat
from .wait import FIRMWARE_WAIT, wait_for

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

    def search(self, query: str, limit: int = 10) -> list[NameMatch]:
        """Find nodes by name, best first, when you cannot type the name.

        Imported names carry emoji and accents - "🏔️ West Lomond 📡" is one
        real node - so matching is done on letters and digits alone, with
        accents folded and word order ignored. The ranking happens at the
        workbench rather than here, so this client and the Go one agree about
        which result is the top one.

        Returns an empty list rather than raising: "nothing matched" is an
        answer, and the caller usually wants to widen the query rather than
        handle an exception.
        """
        got = self._wb.call("nodes.search", {"query": query, "limit": limit}) or {}
        return [NameMatch.parse(m) for m in got.get("matches") or []]

    def find(self, query: str, least: float = 0.5) -> Node:
        """The one node a search meant, or a refusal naming what it did find.

        ``least`` is the score below which the top answer is not good enough to
        act on. Taking the top result unconditionally is how a script ends up
        sending an advert from a node that merely shared a word with what was
        asked for, and it does that silently.
        """
        matches = self.search(query, limit=5)
        if not matches or matches[0].score < least:
            near = ", ".join(f"{m.name!r} ({m.score:.2f})" for m in matches[:3])
            raise errors.NotFound(
                "nodes.search",
                f"nothing matches {query!r} well enough"
                + (f"; nearest were {near}" if near else ""),
                "not_found",
            )
        return Node(self._wb, matches[0].name)

    def near(self, node: Node | str, count: int = 0) -> list[Node]:
        """The nodes closest to this one, nearest first.

        Trimming an imported deployment to a neighbourhood is the first thing
        anybody does with one, and the distance is the workbench's own - the
        same great circle its path losses use.
        """
        got = self._wb.call("nodes.near", {"node": str(node), "count": count}) or {}
        return [Node(self._wb, n["name"]) for n in got.get("near") or []]

    def of_kind(self, kind: str) -> list[Node]:
        return [Node(self._wb, n.name) for n in self.list() if n.kind == kind]

    def place(
        self,
        name: str,
        kind: Kind | str = Kind.SIMPLE_REPEATER,
        lat: float = 0.0,
        lon: float = 0.0,
        height_m: float | None = None,
        tx_dbm: float | None = None,
        board: Board | str | None = None,
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

    def delete(self, *nodes: Node | str) -> None:
        """Remove them, in one rebuild.

        All or none: a name that is not there refuses and removes nothing,
        because half a deletion leaves a scenario nobody described and no way
        to tell which half survived without asking again.
        """
        if nodes:
            self._wb.call("nodes.delete_many", _names(nodes))

    def keep(self, *nodes: Node | str) -> None:
        """Delete everything these do not name.

        The complement is worked out at the workbench rather than here, so it
        cannot be computed against a list that changed in between.
        """
        self._wb.call("nodes.keep", _names(nodes))

    def select(self, *nodes: Node | str, add: bool = False) -> None:
        self._wb.call(
            "nodes.add_to_selection" if add else "nodes.select_many", _names(nodes)
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

    def output(self, source: str = "serial", lines: int = 200) -> list[str]:
        """What this node printed, from one of four voices.

        `serial` is the board's own port - a native node's standard error;
        `boot` is the ROM's, on a board whose application talks over USB;
        `emulator` is what QEMU or Renode said about running it; `radio` is
        the radio model's log.

        The lines, not a count of them: a board that has gone quiet is read
        by looking at what it last said.
        """
        got = (
            self._wb.call(
                "node.output",
                {"node": self.name, "source": source, "lines": lines},
            )
            or {}
        )
        return got.get("tail") or []

    # ---- looking at it, and prodding it ---------------------------------

    @property
    def device(self) -> Device:
        """This node as a device to drive: its screen, buttons and panel. All
        of it works headless - the display is the framebuffer the controller
        holds, not a picture of the desktop. Distinct from `board`, which is
        the model name this hardware is."""
        return Device(self._wb, self.name)

    def radio(self) -> dict[str, Any]:
        """What this node's radio is set to - the same thing the workbench
        shows under Radio. What the model assumes, and, for a node that is
        running, what it reports back and where the two differ. Left as a dict
        because a repeater and a companion answer it differently."""
        return self._wb.call("node.radio", {"node": self.name}) or {}

    def wipe(self) -> None:
        """Put this board back to factory: its flash, its card, its files.

        A board keeps what it was told between runs, as hardware does, so a
        node configured into a corner stays there until this is called. Refused
        while it is running, rather than rewriting a flash underneath the
        emulator holding it.
        """
        self._wb.call("node.wipe", {"node": self.name})

    def card(
        self,
        *,
        fitted: bool | None = None,
        file: str | None = None,
        wipe: bool = False,
    ) -> CardSlot:
        """What is in this node's card slot, and changing it.

        A slot is not a fitted card: the board says the slot exists, this says
        whether it is filled. Two of the same handheld in one network, one with
        storage and one without, is an ordinary thing to want.

        ``file`` hands the node a card of your own - shared between runs, or
        prepared in advance; an empty string returns it to its own, named after
        it and kept beside its flash. ``wipe`` erases it, which is what
        reformatting one is, and is refused while the node is running.

        A firmware marked as needing a card fills the slot whatever this says,
        because a build that keeps its settings there boots into nothing
        without one.
        """
        p: dict[str, Any] = {"node": self.name}
        if fitted is not None:
            p["fitted"] = fitted
        if file is not None:
            p["file"] = file
        if wipe:
            p["wipe"] = True
        return CardSlot.parse(self._wb.call("node.card", p) or {})

    def output_window(self, source: str = "serial") -> None:
        """Open one of this node's logs in a window of its own.

        A tab is one pane. What people do while a board is misbehaving is watch
        its screen and two of its logs together - what the board printed beside
        what the emulator said about running it - and that needs windows.
        """
        self._wb.call("node.output_window", {"node": self.name, "source": source})

    def move(self, lat: float, lon: float) -> None:
        """Put it somewhere else. The physics moves with it: cached losses for
        this node are forgotten."""
        self._wb.call("nodes.move", {"name": self.name, "lat": lat, "lon": lon})

    def set_regions(self, *regions: str) -> None:
        """What this node relays flood traffic for."""
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
    def build(self) -> Build | None:
        """The build this node runs, or None if it is pinned to nothing.

        The whole row rather than the version string, because deleting a build
        or comparing two needs its path and its board, and reassembling those
        from a version is the kind of guesswork that deletes the wrong file.
        """
        want = self.info.firmware
        if not want:
            return None
        for b in self._wb.firmware.library():
            if b.version == want:
                return b
        return None

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
    def board(self, name: Board | str) -> None:
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

    def serve(self, over: Transport = Transport.TCP) -> str:
        """Hand this companion to a real client, and say where to point it."""
        got = self._wb.call("bench.serve", {"node": self.name, "kind": over}) or {}
        return got.get("addr", "")

    def unserve(self) -> None:
        self._wb.call("bench.drop", {"node": self.name})

    @property
    def console(self) -> Console:
        from .parts import Console

        return Console(self._wb, self.name)

    def wait_running(self, timeout: timedelta = FIRMWARE_WAIT) -> None:
        """Wait for its firmware process to be up.

        ``timeout`` is wall clock - how long you are prepared to sit here - not
        simulated time. Starting a process is real work on the real machine.
        """

        def check():
            s = self.stat
            if s and s.running:
                return True, ""
            return False, (s.state if s else "no stat row yet")

        wait_for(check, timeout, f"firmware on {self.name}")


def _names(nodes: tuple[Node | str, ...]) -> list[str]:
    """Names, whether handles or strings were passed.

    search() and near() hand back handles and every verb takes names, so
    without this each caller writes the same map(str) - and the one that
    forgets sends a repr down the socket and is told there is no node named
    "<Node 'Bench'>".
    """
    return [str(n) for n in nodes]
