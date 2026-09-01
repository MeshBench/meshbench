"""Which workbenches are running on this machine.

``Workbench.attach()`` goes to one address, which is enough while there is one
session per user. Two runs side by side - a soak beside the workbench somebody
is watching, two jobs on one CI runner - need a second address, and until this
existed the only record of where it was lived in the head of whoever typed it.

This is a module function rather than a method because the question comes
*before* a connection: a script asks what is running in order to decide what to
attach to.

**Telling a live session from what a dead one left behind.** A workbench killed
with SIGKILL cannot clean up after itself, and neither obvious check survives
that. A unix socket file outlives the process that bound it; a pid is reused,
so a pid that exists today may name somebody else's program. Both would report
a dead session as running. So the check is a connect to the address itself,
which is the same check the workbench makes before it takes an address, and the
leftover file is removed when nothing answers. A session's own tidying up
shortens this directory; it is not what makes the answer right.

**Windows works the same way.** There the address is a loopback host and port
and the check is a TCP connect. Nothing here is unix-only.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, replace
from pathlib import Path
from typing import Any

from . import errors
from ._socket import Connection, _cache_dir

#: Chooses the directory the session files live in, for a test or a CI job that
#: wants a registry of its own rather than the user's.
SESSIONS_ENV = "MESHBENCH_CONTROL_SESSIONS"

#: How long a live session is given to describe itself. Generous, because the
#: answer is not worth a wrong row: a session in the middle of something slow
#: is still running and is listed either way, with its description missing.
DETAIL_WAIT = 2.0


@dataclass(frozen=True)
class Session:
    """One running workbench, as somebody choosing between several sees it.

    Snapshot, read once. The description - version, mode, project, node count -
    is asked of the session while the list is being built, so it is what was
    true a moment ago rather than what was true when the session started. It is
    empty for a session too busy to answer in the moment it was asked; that
    session is still listed, because it is still running.
    """

    #: Where it answers, in the form ``-control-socket`` and ``attach`` take.
    address: str = ""
    #: What separates two otherwise identical runs. ``started_at`` is when the
    #: socket opened, which is when the session became something another
    #: process could reach.
    pid: int = 0
    started_at: str = ""
    version: str = ""
    #: "workbench" or "headless", and empty if it did not answer in time. A
    #: string rather than a windowed flag for that reason: an absent bool reads
    #: as "headless", which would be a claim nobody made.
    mode: str = ""
    #: The fixture or project it has open.
    project: str = ""
    nodes: int = 0
    #: Authorises a TCP connection to this session, and is never part of a
    #: verb's answer: it is read from the 0600 file beside the address.
    token: str = ""
    #: Whether this is the session that was asked, which only the list a
    #: workbench answers with can say. Spelled with the prefix because a
    #: dataclass field named for the first argument of every method collides
    #: with it; on the wire and in the Go client the key is plain "self".
    is_self: bool = False

    @property
    def windowed(self) -> bool:
        """Whether it has an interface. False when it did not say."""
        return self.mode == "workbench"

    def connect(self, timeout: float | None = 300.0) -> Connection:
        """Open a connection to this session, with its own token."""
        return Connection(self.address, timeout=timeout, token=self.token)


def sessions_dir() -> Path:
    """The per-user directory the session files live in."""
    named = os.environ.get(SESSIONS_ENV)
    d = Path(named) if named else _cache_dir() / "sessions"
    d.mkdir(parents=True, exist_ok=True)
    return d


def sessions() -> list[Session]:
    """The workbenches running on this machine, oldest first.

    A session that has died is not listed, however it died, and what it left
    behind is removed on the way past.
    """
    found: list[Session] = []
    for path in sorted(sessions_dir().glob("*.json")):
        row = _read(path)
        if row is None:
            continue
        detail = _describe(row)
        if detail is None:
            # Nothing is answering there, so nothing is running there.
            path.unlink(missing_ok=True)
            continue
        found.append(replace(row, **detail))
    # Oldest first, so two runs listed twice come back in the same order. The
    # timestamps are RFC 3339 written by one program on one machine, so the
    # text sorts in time order, and the address settles a tie whatever happens.
    return sorted(found, key=lambda s: (s.started_at, s.address))


def _read(path: Path) -> Session | None:
    try:
        got = json.loads(path.read_text())
    except (OSError, ValueError):
        return None
    address = got.get("address", "")
    if not address:
        return None
    return Session(
        address=address,
        pid=int(got.get("pid", 0)),
        started_at=str(got.get("started_at", "")),
        token=str(got.get("token", "")),
    )


def _describe(row: Session) -> dict[str, Any] | None:
    """One connection, both answers: whether anything is there, and what it is
    running. A connection that opens is a live session whether or not it finds
    a moment to describe itself.
    """
    try:
        conn = row.connect(timeout=DETAIL_WAIT)
    except (OSError, ConnectionError, ValueError, errors.MeshbenchError):
        return None
    try:
        reply = conn.call("session.hello")
        got = reply.get("result") or {}
        return {
            "version": str(got.get("version", "")),
            "mode": str(got.get("mode", "")),
            "project": str(got.get("project", "")),
            "nodes": int(got.get("nodes", 0)),
        }
    except (OSError, ConnectionError, ValueError):
        return {}
    finally:
        conn.close()
