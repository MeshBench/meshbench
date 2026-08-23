"""Waiting, in one place.

Every wait in this package is a function here, never a sleep in a script.
`tools/soak/soak.py` hand-wrote the same poll loop three times in seventy-two
lines, each with its own interval and its own timeout, and its own header
records having sampled the wrong moment because of it.

They poll. When the socket learns to push, this file changes and no caller
does - which is the whole reason the clients were built before the events.
"""

from __future__ import annotations

import time
from collections.abc import Callable

from .errors import Timeout

#: How often a wait asks.
#:
#: A tenth of a second, because these wait on things that take seconds to
#: minutes - firmware coming up, a warm finishing, a run ending - and a faster
#: poll would be a verb per frame against a store that has real work to do.
POLL_EVERY = 0.1


def wait_for(
    check: Callable[[], tuple[bool, str]],
    timeout: float,
    what: str,
    poll: float = POLL_EVERY,
) -> None:
    """Poll until check says yes, or the time runs out.

    check returns whether it is done and, if not, what it saw - which is what
    the Timeout reports. An exception from check stops the wait rather than
    being retried: a verb refusing because a node does not exist will refuse
    the same way in ten seconds.
    """
    deadline = time.monotonic() + timeout
    last = ""
    while True:
        done, saw = check()
        if done:
            return
        if saw:
            last = saw
        if time.monotonic() > deadline:
            raise Timeout(what, timeout, last)
        time.sleep(poll)


def seconds(value: float | str) -> float:
    """Accept 90, or "90s", or "3m", or "1h".

    Because a test that says timeout="10m" reads better than timeout=600, and
    a reader should not have to divide.
    """
    if isinstance(value, (int, float)):
        return float(value)
    text = value.strip().lower()
    units = {"s": 1.0, "m": 60.0, "h": 3600.0}
    if text and text[-1] in units:
        return float(text[:-1]) * units[text[-1]]
    return float(text)
