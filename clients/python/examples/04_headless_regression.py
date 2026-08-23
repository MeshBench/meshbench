#!/usr/bin/env python3
"""Example 4: the one CI runs.

    ./04_headless_regression.py [fixture] [junit.xml]

No display, no GPU, no toolkit. Opens a fixture, runs it, checks its
assertions, writes JUnit, and exits non-zero if the mesh stopped delivering.
This is the shape a MeshCore pull request would use.

Costs: as long as the fixture asks for. fife-strict at five simulated minutes
is a couple of minutes of wall clock with no firmware, and considerably more
with it.
"""

import sys
import xml.etree.ElementTree as ET

from meshbench import Workbench

SEED = 9001


def write_junit(path: str, results: list[dict], provenance: str) -> None:
    """A JUnit file, with the caveats inside it.

    In the file rather than only on stdout, because the file is what a CI
    system keeps and shows six months later - and a delivery figure with no
    note of what the model assumed is exactly the number this project exists
    not to publish.
    """
    suite = ET.Element(
        "testsuite",
        name="meshbench",
        tests=str(len(results)),
        failures=str(sum(1 for r in results if not r.get("pass"))),
    )
    ET.SubElement(suite, "properties").append(
        ET.Element("property", name="meshbench.provenance", value=provenance)
    )
    for r in results:
        case = ET.SubElement(
            suite, "testcase", classname="assertions", name=str(r.get("kind"))
        )
        if not r.get("pass"):
            ET.SubElement(
                case,
                "failure",
                message=f"got {r.get('got')}, want {r.get('want')}",
            )
    ET.ElementTree(suite).write(path, encoding="utf-8", xml_declaration=True)


def main() -> int:
    fixture = sys.argv[1] if len(sys.argv) > 1 else "fife-strict"
    junit = sys.argv[2] if len(sys.argv) > 2 else ""

    with Workbench.headless(fixture=fixture, seed=SEED) as wb:
        wb.sim.run(minutes=5, wait="60m")

        report = wb.call("assert.check") or {}
        results = report.get("results") or []
        passed, total = report.get("passed", 0), report.get("total", 0)
        provenance = str(wb.provenance())

        # The caveats above the numbers, always. Not a footnote: this is the
        # output somebody pastes into a pull request.
        print(provenance)
        print(f"{wb.events.total()} events, {passed} of {total} assertions passed")
        for r in results:
            if not r.get("pass"):
                print(
                    f"  FAILED {r.get('kind')}: got {r.get('got')}, "
                    f"want {r.get('want')}"
                )

        if junit:
            write_junit(junit, results, provenance)

        if total == 0:
            # Not a pass. A fixture with no assertions can report but cannot
            # pass or fail, and a green tick that checked nothing is the worst
            # outcome available here.
            print(f"{fixture} carries no assertions, so this checked nothing")
            return 2
        return 0 if passed == total else 1


if __name__ == "__main__":
    sys.exit(main())
