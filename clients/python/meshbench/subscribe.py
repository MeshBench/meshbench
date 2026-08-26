"""Being told, rather than asking.

The socket is request/reply, and stays that way: a script sends a verb and
reads its answer. A subscription is the other shape - the workbench writing a
line when something changes, unbidden - and it does not fit a call, so it is
given a connection of its own to stream on. A client that never subscribes
sees exactly the request/reply protocol it always did.

Each notification is ``{"event": ..., "data": ...}`` with no id. The absent id
is the whole distinction: a reply carries the id it answered, a notification
never does, so the two can never be confused for one another on the wire.
"""

from __future__ import annotations

import json
from collections.abc import Iterator
from dataclasses import dataclass
from typing import Any

from ._socket import Connection

#: The verb that opens a subscription on a connection.
SUBSCRIBE = "session.subscribe"


@dataclass(frozen=True)
class Notification:
    """One server-pushed event. ``dropped`` is how many snapshot notifications
    the server coalesced away before this one - zero for every other topic."""

    topic: str
    data: Any
    dropped: int = 0


class Subscription:
    """A live stream of notifications on a connection of its own.

    Iterate it for events; it blocks until the next one arrives and ends when
    the workbench hangs up. Use it as a context manager, or call close, so the
    extra connection does not outlive the interest.
    """

    def __init__(
        self,
        *topics: str,
        address: str | None = None,
    ):
        # No read timeout: a stream waits as long as it must between events,
        # where a call would rather fail than hang.
        self._conn = Connection(address, timeout=None)
        reply = self._conn.call(SUBSCRIBE, {"topics": list(topics)})
        if reply.get("error"):
            self._conn.close()
            raise ConnectionError(f"subscribe refused: {reply['error']}")
        self._file = self._conn._file

    def __iter__(self) -> Iterator[Notification]:
        return self

    def __next__(self) -> Notification:
        line = self._file.readline()
        if not line:
            raise StopIteration
        note = json.loads(line.decode())
        return Notification(
            topic=note.get("event", ""),
            data=note.get("data"),
            dropped=note.get("dropped", 0),
        )

    def close(self) -> None:
        # Shut the socket down before closing, so an iterator blocked on the
        # next event - the ordinary way to read a stream - is released rather
        # than left deadlocked against the close.
        self._conn.shutdown()
        self._conn.close()

    def __enter__(self) -> Subscription:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()


def subscribe(*topics: str, address: str | None = None) -> Subscription:
    """Open a subscription to the given topics - "status", "snapshot", and
    whatever else the workbench publishes."""
    return Subscription(*topics, address=address)
