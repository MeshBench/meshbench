"""Which workbench this client may drive.

A client and the workbench it drives must be the same release. The protocol
number beside this rule says whether two ends can understand each other's
frames; it moves rarely and on purpose, so it cannot answer the question a
script actually has, which is whether the client in this virtualenv is the one
that came with the workbench on the PATH. Two releases apart with no protocol
bump between them connect happily and then disagree about a verb's parameters,
forty calls in, looking like the simulation misbehaving.
"""

from __future__ import annotations


def release() -> str:
    """The release this client belongs to, as PyPI spells it.

    Read from ``__version__`` rather than kept here, because the release
    workflow stamps that one line and a second copy would be a second thing to
    remember. Imported inside the function on purpose: the package's
    ``__init__`` imports this module, so a top-level import would be a cycle.

    A checkout carries whatever ``__version__`` last said, which is the previous
    release. That is not worth guarding against: a workbench built from the same
    checkout carries no release at all, so the pair is never compared.
    """
    from meshbench import __version__

    return __version__


def paired_release(ours: str, theirs: str) -> bool:
    """Whether these two releases may be used together.

    An exact match, or one of the two ends not being a release at all.

    The second half is what keeps the tree usable by the people working on it. A
    workbench built from a working copy has no release stamped in it, so
    insisting on equality would refuse every pair a developer has, for a
    disagreement that does not exist. Nothing is lost: what the rule exists to
    catch is a released client meeting a released workbench of another number,
    and both ends of that pair carry their stamp.
    """
    return not ours or not theirs or ours == theirs


def pairing_note(ours: str, theirs: str) -> str:
    """What to say about a check that did not compare anything.

    A pair nothing verified should be visible rather than quietly assumed sound.
    Returned rather than logged, because a client is a library: a script that
    wants the line in its output can print it, and one that does not is not made
    noisy by a rule that did not apply to it.
    """
    if not ours and not theirs:
        return (
            "release check skipped: neither this client nor the workbench "
            "is a release build"
        )
    if not ours:
        return (
            "release check skipped: this client is a development build; "
            f"the workbench is {theirs}"
        )
    if not theirs:
        return (
            "release check skipped: the workbench is a development build; "
            f"this client is {ours}"
        )
    return ""
