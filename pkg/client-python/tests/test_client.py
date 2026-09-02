"""The client against a real workbench, not a stub.

A stub would answer whatever this file said it should, which is worth nothing:
the entire risk in a client is disagreeing with the thing it drives. So every
test here starts a headless process, drives it over a socket of its own, and
stops it.

Needs a meshbench binary. MESHBENCH_BINARY names one; otherwise PATH.
"""

from __future__ import annotations

import contextlib
import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from datetime import timedelta

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
    found = shutil.which("meshbench")
    if found:
        return found
    if not shutil.which("go"):
        pytest.skip("no meshbench binary and no Go to build one")
    out = os.path.join(tempfile.mkdtemp(prefix="meshbench-bin"), "meshbench")
    build = subprocess.run(
        ["go", "build", "-o", out, "./cmd/meshbench"],
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


def test_a_client_speaking_another_protocol_is_refused(wb):
    """A wire version this build cannot speak is refused before the verb runs.

    Raw on the unix socket, because this client declares the version it does
    speak and the case worth testing is a client that does not - an older
    script, or a wheel installed beside a newer workbench. What it must not get
    is the verb failing, which is what sent people looking at the simulation.
    """
    if not hasattr(socket, "AF_UNIX") or not wb.hello.socket.startswith("/"):
        pytest.skip("no unix socket to speak raw on")
    s = socket.socket(socket.AF_UNIX)
    s.settimeout(10.0)
    s.connect(wb.hello.socket)
    with contextlib.closing(s):
        frame = {
            "id": 3,
            "method": "session.hello",
            "protocol": meshbench.PROTOCOL + 1,
        }
        s.sendall((json.dumps(frame) + "\n").encode())
        reply = json.loads(s.makefile("rb").readline().decode())

    assert reply.get("code") == "protocol_mismatch", reply
    said = reply.get("error", "")
    assert str(meshbench.PROTOCOL + 1) in said
    assert str(meshbench.PROTOCOL) in said
    assert "Upgrade" in said


def test_this_client_declares_its_protocol_first(tmp_path):
    """The client half of the same mechanism.

    A workbench can only refuse a version it was told about, and a real one
    never echoes the declaration back - so what this client puts on the wire is
    the one thing here that a socket of our own has to show us. Once, on the
    first frame: the answer cannot change while the connection is open.
    """
    if not hasattr(socket, "AF_UNIX"):
        pytest.skip("no unix socket on this platform")
    from meshbench._socket import Connection

    path = str(tmp_path / "fake.sock")
    seen = _record_frames(path)
    with contextlib.closing(Connection(path)) as conn:
        conn.call("session.hello")
        conn.call("session.verbs")

    assert seen[0]["protocol"] == meshbench.PROTOCOL
    assert "protocol" not in seen[1]


def test_a_version_refusal_keeps_the_workbenchs_own_words():
    """When the workbench is the end that notices, its sentence is the one
    raised: it knows its build and which end is the older one, and a paraphrase
    would lose both."""
    said = (
        "this client speaks control protocol 1 and this workbench (v9.9.9) "
        "speaks 2. Upgrade this client to the one that ships with v9.9.9"
    )
    e = meshbench.ProtocolMismatch(1, 0, said=said)
    assert str(e) == said
    assert e.client == 1
    # And the mismatch this client notices itself still says both numbers.
    own = meshbench.ProtocolMismatch(1, 2, "v9.9.9", "/run/mb.sock")
    assert "1" in str(own) and "2" in str(own) and "Upgrade" in str(own)


RELEASED = "9.9.9"


@pytest.fixture(scope="session")
def released_binary() -> str:
    """A workbench that believes it is a release, built once for the whole run.

    The rule this exercises only applies between two release builds, and there
    is no other way to make one: the release is a linker flag, so a checkout
    cannot produce one by accident and a test cannot fake it after the fact.
    """
    if not shutil.which("go"):
        pytest.skip("no Go to build a stamped workbench with")
    out = os.path.join(tempfile.mkdtemp(prefix="meshbench-release"), "meshbench")
    stamped = "github.com/MeshBench/meshbench/internal/app/version.Version"
    stamp = f"-X {stamped}=v{RELEASED}"
    build = subprocess.run(
        ["go", "build", "-ldflags", stamp, "-o", out, "./cmd/meshbench"],
        cwd=REPO,
        capture_output=True,
        text=True,
        check=False,
    )
    if build.returncode != 0:
        pytest.skip(f"could not build a stamped workbench: {build.stderr}")
    return out


def test_a_client_from_another_release_is_refused(released_binary, tmp_path):
    """A client and the workbench it drives must be the same release.

    Raw on the socket, because the point is that the workbench refuses rather
    than that the client is polite: a third-party script speaking this wire
    gets the same answer as one using a shipped client.
    """
    if not hasattr(socket, "AF_UNIX"):
        pytest.skip("no unix socket on this platform")
    path = str(tmp_path / "control.sock")
    proc = subprocess.Popen(
        [released_binary, "headless", "-control-socket", path],
        stderr=subprocess.DEVNULL,
    )
    try:
        _wait_for_socket(path)
        s = socket.socket(socket.AF_UNIX)
        s.settimeout(10.0)
        s.connect(path)
        with contextlib.closing(s):
            frame = {
                "id": 3,
                "method": "session.hello",
                "protocol": meshbench.PROTOCOL,
                "release": "1.5.0",
            }
            s.sendall((json.dumps(frame) + "\n").encode())
            reply = json.loads(s.makefile("rb").readline().decode())
    finally:
        proc.kill()
        proc.wait()

    assert reply.get("code") == "version_mismatch", reply
    said = reply.get("error", "")
    # Both releases and the remedy: a bare "version mismatch" leaves a reader
    # to work out which of the two things they have installed to change.
    assert "1.5.0" in said and RELEASED in said
    assert "must be the same release" in said


def test_this_client_refuses_a_workbench_from_another_release(
    released_binary, tmp_path
):
    """And the shipped client reports it as the mismatch it is, at connect,
    rather than as session.hello failing forty calls before anybody looks."""
    with pytest.raises(meshbench.VersionMismatch) as e:
        Workbench.headless(
            binary=released_binary,
            socket=str(tmp_path / "control.sock"),
            stderr=subprocess.DEVNULL,
        )
    said = str(e.value)
    assert meshbench.release() in said and RELEASED in said
    assert "must be the same release" in said


def test_this_client_declares_its_release_first(tmp_path):
    """A workbench can only refuse a pair it was told about, so the release
    goes on the first frame of every connection and on no other."""
    if not hasattr(socket, "AF_UNIX"):
        pytest.skip("no unix socket on this platform")
    from meshbench._socket import Connection

    path = str(tmp_path / "fake.sock")
    seen = _record_frames(path)
    with contextlib.closing(Connection(path)) as conn:
        conn.call("session.hello")
        conn.call("session.verbs")

    assert seen[0]["release"] == meshbench.release()
    assert "release" not in seen[1]


def test_a_development_workbench_is_served_and_says_the_check_was_skipped(wb):
    """A build from a working copy stamps no release, and refusing it would
    make the tree unusable by the people changing it. The skip is reported
    rather than silent, so a pair nothing verified does not read as one that
    was checked and matched."""
    assert wb.hello.release == ""
    assert "skipped" in wb.version_check
    assert "development build" in wb.version_check


def test_the_pairing_rule_is_exact_match_or_an_unstamped_end():
    assert meshbench.paired_release("1.0.0", "1.0.0")
    assert not meshbench.paired_release("1.0.0", "1.0.1")
    assert not meshbench.paired_release("2.0.0", "1.0.0")
    # An end that names no release has no second version to disagree with.
    assert meshbench.paired_release("", "1.0.0")
    assert meshbench.paired_release("1.0.0", "")
    assert meshbench.paired_release("", "")
    # A check that compared nothing says so; one that compared says nothing.
    assert meshbench.pairing_note("1.0.0", "1.0.0") == ""
    assert "skipped" in meshbench.pairing_note("1.0.0", "")


def test_a_release_refusal_keeps_the_workbenchs_own_words():
    said = (
        "this client is from MeshBench 1.5.0 and this workbench is MeshBench "
        "2.0.0. A client and the workbench it drives must be the same release"
    )
    e = meshbench.VersionMismatch("1.5.0", "", said=said)
    assert str(e) == said
    # And the mismatch this client notices itself names both releases.
    own = meshbench.VersionMismatch("1.5.0", "2.0.0")
    assert "1.5.0" in str(own) and "2.0.0" in str(own)


def _wait_for_socket(path: str, timeout: float = 30.0) -> None:
    """Wait for a workbench started here to be answering."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if os.path.exists(path):
            s = socket.socket(socket.AF_UNIX)
            try:
                s.connect(path)
                return
            except OSError:
                pass
            finally:
                s.close()
        time.sleep(0.05)
    raise AssertionError(f"nothing answered at {path} within {timeout}s")


def _record_frames(path: str) -> list[dict]:
    """A socket of our own that answers everything and keeps what it was sent.

    A real workbench never echoes a declaration back, so what this client puts
    on the wire is the one thing here only a fake can show us.
    """
    listener = socket.socket(socket.AF_UNIX)
    listener.bind(path)
    listener.listen(1)
    seen: list[dict] = []

    def serve() -> None:
        conn, _ = listener.accept()
        with contextlib.closing(conn):
            f = conn.makefile("rb")
            while True:
                line = f.readline()
                if not line:
                    return
                req = json.loads(line.decode())
                seen.append(req)
                conn.sendall(
                    (json.dumps({"id": req["id"], "result": {}}) + "\n").encode()
                )

    threading.Thread(target=serve, daemon=True).start()
    return seen


def test_building_a_network_from_nothing(wb):
    wb.project.new()
    wb.nodes.place_many(
        [
            {
                "name": "R1",
                "kind": meshbench.Kind.SIMPLE_REPEATER,
                "lat": 56.20,
                "lon": -3.20,
            },
            {
                "name": "R2",
                "kind": meshbench.Kind.SIMPLE_REPEATER,
                "lat": 56.12,
                "lon": -3.02,
            },
            {
                "name": "C1",
                "kind": meshbench.Kind.COMPANION,
                "lat": 56.19,
                "lon": -3.17,
            },
        ]
    )
    assert len(wb.nodes) == 3
    assert "C1" in wb.nodes
    # Read back, not just counted: a client that silently dropped a parameter
    # would still have produced three nodes.
    c1 = wb.nodes.info("C1")
    assert c1.kind == meshbench.Kind.COMPANION
    assert 56.18 < c1.lat < 56.20

    wb.nodes.delete("R2")
    assert len(wb.nodes) == 2


def test_placing_a_node_on_a_board(wb):
    """#216: nodes.place took no board, so a script could build a mesh and not
    build the one it wanted."""
    wb.project.new()
    wb.nodes.place(
        "Deck",
        meshbench.Kind.COMPANION,
        56.19,
        -3.17,
        board=meshbench.Board.LILYGO_TDECK,
    )
    assert wb.nodes["Deck"].board == meshbench.Board.LILYGO_TDECK

    # A board is physics - the transmit ceiling, the noise figure, the
    # battery - so a name nothing matches refuses rather than falling back.
    with pytest.raises(meshbench.BadParams):
        wb.nodes.place("Wrong", lat=56, lon=-3, board="LilyGo T-Deck Pro Max")


def test_changing_what_a_node_is(wb):
    wb.project.new()
    node = wb.nodes.place("Deck", meshbench.Kind.COMPANION, 56.19, -3.17)
    node.board = meshbench.Board.LILYGO_TDECK
    assert node.board == meshbench.Board.LILYGO_TDECK


def test_every_generated_board_is_one_the_workbench_knows(wb):
    """A constant that compiles and is not a board the simulator knows is worse
    than a string, because it looks checked."""
    wb.project.new()
    assert len(meshbench.Board) > 0
    for i, board in enumerate(meshbench.Board):
        wb.nodes.place(
            f"n{i}", lat=56 + i / 1000, lon=-3, board=board
        )  # refuses if the workbench disagrees


def test_every_generated_kind_is_one_the_workbench_knows(wb):
    wb.project.new()
    for i, kind in enumerate(meshbench.Kind):
        wb.nodes.place(kind.value, kind, 56 + i / 1000, -3)


def test_scheduling_and_asserting(wb):
    """Through the shape rather than through call().

    The verb takes milliseconds; a caller says twenty seconds. That difference
    is the entire reason this layer exists.
    """
    # A blank network rather than a fixture: the shipped ones carry their own
    # assertions, and counting mine among theirs would prove nothing.
    wb.project.new()
    wb.nodes.place("R1", lat=56.2, lon=-3.2)

    wb.schedule.add(
        "R1",
        "send hello",
        at=timedelta(seconds=5),
        every=timedelta(seconds=20),
    )
    assert len(wb.schedule) == 1

    wb.assertions.delivered(at_least=1)
    report = wb.assertions.check()
    assert report.total == 1
    # Nothing has run, so it has not been met - and the report says which one.
    assert not report.ok
    assert len(report.failures) == 1
    assert report.failures[0].kind
    # And the caveats travel with the verdict.
    assert "best case" in str(report)


def test_no_assertions_is_not_a_pass():
    """A green tick that checked nothing is the worst outcome available."""
    empty = meshbench.Report()
    assert not empty.ok
    assert "checked nothing" in str(empty)


def test_junit_carries_the_provenance(tmp_path):
    report = meshbench.Report(
        passed=1,
        total=2,
        checks=[
            meshbench.Check("delivered", "", True, "40", "at least 40"),
            meshbench.Check("sent", "R1", False, "99", "at most 12"),
        ],
        provenance=meshbench.Provenance(rf_mode="waveform", seed=9001),
    )
    path = tmp_path / "results.xml"
    report.write_junit(str(path))
    body = path.read_text()
    for want in ("best case", "waveform", "at most 12", 'failures="1"'):
        assert want in body


def test_a_bad_name_deletes_nothing(wb):
    """Half a deletion leaves a scenario nobody described."""
    wb.project.new()
    for name in ("A", "B", "C"):
        wb.nodes.place(name, lat=56.2, lon=-3.2)
    with pytest.raises(meshbench.NotFound):
        wb.nodes.delete("A", "Nowhere")
    assert len(wb.nodes) == 3


def test_a_window_verb_refuses_headless(wb):
    """It says so at the client, not after twelve refusals in a row."""
    with pytest.raises(meshbench.Unavailable):
        wb.window("anything", tab="Hardware")


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
    wb.sim.run(timedelta(seconds=2), wait=timedelta(minutes=2))
    after = wb.sim.state()
    assert not after.playing, "run() returned while the clock was still going"
    assert after.now_ms - before.now_ms >= 2000


def test_the_same_seed_reaches_the_same_state(binary, tmp_path):
    """Determinism is a feature, and the client must not be what breaks it."""

    # What a seed promises is the same simulation, not the same machine.
    # warming, links_measured and ground say how much terrain has
    # arrived in this cache, so two runs either side of a fetch disagree on
    # them while every event matches: this failed on tiles_cached 0 against
    # 196 with the clock and the event count identical. The Go client's
    # TestTheSameSeedReachesTheSameState compares these three and has never
    # had the problem; the two clients should mean the same thing by it.
    decided_by_the_seed = ("now_ms", "events", "seed")

    def once(n: int):
        w = Workbench.headless(
            binary=binary,
            socket=str(tmp_path / f"seed{n}.sock"),
            fixture="fife-strict",
            seed=4242,
            stderr=subprocess.DEVNULL,
        )
        try:
            w.sim.run(timedelta(seconds=3), wait=timedelta(minutes=2))
            state = w.sim.state()
            return {f: getattr(state, f) for f in decided_by_the_seed}
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
        node.wait_running(timeout=timedelta(seconds=0.6))
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


def test_finding_a_node_whose_name_you_cannot_type(wb):
    """The case the ScotMesh example is built on.

    Real imported names carry emoji either side and sometimes an accent, so
    every script that wanted one node was reduced to fetching all of them and
    matching by hand.
    """
    wb.project.new()
    wb.nodes.place_many(
        [
            {"name": "\U0001f3d4️ West Lomond \U0001f4e1", "lat": 56.24, "lon": -3.29},
            {"name": "West Lomond Relay Two", "lat": 56.25, "lon": -3.28},
            {"name": "Beinn Àrd ⛰", "lat": 56.30, "lon": -3.40},
            {"name": "\U0001f4fb Dunfermline Repeater", "lat": 56.07, "lon": -3.46},
        ]
    )

    hits = wb.nodes.search("west lomond")
    assert hits, "the emoji name was not found at all"
    # The exact name beats the one that merely starts the same way. A caller
    # taking the top result is taking this.
    assert hits[0].name == "\U0001f3d4️ West Lomond \U0001f4e1"
    assert hits[0].score > hits[1].score

    # Accents fold, so the Gaelic name is reachable from an ASCII keyboard.
    assert wb.nodes.find("beinn ard").name == "Beinn Àrd ⛰"

    # find() refuses rather than handing back the nearest thing, and says what
    # it did find - which is the difference between a typo and an absence.
    with pytest.raises(meshbench.NotFound) as e:
        wb.nodes.find("Ben Nevis")
    assert "Ben Nevis" in str(e.value)

    # The handle works: a name found this way is a name every other verb takes.
    assert wb.nodes.find("dunfermline").info.lat < 56.1


@pytest.fixture
def imported(wb, tmp_path):
    """Import builds, and take them back out however the test ends.

    These land in the machine's real firmware cache - the verb uses it and
    nothing overrides it - so a test that fails half way through would leave a
    build behind in somebody's library. Deleting on the way out regardless is
    the difference between a failing test and a failing test plus a mess.
    """
    # Not arbitrary bytes: an import for a board is checked against what the
    # ROM bootloader needs to find - an image header where the part boots from,
    # and a partition table at 0x8000. A published release carries the whole
    # flash and the application on its own under names differing by one word,
    # and only one of them boots, so a placeholder here would be testing the
    # labels against a file no board could start.
    flash = bytearray(b"\xff" * 0x9000)
    flash[0] = 0xE9  # an image header, where an ESP32-S3 boots from
    flash[3] = 1 << 4  # declaring 2 MB of flash
    flash[0x8000] = 0xAA  # and a partition table where the bootloader reads one
    flash[0x8001] = 0x50
    image = tmp_path / "firmware.bin"
    image.write_bytes(bytes(flash))
    made = []

    def do(role="companion_radio", board="LilyGo_TDeck", label=""):
        b = wb.firmware.import_(str(image), role, board=board, label=label)
        made.append(b)
        return b

    yield do
    for b in made:
        # Already gone is fine: deleting one is what a test here does.
        with contextlib.suppress(meshbench.MeshbenchError):
            wb.firmware.delete(b)


def test_two_imports_of_one_file_are_two_builds(wb, imported):
    """Labelled imports, which is what makes replacing a build possible.

    Every import used to be called "imported" and land in a directory nothing
    lists, so a second one replaced the first in place: the library showed one
    entry, and there was no way to say which of two local builds a node was on
    or to delete the older.
    """
    first = imported(label="wadamesh-a")
    second = imported(label="wadamesh-b")
    assert first.version == "wadamesh-a"
    assert second.version == "wadamesh-b"

    # Both in the library, and both on disk - the part that was broken. The
    # library reported a count and kept the rows where only a panel could
    # reach them, so this list was an integer.
    on_disk = {b.version for b in wb.firmware.on_disk()}
    assert {"wadamesh-a", "wadamesh-b"} <= on_disk

    # And the older one can be deleted by itself, which is the point.
    wb.firmware.delete(first)
    assert "wadamesh-a" not in {b.version for b in wb.firmware.on_disk()}
    assert "wadamesh-b" in {b.version for b in wb.firmware.on_disk()}


def test_a_default_import_label_is_a_timestamp(wb, imported):
    """No label still gives a distinguishable build, rather than a collision."""
    got = imported(board="Heltec_v3")
    assert got.version.startswith("imported-")
    assert got.version in {b.version for b in wb.firmware.on_disk()}


def test_the_import_chain_refuses_in_the_wrong_order(wb):
    """Each step needs the one before it, and says so rather than doing half.

    The chain is where scripted imports go wrong, and the failures are quiet:
    committing nothing, or inferring against no source, used to be the kind of
    thing that produced an empty mesh and no error.
    """
    with pytest.raises(meshbench.Refused):
        wb.live.commit()  # nothing fetched
    with pytest.raises(meshbench.Refused):
        wb.live.infer()  # no source
    with pytest.raises(meshbench.Refused):
        wb.live.apply_regions()  # nothing inferred

    # A pasted trailing slash is tidied at the workbench rather than turning
    # into a double slash in every request it then makes.
    assert wb.live.set_source("http://127.0.0.1:9/feed/") == "http://127.0.0.1:9/feed"

    # A source that cannot be reached is an error, not a hang and not silence.
    with pytest.raises(meshbench.Refused):
        wb.live.fetch()


def test_infer_result_is_not_a_verb_to_call(wb):
    """It is the reading goroutine's own callback.

    It used to be reachable from the socket like everything else, and the
    version that ignored that answered by replacing a finished inference with
    an empty one - so a mesh that had just been imported correctly went silent,
    and nothing said why. The socket refuses the workbench's own callbacks now,
    and this is what a client sees when it names one.
    """
    with pytest.raises(meshbench.BadParams):
        wb.call("infer.result")


# A square over Fife, as a FeatureCollection with a named feature - the shape
# anything that exports a drawn polygon produces.
FIFE_ISH = {
    "type": "FeatureCollection",
    "features": [
        {
            "type": "Feature",
            "properties": {"name": "Test square"},
            "geometry": {
                "type": "Polygon",
                "coordinates": [
                    [
                        [-3.4, 56.0],
                        [-2.8, 56.0],
                        [-2.8, 56.4],
                        [-3.4, 56.4],
                        [-3.4, 56.0],
                    ]
                ],
            },
        }
    ],
}


def test_a_study_area_from_geojson(wb, tmp_path):
    """Your own polygon, rather than one the gazetteer has a name for.

    boundary.set searches Nominatim, so it needs the network and needs the area
    to have an administrative name. A catchment, a valley or a polygon drawn
    this morning has neither, and the GeoJSON parser this uses has been in the
    tree the whole time with nothing outside the process able to reach it.
    """
    assert wb.boundary.list() == []

    # As a dict, as a document, and as a file: all three are things a caller
    # actually has.
    assert wb.boundary.load(FIFE_ISH) == ["Test square"]
    assert wb.boundary.list() == ["Test square"]

    path = tmp_path / "tay-catchment.geojson"
    path.write_text(json.dumps(FIFE_ISH["features"][0]["geometry"]))
    # A bare geometry carries no name, so the file's own name is used - which
    # is what somebody who named it "tay-catchment.geojson" already told us.
    assert wb.boundary.use(str(path)) == ["tay-catchment"]
    assert set(wb.boundary.list()) == {"Test square", "tay-catchment"}

    wb.boundary.remove("tay-catchment")
    assert wb.boundary.list() == ["Test square"]


def test_a_study_area_prunes_what_is_outside(wb):
    """What the boundary is for, on a mesh that was loaded before it was set."""
    wb.project.new()
    wb.nodes.place_many(
        [
            {"name": "Inside", "lat": 56.2, "lon": -3.1},
            {"name": "Outside", "lat": 57.5, "lon": -4.2},
        ]
    )
    wb.boundary.load(FIFE_ISH)

    # No margin, or the node 150 km away is kept on the grounds that it might
    # interfere - which is the right default and the wrong thing to assert on.
    assert wb.boundary.prune(margin_km=0) == 1
    assert [n.name for n in wb.nodes.list()] == ["Inside"]


def test_geojson_that_is_not_geojson_says_so(wb, tmp_path):
    """A refusal that names the problem, rather than an empty study area."""
    with pytest.raises(meshbench.BadParams):
        wb.boundary.load('{"type": "Point", "coordinates": [0, 0]}')
    with pytest.raises(meshbench.MeshbenchError):
        wb.boundary.load(str(tmp_path / "nothing-here.geojson"))


def test_attach_or_start_reuses_the_session_it_started(binary, tmp_path, monkeypatch):
    """The whole point of the pair, and it did not work.

    headless() and launch() invent a private address when none is named, so
    that two of them do not fight over the per-user default. attach_or_ was
    inheriting that: every run failed to attach, started a session somewhere
    nobody would look again, and the next run did the same - so a script
    written to be re-run built a second workbench each time and appeared to
    lose its scenario.

    The environment names the address, so this exercises the default path
    without touching the one the operator's own workbench is on.
    """
    monkeypatch.setenv(meshbench.SOCKET_ENV, str(tmp_path / "shared.sock"))
    monkeypatch.setenv("MESHBENCH_BINARY", binary)

    first = Workbench.attach_or_headless(binary=binary, stderr=subprocess.DEVNULL)
    try:
        assert first.owns_process, (
            "nothing was listening, so it should have started one"
        )
        first.project.new()
        first.nodes.place("Marker", lat=56.0, lon=-3.0)

        second = Workbench.attach_or_headless(binary=binary, stderr=subprocess.DEVNULL)
        try:
            # The same session, which is the claim: it found the scenario the
            # first one built rather than starting an empty one of its own.
            assert not second.owns_process, "it started a second session"
            assert second.hello.pid == first.hello.pid
            assert "Marker" in second.nodes
        finally:
            second.close()
    finally:
        first.close()


def test_subscribe_is_told_when_the_log_changes(wb):
    """#214: a subscribed connection is pushed changes rather than polling.

    ui.said appends a log line, which the store announces on the "status" topic;
    a subscription opened beforehand must see it. The stream runs on a thread
    because iterating it blocks until the next event - which is the point."""
    import threading
    import time

    seen: list[meshbench.Notification] = []
    sub = wb.subscribe("status")

    def drain() -> None:
        for note in sub:
            seen.append(note)

    reader = threading.Thread(target=drain, daemon=True)
    reader.start()

    wb.say("marker-6f3a")

    def landed() -> bool:
        return any("marker-6f3a" in json.dumps(n.data) for n in seen)

    deadline = time.time() + 5
    while time.time() < deadline and not landed():
        time.sleep(0.05)
    sub.close()

    assert landed(), f"no status notification carried the line; got {seen}"
    status = next(n for n in seen if "marker-6f3a" in json.dumps(n.data))
    assert status.topic == "status"


def test_a_plain_client_is_untouched_by_notifications(wb):
    """Backward compatibility: a client that never subscribes still reads one
    reply per call, even while another connection is being pushed events."""
    sub = wb.subscribe("status")
    try:
        wb.say("some-line")  # fans out to the subscriber, not to wb's own conn
        # wb's own request/reply must still line up perfectly.
        assert wb.describe().get("nodes") is not None
        assert wb.verbs()
    finally:
        sub.close()


def test_board_api_refuses_a_node_that_is_not_running(wb):
    """#257: the board API - screen, screenshot, buttons, touch, radio - drives
    a running board. Booting an emulated one is too slow and flaky for this
    suite; what is checked here is the client layer, that each method reaches
    its verb and a refusal on a stopped node comes back as an error rather than
    a zero value read as success."""
    wb.project.new()
    wb.nodes.place("R1", meshbench.Kind.SIMPLE_REPEATER, 56.20, -3.20)
    n = wb.nodes["R1"]
    d = n.device

    import pytest as _pytest

    with _pytest.raises(meshbench.MeshbenchError):
        d.screen()
    with _pytest.raises(meshbench.MeshbenchError):
        d.screenshot()
    with _pytest.raises(meshbench.MeshbenchError):
        d.press(0)
    with _pytest.raises(meshbench.MeshbenchError):
        d.type("x")
    with _pytest.raises(meshbench.MeshbenchError):
        d.tap_at(10, 10)
    with _pytest.raises(meshbench.MeshbenchError):
        n.radio()


def test_checkpoint_round_trips(wb):
    """#207: a checkpoint is only worth anything if what comes back is what went
    in. Build a network, move the clock, freeze it, throw the session away,
    restore, and the network and the moment are both back."""
    wb.project.new()
    wb.nodes.place_many(
        [
            {
                "name": "R1",
                "kind": meshbench.Kind.SIMPLE_REPEATER,
                "lat": 56.20,
                "lon": -3.20,
            },
            {
                "name": "R2",
                "kind": meshbench.Kind.SIMPLE_REPEATER,
                "lat": 56.12,
                "lon": -3.02,
            },
        ]
    )
    wb.call("sim.settle", {"steps": 10})
    now = wb.call("sim.state")["now_ms"]
    assert now > 0

    cp = wb.checkpoint("py-trip")
    assert cp["nodes"] == 2
    assert cp["now_ms"] == now
    assert "py-trip" in wb.checkpoints()

    wb.project.new()
    assert len(wb.nodes) == 0

    r = wb.restore("py-trip")
    assert r["nodes"] == 2
    assert r["target_ms"] == now
    assert r["replaying"] is True
    assert "R1" in wb.nodes and "R2" in wb.nodes


@pytest.fixture
def two_workbenches(binary, tmp_path, monkeypatch):
    """Two sessions at once, on a registry of their own.

    Its own registry because these tests delete what they find dead in it, and
    the machine running them may have somebody's workbench open.
    """
    monkeypatch.setenv(meshbench.SESSIONS_ENV, str(tmp_path / "sessions"))
    quiet = (
        subprocess.DEVNULL if not os.environ.get("MESHBENCH_VERBOSE") else sys.stderr
    )
    started = [
        Workbench.headless(
            binary=binary, socket=str(tmp_path / f"{name}.sock"), stderr=quiet
        )
        for name in ("a", "b")
    ]
    yield started
    for w in started:
        with contextlib.suppress(Exception):
            w.close()


def test_two_running_workbenches_can_be_told_apart(two_workbenches, tmp_path):
    a, b = two_workbenches
    rows = meshbench.sessions()
    by = {r.address: r for r in rows}
    assert set(by) == {str(tmp_path / "a.sock"), str(tmp_path / "b.sock")}, rows

    for row in rows:
        # Everything a script needs in order to choose one: where, which
        # process, when it started, and what it is running.
        assert row.pid > 0
        assert row.started_at
        assert row.version
        assert row.mode == "headless"
        assert row.windowed is False
    assert by[a.hello.socket].pid == a.hello.pid
    assert by[b.hello.socket].pid == b.hello.pid
    assert a.hello.pid != b.hello.pid


def test_a_row_from_the_listing_can_be_attached_to(two_workbenches):
    a, _ = two_workbenches
    row = next(r for r in meshbench.sessions() if r.address == a.hello.socket)
    with Workbench.attach(row) as also_a:
        assert also_a.hello.pid == a.hello.pid


def test_a_session_lists_the_others_and_marks_its_own_row(two_workbenches):
    a, b = two_workbenches
    rows = a.sessions()
    assert {r.address for r in rows} == {a.hello.socket, b.hello.socket}
    mine = [r for r in rows if r.is_self]
    assert [r.address for r in mine] == [a.hello.socket]
    # Its own row is described from the inside; the other one by asking it.
    assert mine[0].version == a.hello.version
    assert next(r for r in rows if not r.is_self).version == b.hello.version
    # A token is not a thing a reply carries.
    assert all(r.token == "" for r in rows)


def test_a_killed_workbench_is_not_reported_as_running(two_workbenches):
    """The one that matters. SIGKILL leaves the socket file and the row behind
    and gives the process no chance to tidy either up, so anything trusting
    what is on disk would report a dead session as running."""
    if sys.platform == "win32":
        pytest.skip("no SIGKILL on Windows")
    a, b = two_workbenches
    dead = b.hello.socket
    os.kill(b.hello.pid, signal.SIGKILL)
    b._process.wait(timeout=30)
    # The socket it bound is still on disk, which is exactly why a stat would
    # not do.
    assert os.path.exists(dead)

    rows = meshbench.sessions()
    assert [r.address for r in rows] == [a.hello.socket], rows
    assert [r.address for r in a.sessions()] == [a.hello.socket]
