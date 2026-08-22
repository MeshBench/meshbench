"""The rest of a scripted run: the project, the clock, firmware, what
happened, and what a node said."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from . import errors
from .types import Build, Event, JobInfo, SimState
from .wait import seconds, wait_for

if TYPE_CHECKING:  # pragma: no cover
    from .workbench import Workbench


class Project:
    """Opening, saving, and starting over. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def new(self, place: str | None = None) -> None:
        """An empty network.

        With a place it becomes the study area and the map is framed on it,
        because those are the same wish - and because a blank network with no
        place is a map in the middle of the Atlantic.
        """
        self._wb.call("project.new", {"place": place} if place else {})

    def open(self, path: str) -> None:
        self._wb.call("project.open", path)

    def save(self, name: str) -> str:
        """Write it out.

        Worth doing before anything that might restart the process: the
        scenario lives in the process, not on disk.
        """
        return (self._wb.call("project.save", {"name": name}) or {}).get("path", "")

    def list(self) -> list[str]:
        return (self._wb.call("project.list") or {}).get("projects", [])


class Sim:
    """The clock, and the run. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def state(self) -> SimState:
        return SimState.parse(self._wb.call("sim.state") or {})

    @property
    def playing(self) -> bool:
        return self.state().playing

    @property
    def now_ms(self) -> int:
        return self.state().now_ms

    def start(self) -> None:
        """Warm the links, start firmware on every node, and play."""
        self._wb.call("sim.start")

    def play(self) -> None:
        self._wb.call("sim.play")

    def pause(self) -> None:
        self._wb.call("sim.pause")

    def step(self) -> None:
        self._wb.call("sim.step")

    def reset(self) -> None:
        self._wb.call("sim.reset")

    def settle(self, steps: int = 60) -> None:
        """Step a paused run, which is how a command gets the time it needs to
        be answered without starting the clock."""
        self._wb.call("sim.settle", {"steps": steps})

    @property
    def seed(self) -> int:
        return self.state().seed

    @seed.setter
    def seed(self, value: int) -> None:
        """Fix the run. Same seed, same scenario, same result - which is what
        makes a *changed* result mean something."""
        self._wb.call("sim.seed", {"seed": value})

    @property
    def step_ms(self) -> int:
        return self.state().step_ms

    @step_ms.setter
    def step_ms(self, value: int) -> None:
        self._wb.call("sim.speed", {"step_ms": value})

    def set_real_firmware(self, on: bool = True) -> None:
        self._wb.call("sim.kind", {"real": on})

    def run(
        self,
        ms: int | None = None,
        seconds_: float | None = None,
        minutes: float | None = None,
        wait: float | str = "30m",
    ) -> None:
        """Advance the mesh's own clock by this much, and wait for it.

        Simulated time, not yours. Five minutes here is five minutes of the
        mesh's clock; on 155 emulated nodes that is a great deal longer than
        five of yours, which is why the wait is a separate and generous
        argument.
        """
        total = ms or 0
        if seconds_:
            total += int(seconds_ * 1000)
        if minutes:
            total += int(minutes * 60_000)
        if total <= 0:
            raise ValueError("run() needs a length: ms=, seconds_= or minutes=")
        self._wb.call("sim.run", {"for_ms": total})
        self.wait_stopped(wait)

    def wait_stopped(self, timeout: float | str = "30m") -> None:
        def check():
            st = self.state()
            if not st.playing:
                return True, ""
            return False, f"{st.now_ms / 1000:.1f}s of simulated time"

        wait_for(check, seconds(timeout), "the run to finish")

    def wait_until(self, at_ms: int, timeout: float | str = "30m") -> None:
        def check():
            st = self.state()
            if st.now_ms >= at_ms:
                return True, ""
            return False, f"{st.now_ms / 1000:.1f}s"

        wait_for(
            check, seconds(timeout), f"simulated time to reach {at_ms / 1000:.1f}s"
        )


class Firmware:
    """What this machine can run, and what it is running. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def library(self) -> list[Build]:
        got = self._wb.call("firmware.library") or {}
        return [Build.parse(b) for b in got.get("builds") or []]

    def on_disk(self) -> list[Build]:
        """Only the ones this machine holds, which is the only thing that
        decides what a node can run. A build that failed to download and one in
        daily use look identical from anywhere else."""
        return [b for b in self.library() if b.on_disk]

    def find(self, version: str, board: str = "") -> Build:
        """One build, by version and - where the version alone is ambiguous,
        which it is for every board image - by board."""
        for b in self.library():
            if b.version == version and (not board or b.board == board):
                return b
        raise errors.NotFound(
            "firmware.library",
            f"no build {version!r} for board {board!r}",
            "not_found",
        )

    def scan(self) -> None:
        """Ask the catalogue what is published, which is how a build nobody has
        downloaded becomes offerable."""
        self._wb.call("firmware.published")

    def download(self, role: str, version: str, board: str = "") -> None:
        p: dict[str, Any] = {"role": role, "version": version}
        if board:
            p["board"] = board
        self._wb.call("firmware.download", p)

    def import_(self, path: str, role: str, board: str = "") -> Build:
        """Take a build from a path - the one way a locally built image gets
        into the library."""
        p: dict[str, Any] = {"path": path, "role": role}
        if board:
            p["board"] = board
        return Build.parse(self._wb.call("firmware.import", p) or {})

    def use_for_role(self, role: str, build: Build | str) -> None:
        version = build if isinstance(build, str) else build.version
        self._wb.call("firmware.set", {"role": role, "version": version})

    def start(self) -> None:
        """Bring up firmware on every node.

        Asynchronous, and always has been: it answers with what it has begun,
        not with what is up. It was synchronous once, and on 155 nodes that
        froze the window and the socket together for as long as it was left -
        which read as a crash and was reported as one.
        """
        self._wb.call("firmware.start")

    def state(self) -> dict[str, Any]:
        return self._wb.call("firmware.state") or {}

    def needed(self) -> list[dict[str, Any]]:
        """The roles with nodes and no build pinned, with what could be. A run
        refuses to start until every one is answered."""
        return (self._wb.call("firmware.needed") or {}).get("roles", [])

    def wait_started(self, timeout: float | str = "10m") -> None:
        def check():
            st = self.state()
            running, nodes = st.get("running", 0), st.get("nodes", 0)
            if not st.get("starting") and nodes and running >= nodes:
                return True, ""
            return False, f"{running} of {nodes} running"

        wait_for(check, seconds(timeout), "firmware to come up")


class Events:
    """What the engine has done. Live."""

    def __init__(self, wb: Workbench) -> None:
        self._wb = wb

    def recent(self, limit: int = 50) -> list[Event]:
        """The tail.

        A tail, and only a tail: the store keeps a bounded one because a long
        run has millions. A script that needs all of them dumps per round -
        reading only the tail after a busy flood samples the most congested
        moment of it, which is a mistake already made once here.
        """
        got = self._wb.call("events.recent", {"limit": limit}) or {}
        return [Event.parse(e) for e in got.get("events") or []]

    def total(self) -> int:
        return (self._wb.call("events.recent", {"limit": 1}) or {}).get("total", 0)

    def dump(self, path: str) -> int:
        """Write every event held to a file, one JSON object per line."""
        return (self._wb.call("events.dump", {"path": path}) or {}).get("written", 0)

    def wait(
        self,
        kind: str = "",
        from_: str = "",
        to: str = "",
        timeout: float | str = "5m",
    ) -> Event:
        """Wait for an event to match, and return it."""
        found: list[Event] = []

        def matches(e: Event) -> bool:
            return (
                (not kind or e.kind == kind)
                and (not from_ or e.from_ == from_)
                and (not to or e.to == to)
            )

        def check():
            evs = self.recent(500)
            for e in evs:
                if matches(e):
                    found.append(e)
                    return True, ""
            return False, f"{len(evs)} events, none matching"

        want = (
            " ".join(
                x
                for x in (
                    kind,
                    f"from {from_}" if from_ else "",
                    f"to {to}" if to else "",
                )
                if x
            )
            or "anything"
        )
        wait_for(check, seconds(timeout), f"an event matching {want}")
        return found[0]


class Console:
    """One node's firmware console. Live."""

    def __init__(self, wb: Workbench, node: str) -> None:
        self._wb = wb
        self.node = node

    def send(self, line: str) -> None:
        self._wb.call("console.type", {"node": self.node, "command": line})

    def read(self) -> list[str]:
        return (self._wb.call("console.read", {"node": self.node}) or {}).get(
            "lines", []
        )

    def ask(self, line: str, steps: int = 100) -> str:
        """Send a line and wait for the node to answer it.

        The important one. A node reads its serial input on its next loop and
        its loop only runs when the engine steps, so reading straight after
        sending reads the moment *before* the command was sent - every script
        that has done this by hand got an empty reply and concluded the console
        was broken. This gives the mesh its own time first.
        """
        before = self.read()
        self.send(line)
        sim = self._wb.sim
        st = sim.state()
        if st.playing:
            sim.wait_until(st.now_ms + steps * max(st.step_ms, 1), timeout="2m")
        else:
            sim.settle(steps)
        after = self.read()
        return "\n".join(after[len(before) :]) if len(after) > len(before) else ""


class Job:
    """A long operation the workbench is doing. Live: a handle to an id."""

    def __init__(self, wb: Workbench, job_id: str) -> None:
        self._wb = wb
        self.id = job_id

    def info(self) -> JobInfo | None:
        for j in self._wb.jobs():
            if j.get("id") == self.id:
                return JobInfo.parse(j)
        return None

    def cancel(self) -> None:
        """Stop it, where whoever started it left a way to.

        A job with no cancel refuses by name rather than silently doing
        nothing: an operator who asked deserves to be told, not left watching a
        bar that carries on.
        """
        self._wb.call("job.cancel", {"id": self.id})

    def wait(self, timeout: float | str = "30m") -> None:
        def check():
            info = self.info()
            if info is None or info.finished:
                return True, ""
            return False, f"{info.what}, {info.done} of {info.total}"

        wait_for(check, seconds(timeout), f"job {self.id}")
