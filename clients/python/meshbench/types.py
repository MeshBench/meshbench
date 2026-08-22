"""The values a script holds.

Snapshots, all of them: read once, and never changing afterwards. The live
things - the workbench, a node handle - are classes with methods that re-read
the session every time they are asked. Which one a thing is decides whether it
can be held across a run and still be true.
"""

from __future__ import annotations

from dataclasses import dataclass, field, fields
from typing import Any

# Node kinds, as the scenario names them.
SIMPLE_REPEATER = "simple-repeater"
ADVANCED_REPEATER = "advanced-repeater"
COMPANION = "companion"
#: Holds posts for clients to collect and **does not forward**. A mesh that
#: treats one as a repeater overstates its own reach.
ROOM_SERVER = "room-server"
#: Runs no firmware and transmits nothing: captures the summed field at its
#: antenna and hands back IQ.
SDR_OBSERVER = "sdr-observer"
#: Interference that is not MeshCore, propagated through the same terrain as
#: everything else.
EMITTER = "emitter"

# Event classes, as the engine buckets them.
SENT = "sent"
RECEIVED = "received"
HALF_DUPLEX = "half-duplex"
INTERFERENCE = "interference"
FLOOR = "floor"


def _from_dict(cls, raw: dict[str, Any]):
    """Build a dataclass from a reply, ignoring what it has not heard of.

    Ignoring rather than failing, deliberately: the workbench adds fields to
    results as it grows, and a client that raised on an unfamiliar key would
    break on every release for no benefit at all.
    """
    known = {f.name for f in fields(cls)}
    return cls(**{k: v for k, v in raw.items() if k in known})


@dataclass(frozen=True)
class Hello:
    """What a connection is talking to. Read once, at connect."""

    protocol: int = 0
    version: str = ""
    #: "workbench" or "headless".
    mode: str = ""
    socket: str = ""
    verbs: int = 0
    #: pid and started_at tell a restart from a reconnect. The scenario does
    #: not survive a restart - nodes, boundary, inference and firmware
    #: assignments live in the process - so a script picking up a session it
    #: did not start has to be able to ask.
    pid: int = 0
    started_at: str = ""

    @property
    def headless(self) -> bool:
        return self.mode == "headless"

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Hello:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class NodeInfo:
    """What the network is, per node.

    What a node is *doing* is NodeStat: the two change on completely different
    timescales, and the store publishes them apart.
    """

    name: str = ""
    kind: str = ""
    lat: float = 0.0
    lon: float = 0.0
    height_m: float = 0.0
    tx_dbm: float = 0.0
    regions: list[str] = field(default_factory=list)
    firmware: str = ""
    #: What the node is, by board profile name. Not firmware_board, which is
    #: what its image was built for - the two agree most of the time and come
    #: apart the moment a host build is pointed at a T-Deck.
    board: str = ""
    firmware_board: str = ""
    sent: int = 0
    heard: int = 0
    selected: bool = False

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> NodeInfo:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class NodeStat:
    """What one node is costing and doing right now."""

    name: str = ""
    backend: str = ""
    firmware: str = ""
    running: bool = False
    #: "running", "stopped", or one of the transitions. A boolean cannot say
    #: "changing firmware", and a row that goes blank while it happens looks
    #: like a node that has died.
    state: str = ""
    board: str = ""
    pid: int = 0
    rss_bytes: int = 0
    cpu_ms: int = 0
    cpu_pct: float = 0.0
    sent: int = 0
    heard: int = 0
    last_sent_ms: int = 0
    last_heard_ms: int = 0
    last_sent_to: str = ""
    last_heard_from: str = ""
    #: The chip's own counters - the only way to tell a busy mesh from a radio
    #: that cries busy too readily.
    irq_reads: int = 0
    busy_reads: int = 0
    busy_ms: int = 0
    spurious: int = 0

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> NodeStat:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Event:
    """One thing the engine did.

    The frame bytes are deliberately absent: a long run has millions of these,
    and the one packet somebody wants is asked for by id.
    """

    at_ms: int = 0
    kind: str = ""
    from_: str = ""
    to: str = ""
    message_id: int = 0
    packet_id: int = 0
    #: None where the ratio is infinite - a reception with no noise at all -
    #: because JSON has no way to say infinity and null is what it has for "no
    #: value". Absent is not zero.
    snr_db: float | None = None
    detail: str = ""
    class_: str = ""

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Event:
        # "from" and "class" are keywords, so they carry a trailing underscore
        # here. Renamed rather than left as dictionary keys: a script writing
        # event.from_ is nicer than event["from"], and this is the one place
        # that has to know.
        r = dict(raw)
        r["from_"] = r.pop("from", "")
        r["class_"] = r.pop("class", "")
        return _from_dict(cls, r)


@dataclass(frozen=True)
class SimState:
    """The clock."""

    playing: bool = False
    now_ms: int = 0
    until_ms: int = 0
    events: int = 0
    step_ms: int = 0
    seed: int = 0

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> SimState:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Build:
    """One firmware image, as the library sees it.

    Version, board and role travel together because a board image is not a
    build on its own: "wadamesh" means nothing until it is wadamesh for a
    LilyGo_TDeck, built as a companion.
    """

    role: str = ""
    version: str = ""
    board: str = ""
    bytes: int = 0
    on_disk: bool = False
    path: str = ""
    in_use: int = 0
    #: Exists only because nodes are pinned to it: nothing on disk, nothing
    #: published. Pinning to one succeeds and then fails at start, which reads
    #: as the library losing builds rather than as a pin nobody can honour.
    unavailable: bool = False

    def __str__(self) -> str:
        if not self.board:
            return self.version
        return f"{self.board} - {self.role} {self.version}"

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Build:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class JobInfo:
    """A long operation in flight."""

    id: str = ""
    what: str = ""
    done: int = 0
    total: int = 0
    finished: bool = False

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> JobInfo:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Provenance:
    """What a measurement was measured under.

    Printed above any number a script emits. Not decoration: a scripted number
    gets pasted into a report with the caveats stripped, so the caveats have to
    be in the value.
    """

    rf_mode: str = ""
    excess_loss_db: float = 0.0
    calibrated: bool = False
    seed: int = 0

    def __str__(self) -> str:
        fit = (
            "excess loss fitted to real receptions"
            if self.calibrated
            else "default excess loss"
        )
        return (
            f"MeshBench: {self.rf_mode} reception, {fit} — a best case; "
            "no multipath, no body loss, no oscillator error"
        )
