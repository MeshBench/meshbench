"""The connection, and everything hanging off it."""

from __future__ import annotations

import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from datetime import timedelta
from typing import Any

from . import errors
from ._socket import (
    BINARY_ENV,
    MAX_UNIX_PATH,
    PROTOCOL,
    RENDEZVOUS_ENV,
    Connection,
    default_address,
)
from .boundary import Boundary
from .checks import Assertions, Schedule
from .live import Live
from .nodes import Node, Nodes
from .parts import Console, Events, Firmware, Job, Project, Sim
from .sets import Tab
from .subscribe import Subscription
from .types import Hello, NodeStat, Provenance
from .wait import JOB_WAIT, wait_for


class Workbench:
    """A running session.

    Use it as a context manager. `launch` and `headless` own the process they
    started and stop it on the way out; `attach` never does - a script must not
    be able to close the workbench somebody is looking at by falling off the
    end of a `with`.
    """

    def __init__(self, conn: Connection, process: subprocess.Popen | None = None):
        self._conn = conn
        self._process = process
        self.hello = Hello()
        self._greet()

    # ---- connecting ------------------------------------------------------

    @classmethod
    def attach(cls, socket: str | None = None, timeout: float = 300.0) -> Workbench:
        """Connect to a workbench that is already running."""
        path = socket or default_address()
        try:
            conn = Connection(path, timeout=timeout)
        except OSError as e:
            raise errors.MeshbenchError(
                f"no workbench is listening at {path}: {e}"
            ) from e
        return cls(conn)

    @classmethod
    def headless(
        cls,
        fixture: str | None = None,
        seed: int | None = None,
        socket: str | None = None,
        binary: str | None = None,
        extra: list[str] | None = None,
        start_timeout: float = 90.0,
        stderr: Any = None,
    ) -> Workbench:
        """Start a session with no window, and own it.

        The one to use from a test or from CI: no display, no GPU, no toolkit.
        """
        return cls._spawn(
            "headless", fixture, seed, socket, binary, extra, start_timeout, stderr
        )

    @classmethod
    def launch(cls, **kw) -> Workbench:
        """Open the desktop workbench and own it. Needs a display."""
        return cls._spawn("workbench", **_launch_kw(kw))

    @classmethod
    def attach_or_headless(cls, **kw) -> Workbench:
        """Use the session that is running, or start one with no window.

        For a script somebody runs repeatedly by hand: the second run should
        carry on from the first rather than clearing everything down.

        Note which half you get. Attaching does not own the process and leaves
        it running at the end; starting one does own it and stops it. A script
        that wants the session to survive should attach to a session started
        separately.
        """
        return cls._attach_or(cls.headless, kw)

    @classmethod
    def attach_or_launch(cls, **kw) -> Workbench:
        """Use the session that is running, or open the desktop workbench.

        The windowed half of the pair, so a re-run can put something back on
        screen. Needs a display.
        """
        return cls._attach_or(cls.launch, kw)

    @classmethod
    def _attach_or(cls, start, kw: dict) -> Workbench:
        # The session that is started is started *at the address that was just
        # tried*, which is the whole point of the pair.
        #
        # headless() and launch() called directly invent a private address, so
        # that two of them - two pytest workers, two scripts - do not fight
        # over the per-user default. Inheriting that here made attach_or_
        # useless: every run failed to attach, started a session somewhere
        # nobody would look again, and the next run did the same. It read as
        # "reuse does not work" rather than as an address nobody had named.
        kw = dict(kw)
        kw["socket"] = kw.get("socket") or default_address()
        try:
            return cls.attach(kw["socket"])
        except errors.MeshbenchError:
            return start(**kw)

    @classmethod
    def _spawn(
        cls,
        command: str,
        fixture=None,
        seed=None,
        socket=None,
        binary=None,
        extra=None,
        start_timeout=90.0,
        stderr=None,
    ) -> Workbench:
        # An address of its own unless one was named, so two of these do not
        # fight over the per-user default.
        env = dict(os.environ)
        directory = tempfile.mkdtemp(prefix="meshbench")
        path = socket or os.path.join(directory, "control.sock")
        if not socket and (sys.platform == "win32" or len(path) > MAX_UNIX_PATH):
            # Two reasons a unix socket will not do. Windows has none this
            # Python can open, and a temporary directory on macOS is long
            # enough on its own to exceed sun_path. Either way, loopback.
            path = "tcp"
            # A rendezvous file of its own too, or two sessions would overwrite
            # each other's port and token in the per-user one.
            env[RENDEZVOUS_ENV] = os.path.join(directory, "control.json")
        exe = (
            binary
            or os.environ.get(BINARY_ENV)
            or shutil.which("meshbench")
            or "meshbench"
        )
        args = [exe, command, "-control-socket", path]
        if fixture:
            args += ["-fixture", fixture]
        if seed:
            args += ["-seed", str(seed)]
        args += extra or []

        try:
            proc = subprocess.Popen(args, stderr=stderr or sys.stderr, env=env)
        except OSError as e:
            raise errors.MeshbenchError(f"could not start {exe}: {e}") from e
        # Dialling has to look in the same rendezvous the process was told to
        # write, or it would find whatever else on this machine left one.
        if RENDEZVOUS_ENV in env:
            os.environ[RENDEZVOUS_ENV] = env[RENDEZVOUS_ENV]

        # Wait for the socket rather than for a fixed moment: a national
        # fixture takes a while to open and a small one does not, and a sleep
        # long enough for the first is wasted on every run of the second.
        deadline = time.monotonic() + start_timeout
        while True:
            if proc.poll() is not None:
                raise errors.MeshbenchError(
                    f"{exe} {command} exited with {proc.returncode} before "
                    f"answering at {path}"
                )
            try:
                conn = Connection(path)
                break
            except OSError:
                if time.monotonic() > deadline:
                    proc.kill()
                    raise errors.MeshbenchError(
                        f"{exe} {command} did not answer at {path} "
                        f"within {start_timeout:.0f}s"
                    ) from None
                time.sleep(0.05)
        try:
            wb = cls(conn, process=proc)
        except Exception:
            conn.close()
            proc.kill()
            raise
        if fixture:
            # The socket answers before the fixture is open. The windowed
            # build loads it on a worker so the window appears first, so a
            # client can connect, ask what is going on, and be told nothing -
            # an empty job list, no nodes, and a wait_idle that returns
            # instantly having waited for work that had not been queued yet.
            try:
                wb.wait_for_nodes(timedelta(seconds=start_timeout))
            except Exception:
                wb.close()
                raise
        return wb

    def _greet(self) -> None:
        """Ask what this is, and refuse a build this client cannot speak to."""
        try:
            reply = self.call("session.hello")
        except errors.Refused as e:
            if e.code != "protocol_mismatch":
                raise
            # The workbench refused the connection over the version this client
            # declared. Raised as the mismatch it is rather than as
            # session.hello failing, which is the confusion the declaration
            # exists to end.
            raise errors.ProtocolMismatch(PROTOCOL, 0, said=e.message) from None
        self.hello = Hello.parse(reply)
        if self.hello.protocol != PROTOCOL:
            raise errors.ProtocolMismatch(
                PROTOCOL, self.hello.protocol, self.hello.version, self.hello.socket
            )

    # ---- lifetime --------------------------------------------------------

    def close(self) -> None:
        """Hang up, and stop the process if this client started it."""
        try:
            self._conn.close()
        finally:
            if self._process is not None and self._process.poll() is None:
                # An interrupt asks the run to stop its firmware on the way
                # out. Windows cannot be sent SIGINT this way - it raises
                # ValueError, or signals the whole process group - so it gets
                # terminate() there and whatever the run was holding is the
                # operating system's problem.
                if sys.platform == "win32":
                    self._process.terminate()
                else:
                    self._process.send_signal(signal.SIGINT)
                try:
                    # Not instant on fifty-eight emulated nodes.
                    self._process.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    self._process.kill()

    def __enter__(self) -> Workbench:
        return self

    def __exit__(self, *exc) -> None:
        self.close()

    @property
    def owns_process(self) -> bool:
        """Whether closing this will stop the workbench."""
        return self._process is not None

    @property
    def is_headless(self) -> bool:
        return self.hello.headless

    # ---- the wire --------------------------------------------------------

    def call(self, verb: str, params: Any = None) -> Any:
        """Run one verb and return its result.

        Public and documented, not an escape hatch to be ashamed of: the shaped
        API will never cover every verb the socket answers, and a verb added
        tomorrow should be usable today. Ask ``session.verbs`` for the list this
        build actually offers: two counts written down here have already gone
        stale, and a number nothing checks is worse than no number.
        """
        reply = self._conn.call(verb, params)
        if reply.get("error"):
            raise errors.refusal(verb, reply["error"], reply.get("code", ""))
        return reply.get("result")

    def subscribe(self, *topics: str) -> Subscription:
        """Stream server-pushed notifications for the given topics, rather than
        polling. Opens a second connection to this same workbench, so closing
        the returned Subscription hangs up only that stream.

        Topics today: "status" (a new console line) and "snapshot" (a compact
        summary after each publish, coalesced by the server so a busy run cannot
        flood a slow reader).
        """
        return Subscription(*topics, address=self._conn.address)

    def checkpoint(self, name: str) -> dict[str, Any]:
        """Freeze the whole session under a name - the network, how it is being
        run, and where the clock had got to - so it can be taken back here."""
        return self.call("session.checkpoint", {"name": name}) or {}

    def restore(self, name: str) -> dict[str, Any]:
        """Rebuild a checkpoint and replay to the moment it was taken. Returns
        as soon as the replay is under way; the sim reaching target_ms is when
        it has actually arrived. Deterministic, so it comes back to exactly
        where it was - at the cost of the replay taking the run's own time."""
        return self.call("session.restore", {"name": name}) or {}

    def checkpoints(self) -> list[str]:
        """What can be restored, by name."""
        return (self.call("session.checkpoints") or {}).get("checkpoints", [])

    def snapshot(self) -> dict[str, Any]:
        """The whole session as the socket summarises it."""
        return self.call("session.snapshot") or {}

    def describe(self) -> dict[str, Any]:
        """The cheap summary: nodes, seed, time, whether it is playing."""
        return self.call("session.describe") or {}

    def journal(self) -> dict[str, Any]:
        """Every command this workbench has been driven with, newest last, and
        when the process started - so a session picked up cold can be told how
        the world got here, and whether it has been restarted."""
        return self.call("session.journal") or {}

    def verbs(self) -> list[str]:
        """Every method this build answers."""
        return (self.call("session.verbs") or {}).get("verbs", [])

    def say(self, text: str) -> None:
        """Leave a line in the session's log, for whoever is watching."""
        self.call("ui.said", text)

    def keep_above(self, on: bool | None = None) -> bool:
        """Whether a panel opened in its own window stays above the main one.

        Reads the preference when called with nothing, sets it when given a
        value, and returns what it now is.

        The preference exists for Linux under Wayland, where no client may ask
        a normal window to stay above others. What can be asked for is a
        layer-shell surface, and that is a different kind of window: the
        compositor gives it no title bar, no taskbar entry and no minimise, so
        the window draws its own bar and its close button returns the panel to
        the main window. Somebody who would rather have the compositor's own
        windows turns this off. On macOS and Windows always-on-top costs
        nothing and the preference does not apply.
        """
        params = {} if on is None else {"on": on}
        return (self.call("ui.keep_above", params) or {}).get("on", True)

    def window(self, node: str | Node, tab: Tab | str = "") -> str:
        """Open a node's own window, on a named tab.

        Windowed sessions only, and it says so here rather than appearing to
        work: a headless run has nothing to open, and a script that "opened the
        Hardware tab" in CI and saw no error will be written to assume it did.

        Tabs are named as they are on the strip - Console, Companion, SDR,
        Settings, Radio, Stats, Activity, Connect, Hardware. Returns the one it
        opened on.
        """
        if self.is_headless:
            raise errors.Unavailable(
                "node.window",
                "this session has no interface attached, so there is nothing to show",
                "unavailable",
            )
        got = self.call("node.window", {"node": str(node), "tab": tab}) or {}
        return got.get("tab", "")

    # ---- the shape -------------------------------------------------------

    @property
    def nodes(self) -> Nodes:
        return Nodes(self)

    def node(self, name: str) -> Node:
        """A handle, without checking it exists - so one can be named before
        it is placed. Every method on it will say so if it does not."""
        return Node(self, name)

    @property
    def schedule(self) -> Schedule:
        return Schedule(self)

    @property
    def assertions(self) -> Assertions:
        return Assertions(self)

    @property
    def sim(self) -> Sim:
        return Sim(self)

    @property
    def project(self) -> Project:
        return Project(self)

    @property
    def firmware(self) -> Firmware:
        return Firmware(self)

    @property
    def events(self) -> Events:
        return Events(self)

    @property
    def boundary(self) -> Boundary:
        """The study area: which nodes are in the question being asked. Set it
        before importing, because the import filters at fetch time."""
        return Boundary(self)

    @property
    def live(self) -> Live:
        """A live deployment feed - CoreScope and the rest - and the import
        chain that brings one in."""
        return Live(self)

    def console(self, node: str) -> Console:
        return Console(self, node)

    def job(self, job_id: str) -> Job:
        return Job(self, job_id)

    def jobs(self) -> list[dict[str, Any]]:
        """Everything long-running that is in flight."""
        return self.snapshot().get("jobs", [])

    def wait_for_nodes(self, timeout: timedelta = JOB_WAIT) -> None:
        """Wait until the session has a network in it.

        For a fixture opened at startup, which happens on a worker: the socket
        answers first, so everything asked before the open lands describes an
        empty session and is believed.
        """

        def check():
            n = self.describe().get("nodes", 0)
            return (True, "") if n else (False, "no nodes yet")

        wait_for(check, timeout, "the fixture to open")

    def wait_idle(self, timeout: timedelta = JOB_WAIT) -> None:
        """Wait for every job to finish.

        The honest way to wait out a warm, which is what most of them are.

        Finished jobs are ignored rather than waited for: some are removed when
        they end and some are only marked - infer.run's is marked - so waiting
        for the list to empty waits forever on half of them. That is a
        difference between the verbs, not something a caller should know.
        """

        def check():
            running = [j for j in self.jobs() if not j.get("finished")]
            if not running:
                return True, ""
            first = running[0]
            return False, (
                f"{len(running)} still running, first is {first.get('what')!r} "
                f"({first.get('done')} of {first.get('total')})"
            )

        wait_for(check, timeout, "the workbench to go idle")

    def node_stats(self) -> list[NodeStat]:
        """Sample every node and return what it found.

        A sample, not a read: it costs a /proc read per node, which is why the
        window only does it while somebody is looking at the panel.
        """
        got = self.call("nodes.stats") or {}
        return [NodeStat.parse(r) for r in got.get("stats") or []]

    def provenance(self) -> Provenance:
        """What this session's measurements are being made under.

        Read from the session rather than carried on each result, for now: the
        verbs do not return it yet, and inventing it here would be a claim this
        client is not entitled to make.
        """
        s = self.snapshot()
        return Provenance(
            rf_mode=s.get("rf_mode", ""),
            excess_loss_db=float(s.get("excess_loss_db") or 0.0),
            calibrated=bool(s.get("calibrated")),
            seed=int(s.get("seed") or 0),
        )


def _launch_kw(kw: dict) -> dict:
    """launch() takes the same arguments as headless(), by design: a script
    should be able to switch between watching a run and not watching it by
    changing one word."""
    return kw
