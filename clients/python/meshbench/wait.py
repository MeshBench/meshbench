"""Waiting, in one place.

Every wait in this package is a function here, never a sleep in a script.
``tools/soak/soak.py`` hand-wrote the same poll loop three times in
seventy-two lines, each with its own interval and its own timeout, and its own
header records having sampled the wrong moment because of it.

They poll. When the socket learns to push, this file changes and no caller
does - which is the whole reason the clients were built before the events.

# Durations are timedelta

Every duration in this package is a ``datetime.timedelta``. Not a number of
seconds, and not a string like "5m": a bare number leaves the reader asking
"five what", and a string means inventing a parser nobody else's code
understands and no editor can complete. Python already has a duration type.

# Which clock

Two clocks appear in this API and they are not the same one:

- **Simulated** - the mesh's own. ``sim.run``, ``schedule.add(at=, every=)``,
  ``sim.wait_until``. Five simulated minutes on 155 emulated nodes is a great
  deal more than five of yours.
- **Wall** - yours. Every ``timeout``, and ``sim.run(wait=)``: how long you are
  prepared to sit there before giving up.

Both are timedeltas, so the parameter is what tells them apart. Each one says
which it is in its own docstring, and the ones that mean the mesh's clock are
never called ``timeout``.
"""

from __future__ import annotations

import time
from collections.abc import Callable
from datetime import timedelta

from .errors import Timeout

#: How long the waits give things by default. Named rather than sprinkled, so
#: they can be read in one place and so a caller who passes nothing gets
#: something defensible rather than something invented at the call site.
FIRMWARE_WAIT = timedelta(minutes=10)
"""Firmware coming up on a whole mesh. Real firmware is minutes; emulated
boards are longer."""

RUN_WAIT = timedelta(minutes=30)
"""A run of simulated time finishing, measured on your clock."""

JOB_WAIT = timedelta(minutes=30)
"""A long job - a warm, a download, a build - finishing."""

EVENT_WAIT = timedelta(minutes=5)
"""One event arriving."""

#: The slowest a wait will poll, and where it starts.
#:
#: It backs off from the first to the second. Something that is about to happen
#: is noticed promptly; something that takes ten minutes is not asked four
#: thousand times on the way. nodes.stats in particular costs a /proc read per
#: node, so polling it at ten hertz on a 155-node mesh is fifteen hundred reads
#: a second - during firmware startup, which is the busiest moment there is.
POLL_FIRST = 0.05
POLL_SLOWEST = 1.0


def as_seconds(value: timedelta) -> float:
    """A timedelta as seconds, refusing anything else by name.

    Strict on purpose. Accepting a bare number as well would put the question
    "five what?" back into every call site, which is the thing timedelta exists
    to answer.
    """
    if isinstance(value, timedelta):
        return value.total_seconds()
    raise TypeError(
        f"expected a datetime.timedelta, got {type(value).__name__} "
        f"({value!r}) - write timedelta(minutes=5) rather than 300 or '5m'"
    )


def wait_for(
    check: Callable[[], tuple[bool, str]],
    timeout: timedelta,
    what: str,
) -> None:
    """Poll until check says yes, or the time runs out.

    ``timeout`` is wall clock: how long you are prepared to sit here.

    check returns whether it is done and, if not, what it saw - which is what
    the Timeout reports. An exception from check stops the wait rather than
    being retried: a verb refusing because a node does not exist will refuse
    the same way in ten seconds.
    """
    limit = as_seconds(timeout)
    deadline = time.monotonic() + limit
    interval = POLL_FIRST
    last = ""
    while True:
        done, saw = check()
        if done:
            return
        if saw:
            last = saw
        if time.monotonic() > deadline:
            raise Timeout(what, limit, last)
        time.sleep(interval)
        interval = min(interval * 1.5, POLL_SLOWEST)
