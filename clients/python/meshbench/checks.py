"""What a run is told to send, and what has to be true of it afterwards.

These were reachable only through ``wb.call("schedule.add", {"at_ms": 5000,
"every_ms": 20000})``, which is the shape this package exists to remove: a verb
name spelled by hand, parameters in milliseconds because that is what the wire
happens to use, and no way for an editor to tell you any of it. An example
written that way is an advertisement for not using the library.
"""

from __future__ import annotations

import xml.etree.ElementTree as ET
from dataclasses import dataclass, field
from datetime import timedelta
from typing import TYPE_CHECKING

from .wait import as_seconds

if TYPE_CHECKING:  # pragma: no cover
    from .nodes import Node
    from .types import Provenance
    from .workbench import Workbench


class Schedule:
    """What the mesh is told to send, and when. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def add(
        self,
        node: str | Node,
        command: str,
        at: timedelta | None = None,
        every: timedelta | None = None,
    ) -> int:
        """Have a node send something, once or repeatedly.

        ``at`` and ``every`` are **simulated** time - the mesh's own clock,
        not yours. The verb underneath takes milliseconds; nobody writing a
        script should have to.

        Repeating traffic has worked all along and nothing said so, which to
        somebody writing a script is the same as it not existing.
        """
        params: dict[str, object] = {"node": str(node), "command": command}
        if at is not None:
            params["at_ms"] = int(as_seconds(at) * 1000)
        if every is not None:
            params["every_ms"] = int(as_seconds(every) * 1000)
        got = self._wb.call("schedule.add", params) or {}
        return got.get("sends", 0)

    def clear(self) -> int:
        """Forget all of them."""
        return (self._wb.call("schedule.clear") or {}).get("cleared", 0)

    def __len__(self) -> int:
        return self._wb.snapshot().get("scheduled_sends", 0)


@dataclass(frozen=True)
class Check:
    """One assertion, and what the run made of it."""

    kind: str = ""
    node: str = ""
    passed: bool = False
    got: str = ""
    want: str = ""

    def __str__(self) -> str:
        mark = "pass" if self.passed else "FAIL"
        where = f" at {self.node}" if self.node else ""
        return f"{mark}  {self.kind}{where}: got {self.got}, want {self.want}"


@dataclass(frozen=True)
class Report:
    """What a run passed and failed, with what it was measured under."""

    passed: int = 0
    total: int = 0
    checks: list[Check] = field(default_factory=list)
    provenance: Provenance | None = None

    @property
    def ok(self) -> bool:
        """Whether every assertion held.

        A report with no assertions is **not** ok. A fixture that carries none
        can report but cannot pass, and a green tick that checked nothing is
        the worst outcome available here.
        """
        return self.total > 0 and self.passed == self.total

    @property
    def failures(self) -> list[Check]:
        return [c for c in self.checks if not c.passed]

    def __str__(self) -> str:
        lines = []
        if self.provenance is not None:
            lines.append(str(self.provenance))
        if self.total == 0:
            lines.append("no assertions, so this run checked nothing")
        else:
            lines.append(f"{self.passed} of {self.total} assertions passed")
        lines += [f"  {c}" for c in self.failures]
        return "\n".join(lines)

    def write_junit(self, path: str, suite: str = "meshbench") -> None:
        """Write a JUnit file, with the caveats inside it.

        In the file rather than only on stdout, because the file is what a CI
        system keeps and shows six months later - and a delivery figure with no
        note of what the model assumed is exactly the number this project
        exists not to publish.
        """
        root = ET.Element(
            "testsuite",
            name=suite,
            tests=str(self.total),
            failures=str(len(self.failures)),
        )
        if self.provenance is not None:
            props = ET.SubElement(root, "properties")
            ET.SubElement(
                props,
                "property",
                name="meshbench.provenance",
                value=str(self.provenance),
            )
        for c in self.checks:
            case = ET.SubElement(
                root,
                "testcase",
                classname=suite + ".assertions",
                name=c.kind + (f" at {c.node}" if c.node else ""),
            )
            if not c.passed:
                ET.SubElement(case, "failure", message=f"got {c.got}, want {c.want}")
        ET.ElementTree(root).write(path, encoding="utf-8", xml_declaration=True)


class Assertions:
    """What has to be true for a run to have passed. Live."""

    #: The kinds this build understands. One it does not is a failure, not a
    #: pass: a green run that checked nothing is the worst outcome available.
    KINDS = ("delivered", "deliveries", "unique_deliveries", "sent", "transmissions")

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def delivered(self, at_least: int, within: timedelta | None = None) -> None:
        """At least this many nodes received something."""
        self.add("delivered", at_least=at_least, within=within)

    def sent(
        self,
        node: str | Node = "",
        at_least: int = 0,
        at_most: int = 0,
        within: timedelta | None = None,
    ) -> None:
        """This node - or the whole mesh - transmitted within these bounds.

        at_most is the interesting one: it is how a relay-suppression change is
        held to not having made the mesh chattier.
        """
        self.add("sent", node=node, at_least=at_least, at_most=at_most, within=within)

    def add(
        self,
        kind: str,
        node: str | Node = "",
        at_least: int = 0,
        at_most: int = 0,
        max_pct: float = 0.0,
        within: timedelta | None = None,
    ) -> int:
        """The general form, for a kind this package has no name for yet."""
        params: dict[str, object] = {"kind": kind}
        if node:
            params["node"] = str(node)
        if at_least:
            params["at_least"] = at_least
        if at_most:
            params["at_most"] = at_most
        if max_pct:
            params["max_pct"] = max_pct
        if within is not None:
            params["within_ms"] = int(as_seconds(within) * 1000)
        got = self._wb.call("assert.add", params) or {}
        return got.get("assertions", 0)

    def check(self) -> Report:
        """Measure every assertion against the run so far.

        The provenance travels with the verdict, because a delivery figure
        without what the model assumed is the number this project exists not to
        publish.
        """
        got = self._wb.call("assert.check") or {}
        checks = [
            Check(
                kind=str(r.get("kind", "")),
                node=str(r.get("node", "")),
                passed=bool(r.get("pass")),
                got=str(r.get("got", "")),
                want=str(r.get("want", "")),
            )
            for r in got.get("results") or []
        ]
        return Report(
            passed=got.get("passed", 0),
            total=got.get("total", 0),
            checks=checks,
            provenance=self._wb.provenance(),
        )

    def __len__(self) -> int:
        return self._wb.snapshot().get("assertions", 0)
