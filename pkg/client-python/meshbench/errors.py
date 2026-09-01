"""What went wrong, in a shape a script can catch.

The workbench answers with a sentence and a code. The sentence is the good
part - "no node is running firmware, so there is nothing to send to" - and it
survives untouched as the exception's message; the code decides which exception
it is.
"""

from __future__ import annotations


class MeshbenchError(Exception):
    """Anything this client raises."""


class Refused(MeshbenchError):
    """A verb the workbench declined, with its own words kept."""

    def __init__(self, verb: str, message: str, code: str = "") -> None:
        super().__init__(f"{verb}: {message}")
        self.verb = verb
        self.message = message
        self.code = code


class UnknownVerb(Refused):
    """Not a method this build has.

    Nearly always a client older or newer than the workbench - which connecting
    is supposed to have caught first, so seeing this is worth looking into.
    """


class BadParams(Refused):
    """The verb refused what it was given."""


class NotFound(Refused):
    """No node, build, area or job of that name."""


class Conflict(Refused):
    """The right request in the wrong state.

    Nothing loaded, nothing running to send to, no import preview to commit.
    """


class Unavailable(Refused):
    """A request this session cannot serve at all.

    A window verb with no window, or hardware that is not here.
    """


class Closing(Refused):
    """The workbench is shutting down. Retry against a new session."""


class ProtocolMismatch(MeshbenchError):
    """A client and a workbench that cannot speak to each other.

    Raised at connect rather than discovered on the fortieth call, because a
    mismatch found halfway through a script looks like the simulation
    misbehaving - and in a CI run that reads as a firmware regression.

    Either end may be the one to notice. When the workbench refuses the
    connection over the version this client declared, ``said`` carries that
    refusal whole and ``workbench`` is 0: it stopped the connection before it
    would say what it was, and its own sentence has the number in it.
    """

    def __init__(
        self,
        client: int,
        workbench: int,
        version: str = "",
        socket: str = "",
        said: str = "",
    ) -> None:
        super().__init__(
            said
            or (
                f"this client speaks protocol {client} and the workbench at "
                f"{socket} speaks {workbench} ({version}). Upgrade whichever is older"
            )
        )
        self.client = client
        self.workbench = workbench


class VersionMismatch(MeshbenchError):
    """A released client driving a workbench from a different release.

    Its own class rather than a Refused with a code, because a script has to be
    able to tell "these two were never meant to be used together" from "this
    build declined what I asked": the remedies have nothing in common.

    ``said`` carries the workbench's own refusal whole when it was the end that
    noticed. This client is the one that notices only against a build old enough
    to ignore what the client declared.
    """

    def __init__(self, client: str, workbench: str, said: str = "") -> None:
        super().__init__(
            said
            or (
                f"this client is from MeshBench {client} and this workbench is "
                f"MeshBench {workbench}. A client and the workbench it drives "
                f"must be the same release: install the {workbench} client, or "
                f"run the {client} workbench"
            )
        )
        self.client = client
        self.workbench = workbench


class Timeout(MeshbenchError):
    """A wait that ran out, saying what it wanted and what it last saw.

    Not a bare deadline: "timeout" in a CI log tells whoever reads it nothing,
    and the state at the moment it gave up is the only thing that does.
    """

    def __init__(self, what: str, after: float, last: str = "") -> None:
        msg = f"waited {after:.0f}s for {what}"
        if last:
            msg += f"; last saw: {last}"
        super().__init__(msg)
        self.what = what
        self.after = after
        self.last = last


_BY_CODE = {
    "unknown_verb": UnknownVerb,
    "bad_params": BadParams,
    "not_found": NotFound,
    "conflict": Conflict,
    "unavailable": Unavailable,
    "closing": Closing,
}


def refusal(verb: str, message: str, code: str) -> Refused:
    """Build the right exception for a code.

    An unrecognised code becomes a plain Refused rather than an error about the
    error: a workbench newer than this client may classify something in a way
    this version has never heard of, and swallowing that would be worse than
    passing it on.
    """
    return _BY_CODE.get(code, Refused)(verb, message, code)
