"""The client against a real workbench, not a stub.

A stub would answer whatever this file said it should, which is worth nothing:
the entire risk in a client is disagreeing with the thing it drives. So every
test here starts a headless process, drives it over a socket of its own, and
stops it.

Needs a meshcoresim binary. MESHBENCH_BINARY names one; otherwise PATH.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile

import pytest

import meshbench
from meshbench import Workbench

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))


@pytest.fixture(scope="session")
def binary() -> str:
    """Build the workbench once for the whole run.

    A few seconds, and it buys tests that fail when the verbs move - which is
    the only reason to have them.
    """
    named = os.environ.get("MESHBENCH_BINARY")
    if named:
        return named
    found = shutil.which("meshcoresim")
    if found:
        return found
    if not shutil.which("go"):
        pytest.skip("no meshcoresim binary and no Go to build one")
    out = os.path.join(tempfile.mkdtemp(prefix="meshbench-bin"), "meshcoresim")
    build = subprocess.run(
        ["go", "build", "-o", out, "./cmd/meshcoresim"],
        cwd=REPO,
        capture_output=True,
        text=True,
        check=False,
    )
    if build.returncode != 0:
        pytest.skip(f"could not build the workbench: {build.stderr}")
    return out


@pytest.fixture
def wb(binary, tmp_path):
    w = Workbench.headless(
        binary=binary,
        socket=str(tmp_path / "control.sock"),
        stderr=subprocess.DEVNULL
        if not os.environ.get("MESHBENCH_VERBOSE")
        else sys.stderr,
    )
    yield w
    w.close()


def test_it_connects_and_says_what_it_is(wb):
    assert wb.hello.mode == "headless"
    assert wb.is_headless
    assert wb.hello.protocol == meshbench.PROTOCOL
    # Enough to tell a restart from a reconnect, which is the whole reason
    # these two are in hello at all.
    assert wb.hello.pid and wb.hello.started_at
    assert wb.hello.verbs > 100


def test_building_a_network_from_nothing(wb):
    wb.project.new()
    wb.nodes.place_many(
        [
            {
                "name": "R1",
                "kind": meshbench.SIMPLE_REPEATER,
                "lat": 56.20,
                "lon": -3.20,
            },
            {
                "name": "R2",
                "kind": meshbench.SIMPLE_REPEATER,
                "lat": 56.12,
                "lon": -3.02,
            },
            {"name": "C1", "kind": meshbench.COMPANION, "lat": 56.19, "lon": -3.17},
        ]
    )
    assert len(wb.nodes) == 3
    assert "C1" in wb.nodes
    # Read back, not just counted: a client that silently dropped a parameter
    # would still have produced three nodes.
    c1 = wb.nodes.info("C1")
    assert c1.kind == meshbench.COMPANION
    assert 56.18 < c1.lat < 56.20

    wb.nodes.delete("R2")
    assert len(wb.nodes) == 2


def test_keep_deletes_the_complement(wb):
    wb.project.new()
    for name in ("A", "B", "C", "D"):
        wb.nodes.place(name, lat=56.2, lon=-3.2)
    wb.nodes.keep("B", "D")
    assert sorted(n.name for n in wb.nodes) == ["B", "D"]


def test_node_stats_carry_the_rows(wb):
    wb.project.open("fife-strict")
    stats = wb.node_stats()
    assert len(stats) == len(wb.nodes)
    # Nothing was started, and it says so as a state rather than only as a
    # boolean: "stopped" and "changing firmware" are not the same answer.
    assert stats[0].name
    assert not stats[0].running
    assert stats[0].state == "stopped"


def test_the_clock_advances_and_stops(wb):
    wb.project.open("fife-strict")
    before = wb.sim.state()
    assert not before.playing
    wb.sim.run(seconds_=2, wait="2m")
    after = wb.sim.state()
    assert not after.playing, "run() returned while the clock was still going"
    assert after.now_ms - before.now_ms >= 2000


def test_the_same_seed_reaches_the_same_state(binary, tmp_path):
    """Determinism is a feature, and the client must not be what breaks it."""

    def once(n: int):
        w = Workbench.headless(
            binary=binary,
            socket=str(tmp_path / f"seed{n}.sock"),
            fixture="fife-strict",
            seed=4242,
            stderr=subprocess.DEVNULL,
        )
        try:
            w.sim.run(seconds_=3, wait="2m")
            return w.sim.state()
        finally:
            w.close()

    assert once(1) == once(2)


def test_refusals_are_typed(wb):
    with pytest.raises(meshbench.UnknownVerb) as e:
        wb.call("no.such.verb")
    # And the workbench's own words survive, which is what a person reads.
    assert "no.such.verb" in str(e.value)

    with pytest.raises(meshbench.NotFound):
        wb.nodes.info("Nowhere")

    # A window verb in a session with no window: present, and refused.
    with pytest.raises(meshbench.Unavailable):
        wb.call("panel.open", {"name": "Map"})


def test_attach_does_not_own_the_process(wb):
    second = Workbench.attach(wb.hello.socket)
    assert not second.owns_process
    second.close()
    # The first is still there.
    assert wb.describe()["nodes"] >= 0


def test_the_loopback_transport(binary, tmp_path, monkeypatch):
    """The transport Windows uses, driven end to end from here.

    Windows has no unix socket CPython can open - socket.AF_UNIX has never
    existed there - so it gets loopback TCP with a token. That would otherwise
    be a path nobody on this project ever runs, which is the definition of code
    that is broken and nobody knows.
    """
    # Its own rendezvous file, the way a launched session gets one: two at once
    # would otherwise overwrite each other's port and token.
    monkeypatch.setenv(meshbench.RENDEZVOUS_ENV, str(tmp_path / "control.json"))

    w = Workbench.headless(binary=binary, socket="tcp", stderr=subprocess.DEVNULL)
    try:
        assert w.hello.socket.startswith("tcp:")
        # And it is a workbench, not merely a socket: the API works over it.
        w.project.new()
        w.nodes.place("A", lat=56, lon=-3)
        assert len(w.nodes) == 1
    finally:
        w.close()


def test_a_unix_path_too_long_says_so(tmp_path):
    """macOS temporary directories are long enough to exceed sun_path on their
    own, and the failure otherwise is a bind refusing with something about an
    invalid argument."""
    from meshbench._socket import MAX_UNIX_PATH, Connection

    long = str(tmp_path / ("x" * (MAX_UNIX_PATH + 10)) / "control.sock")
    with pytest.raises(ConnectionError) as e:
        Connection(long)
    assert "at most" in str(e.value)


def test_two_sessions_at_once(binary, tmp_path):
    """The case one socket per user made impossible."""
    a = Workbench.headless(
        binary=binary, socket=str(tmp_path / "a.sock"), stderr=subprocess.DEVNULL
    )
    b = Workbench.headless(
        binary=binary, socket=str(tmp_path / "b.sock"), stderr=subprocess.DEVNULL
    )
    try:
        assert a.hello.socket != b.hello.socket
        a.project.new()
        a.nodes.place("OnlyInA", lat=56, lon=-3)
        with pytest.raises(meshbench.NotFound):
            b.nodes.info("OnlyInA")
    finally:
        a.close()
        b.close()


def test_a_timeout_says_what_it_was_waiting_for(wb):
    wb.project.new()
    node = wb.nodes.place("Quiet", lat=56, lon=-3)
    with pytest.raises(meshbench.Timeout) as e:
        node.wait_running(timeout=0.6)
    assert "Quiet" in e.value.what
    assert e.value.last, "the timeout does not say what it last saw"


def test_provenance_says_what_the_numbers_were_measured_under(wb):
    wb.project.open("fife-strict")
    p = wb.provenance()
    assert p.rf_mode
    # The sentence is the point: it is what gets printed above a result.
    assert "best case" in str(p)


def test_the_escape_hatch_is_usable(wb):
    """Anything the shaped API has not reached is one line away."""
    got = wb.call("session.describe")
    assert "nodes" in got
