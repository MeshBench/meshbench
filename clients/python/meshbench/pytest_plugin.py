"""A pytest plugin, so a firmware regression test looks like a test.

    def test_the_flood_reaches_glenrothes(meshbench):
        meshbench.project.open("fixtures/fixture-fife-strict.json")
        meshbench.sim.run(timedelta(minutes=5))
        assert meshbench.events.total() > 0

Two fixtures, because starting real firmware on a mesh is minutes and a test
file is not two tests:

- ``meshbench_session`` is one workbench for the whole run, session-scoped.
- ``meshbench`` is that workbench with the scenario cleared between tests, so
  reuse does not leak state from one test into the next.

Registered as an entry point, so ``pip install meshbench`` is enough - there is
no conftest to copy, which is the thing people forget and then debug.
"""

from __future__ import annotations

import contextlib
import os

import pytest

from .workbench import Workbench


def pytest_addoption(parser: pytest.Parser) -> None:
    group = parser.getgroup("meshbench")
    group.addoption(
        "--meshbench-socket",
        default=os.environ.get("MESHBENCH_CONTROL_SOCKET"),
        help="attach to the workbench answering here instead of starting one",
    )
    group.addoption(
        "--meshbench-binary",
        default=os.environ.get("MESHBENCH_BINARY"),
        help="the meshcoresim executable to start",
    )
    group.addoption(
        "--meshbench-fixture",
        default=None,
        help="open this network when the session starts",
    )
    group.addoption(
        "--meshbench-seed",
        type=int,
        default=None,
        help="fix the seed, so a changed result means something",
    )


@pytest.fixture(scope="session")
def meshbench_session(request: pytest.FixtureRequest):
    """One workbench for the whole test run.

    Session-scoped on purpose. Starting firmware on a real mesh is minutes;
    doing it per test would make a suite of twenty an afternoon.

    With --meshbench-socket it attaches to one somebody else is running and
    does not stop it, which is how a developer runs their suite against the
    window they are watching.
    """
    socket = request.config.getoption("--meshbench-socket")
    if socket:
        wb = Workbench.attach(socket)
    else:
        wb = Workbench.headless(
            fixture=request.config.getoption("--meshbench-fixture"),
            seed=request.config.getoption("--meshbench-seed"),
            binary=request.config.getoption("--meshbench-binary"),
        )
    yield wb
    wb.close()


@pytest.fixture
def meshbench(meshbench_session, request: pytest.FixtureRequest):
    """The workbench, with the scenario cleared afterwards.

    Cleared after rather than before, so a failing test's network is still
    there to look at when the suite is run with -x against a live workbench.
    """
    yield meshbench_session
    try:
        meshbench_session.sim.pause()
        meshbench_session.project.new()
    except Exception:  # noqa: BLE001 - see below
        # A cleanup that raises turns one failing test into an error in every
        # test after it, which buries the one that actually broke. The
        # workbench being gone is the usual cause, and the tests that follow
        # will say so themselves.
        pass


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(item, call):
    """Put the provenance in the failure output, whether or not the test asked.

    This is the honesty rule, enforced where it matters most: a failing
    assertion about a mesh is about to be read by somebody deciding whether
    their firmware change broke something, and they need to know the run had no
    multipath, no body loss and no oscillator error before they conclude
    anything.
    """
    outcome = yield
    report = outcome.get_result()
    if report.when != "call" or not report.failed:
        return
    wb = item.funcargs.get("meshbench") or item.funcargs.get("meshbench_session")
    if wb is None:
        return
    # Best effort. A test that failed because the workbench died must not then
    # fail again inside the reporter, which would replace the assertion
    # somebody needs to read with a traceback from here.
    with contextlib.suppress(Exception):
        report.sections.append(("MeshBench", str(wb.provenance())))
