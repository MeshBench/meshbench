"""The values a script holds.

Snapshots, all of them: read once, and never changing afterwards. The live
things - the workbench, a node handle - are classes with methods that re-read
the session every time they are asked. Which one a thing is decides whether it
can be held across a run and still be true.
"""

from __future__ import annotations

from dataclasses import dataclass, field, fields
from typing import Any

from .sets import Class, Kind, Role


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
    #: The release this workbench belongs to, empty for a build from a working
    #: copy. version is prose; this is the number the pairing rule reads.
    release: str = ""
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
    #: What this session has open, which is what tells two of them apart in a
    #: list of what is running.
    project: str = ""
    nodes: int = 0

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
    kind: Kind | str = ""
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
    #: No sent or heard: what a node has transmitted and received is on
    #: NodeStat, from the engine's own scoreboard. They were carried here too
    #: and answered 0 for every node in every reply, which reads as a silent
    #: mesh rather than as a field nobody filled in.
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
    kind: Kind | str = ""
    from_: str = ""
    to: str = ""
    message_id: int = 0
    packet_id: int = 0
    #: None where the ratio is infinite - a reception with no noise at all -
    #: because JSON has no way to say infinity and null is what it has for "no
    #: value". Absent is not zero.
    snr_db: float | None = None
    detail: str = ""
    class_: Class | str = ""

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
    # Three states, because a warm that stopped to ask permission to download
    # terrain is neither running nor finished. Reading "not warming" as
    # "measured" is how a script came to read a study over ground nobody
    # fetched. ground says what it stood on: state of "terrain", "partial" or
    # "bare-earth", and chosen saying whether anybody answered the question.
    warming: bool = False
    links_measured: bool = False
    warm_held: bool = False
    ground: dict[str, Any] = field(default_factory=dict)
    # Whether the seed above is a promise. It is not on a network carrying a
    # node that runs in an emulator: that node's firmware is stepped by the
    # emulator's clock rather than by the run's, so the same seed puts its
    # traffic at a different instant every time. Read it before diffing two
    # runs or subtracting one arm from another.
    reproducible: bool = True
    not_reproducible_why: str = ""

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

    role: Role | str = ""
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
class BuildDetails:
    """One build, in full: what a row cannot hold.

    Separate from :class:`Build` because the library is deliberately a list -
    role, version, size, a tick. Where the file actually is, whether it is a
    whole flash image or half of one, and what has been decided about how it
    runs are the questions somebody has once a build does not do what they
    expected.
    """

    role: Role | str = ""
    version: str = ""
    board: str = ""
    native: bool = False
    on_disk: bool = False
    path: str = ""
    #: Where the settings below are written. Named whether or not any exist,
    #: because "where does this live" is asked of a build that has none as
    #: often as of one that has.
    settings_path: str = ""
    bytes: int = 0
    modified: str = ""
    in_use: int = 0
    #: What reading the front of the image says it is, and whether a board
    #: could start from it. An application-only image imports, lists and pins
    #: exactly like a whole one and then starts nothing.
    kind: str = ""
    bootable: bool = False
    flash_mb: int = 0
    #: Kept beside the image, so they follow this build rather than the board.
    coproc_at_reset: bool = False
    #: This firmware will not get far without storage in the board's slot, so
    #: every node running it is given a card whatever its own slot was set to.
    card_required: bool = False
    notes: str = ""

    def __str__(self) -> str:
        if not self.board:
            return self.version
        return f"{self.board} - {self.role} {self.version}"

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> BuildDetails:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class CardSlot:
    """What is in one node's card slot.

    A slot is not a fitted card: the board says the slot exists, the node says
    whether it is filled, and a firmware that keeps its settings on a card
    fills it regardless.
    """

    node: str = ""
    #: "" for the board's own answer, "fitted" or "empty" for a decision.
    slot: str = ""
    fitted: bool = False
    #: The file behind the card, and the one it would use if handed none.
    file: str = ""
    own_file: str = ""
    bytes: int = 0
    required_by_firmware: bool = False
    board_has_slot: bool = False
    wiped: bool = False

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> CardSlot:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Screen:
    """What a board's display is showing, as numbers rather than a picture.

    Enough to answer "did anything change" after a press or a touch, which is
    the question every check of an input comes down to; for the picture itself
    ask for a screenshot.
    """

    node: str = ""
    #: False when the board has drawn nothing yet, or has no display - the
    #: other fields mean nothing then.
    has_screen: bool = False
    width: int = 0
    height: int = 0
    bpp: int = 0
    on: bool = False
    #: How many framebuffer bytes are non-zero - how much is lit.
    lit: int = 0
    #: Identifies the frame: two screens with the same digest are the same
    #: picture, which lit cannot promise. It is what a wait-for-change watches.
    digest: str = ""

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Screen:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Shot:
    """A captured display: a PNG under the node's own work directory, and the
    frame's dimensions. The frame is exactly what the controller holds."""

    node: str = ""
    path: str = ""
    width: int = 0
    height: int = 0
    bpp: int = 0
    on: bool = False

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Shot:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class JobInfo:
    """A long operation in flight."""

    id: str = ""
    what: str = ""
    done: int = 0
    total: int = 0
    finished: bool = False
    #: Ended without doing what it was for. Separate from ``finished`` because
    #: a waiter needs both: "stop waiting" and "this did not work" are
    #: different answers, and telling them apart by reading ``what`` means
    #: matching on prose.
    failed: bool = False

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


@dataclass(frozen=True)
class NameMatch:
    """One answer from a name search, and how sure it is.

    ``score`` runs 0 to 1, ranked best first by the workbench. It exists so a
    script can tell "found it" from "found something that shares a word": a top
    result at 0.3 is a prompt to look at the list, not a node to start talking
    to.
    """

    name: str = ""
    score: float = 0.0
    kind: Kind | str = ""
    lat: float = 0.0
    lon: float = 0.0

    def __str__(self) -> str:
        return self.name

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> NameMatch:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class ImportPreview:
    """What a fetch found, before anything has been changed.

    ``skipped_no_position`` and ``uncertain`` are the two numbers worth reading
    before committing. A node with no position cannot be simulated at all, and
    an uncertain one is being placed to within kilometres - the answer it gives
    is that vague too, however confident the rest of the output looks.
    """

    records: int = 0
    nodes: int = 0
    skipped_no_position: int = 0
    uncertain: int = 0

    def __str__(self) -> str:
        out = f"{self.records} records, {self.nodes} usable"
        if self.skipped_no_position:
            out += f", {self.skipped_no_position} with no position"
        if self.uncertain:
            out += f", {self.uncertain} placed only roughly"
        return out

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> ImportPreview:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Antenna:
    """What a node stands under: which sort, and where it points.

    Directional in azimuth, so ``bearing_deg`` is not decoration. A beam is
    twenty decibels or more down off its boresight, and the difference between
    a node aimed down a valley and the same node aimed at a hillside is the
    difference between a link and no link.
    """

    node: str = ""
    #: "isotropic", "dipole", "collinear" or "yagi", and "" for a node with no
    #: antenna at all - which is not an omni at 0 dBi, and is said rather than
    #: filled in.
    pattern: str = ""
    gain_dbi_peak: float = 0.0
    beamwidth_deg: float = 0.0
    front_to_back_db: float = 0.0
    #: Compass bearing of the boresight, 0 at north.
    bearing_deg: float = 0.0
    #: Degrees the beam is tilted below the horizon, which is what a mast on a
    #: hill does to reach the town underneath it.
    downtilt_deg: float = 0.0
    #: "vertical", "horizontal" or "circular", and "" for a node that has not
    #: said. Unstated costs nothing; a mismatch costs 3 dB circular to linear
    #: and 20 dB vertical to horizontal.
    polarisation: str = ""
    #: Cable and connector loss, as a positive number.
    feedline_db: float = 0.0
    #: What the pattern manages on its own boresight, before the feedline.
    peak_dbi: float = 0.0

    def __str__(self) -> str:
        if not self.pattern:
            return "no antenna"
        return (
            f"{self.pattern}, {self.gain_dbi_peak:.1f} dBi, "
            f"pointing {self.bearing_deg:.0f} degrees"
        )

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Antenna:
        return _from_dict(cls, raw)


@dataclass(frozen=True)
class Aimed:
    """Where an antenna ended up pointing, and what that won.

    ``gain_dbi`` is the point of it: on an omni the answer is the same as
    before, and a call that reported success while changing nothing would be
    one to distrust.
    """

    node: str = ""
    at: str = ""
    bearing_deg: float = 0.0
    distance_km: float = 0.0
    gain_dbi: float = 0.0

    def __str__(self) -> str:
        return (
            f"{self.node} points at {self.at}, {self.bearing_deg:.0f} degrees, "
            f"{self.gain_dbi:.1f} dBi that way"
        )

    @classmethod
    def parse(cls, raw: dict[str, Any]) -> Aimed:
        return _from_dict(cls, raw)
