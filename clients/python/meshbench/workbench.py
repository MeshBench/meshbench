"""The connection, and everything hanging off it."""

from __future__ import annotations

import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from typing import Any

from . import errors
from ._socket import (
    MAX_UNIX_PATH,
    PROTOCOL,
    RENDEZVOUS_ENV,
    Connection,
    default_address,
)
from .nodes import Node, Nodes
from .parts import Console, Events, Firmware, Job, Project, Sim
from .types import Hello, NodeStat, Provenance
from .wait import wait_for


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
    def attach_or_launch(cls, **kw) -> Workbench:
        """Use the one that is running, or start one.

        For a script somebody runs repeatedly by hand, which is example 2 in
        #209: the second run should carry on from the first rather than
        clearing everything down.
        """
        path = kw.get("socket") or default_address()
        try:
            return cls.attach(path)
        except errors.MeshbenchError:
            return cls.headless(**kw)

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
        exe = binary or shutil.which("meshcoresim") or "meshcoresim"
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
            return cls(conn, process=proc)
        except Exception:
            conn.close()
            proc.kill()
            raise

    def _greet(self) -> None:
        """Ask what this is, and refuse a build this client cannot speak to."""
        self.hello = Hello.parse(self.call("session.hello"))
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
        API will never cover all 213 verbs, and a verb added tomorrow should be
        usable today.
        """
        reply = self._conn.call(verb, params)
        if reply.get("error"):
            raise errors.refusal(verb, reply["error"], reply.get("code", ""))
        return reply.get("result")

    def snapshot(self) -> dict[str, Any]:
        """The whole session as the socket summarises it."""
        return self.call("session.snapshot") or {}

    def describe(self) -> dict[str, Any]:
        """The cheap summary: nodes, seed, time, whether it is playing."""
        return self.call("session.describe") or {}

    def verbs(self) -> list[str]:
        """Every method this build answers."""
        return (self.call("session.verbs") or {}).get("verbs", [])

    def say(self, text: str) -> None:
        """Leave a line in the session's log, for whoever is watching."""
        self.call("ui.said", text)

    def window(self, node: str | Node, tab: str = "") -> str:
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

    def console(self, node: str) -> Console:
        return Console(self, node)

    def job(self, job_id: str) -> Job:
        return Job(self, job_id)

    def jobs(self) -> list[dict[str, Any]]:
        """Everything long-running that is in flight."""
        return self.snapshot().get("jobs", [])

    def wait_idle(self, timeout: float = 1800.0) -> None:
        """Wait for every job to finish - the honest way to wait out a warm,
        which is what most of them are."""

        def check():
            jobs = self.jobs()
            if not jobs:
                return True, ""
            return False, f"{len(jobs)} still running, first is {jobs[0].get('what')!r}"

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
