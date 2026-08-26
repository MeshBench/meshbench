"""The rest of a scripted run: the project, the clock, firmware, what
happened, and what a node said."""

from __future__ import annotations

import time
from datetime import timedelta
from typing import TYPE_CHECKING, Any

from . import errors
from .sets import Board, Kind, Role
from .types import Build, BuildDetails, Event, JobInfo, SimState
from .wait import (
    EVENT_WAIT,
    FIRMWARE_WAIT,
    JOB_WAIT,
    RUN_WAIT,
    as_seconds,
    wait_for,
)

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

    def start(
        self, firmware_wait: timedelta = FIRMWARE_WAIT, wait: timedelta = JOB_WAIT
    ) -> None:
        """Bring the run up: wait out the warm, start every node, and play.

        Deliberately not one call to ``sim.start``. That verb is the play
        button's own handler and answers four ways - it *pauses* if already
        playing, declines while links are being measured, or starts firmware
        and does not play - so a script pressing it once gets whichever of
        those the moment happens to be in.

        Worse, it only starts firmware when **no** node is running. Pin a
        build onto two nodes of a fifty-eight node fixture and it considers
        the mesh started, plays with fifty-six of them down, and says nothing.

        So this asks for the three things it actually wants, in order, and
        checks each one.
        """
        # The links first. Nothing that follows means anything against a
        # matrix that is still being measured.
        self._wb.wait_idle(wait)

        # Then every node that is not up, which firmware.start does and
        # sim.start does only when none of them are.
        st = self._wb.firmware.state()
        if st.get("running", 0) < st.get("nodes", 0):
            self._wb.firmware.start()
            self._wb.firmware.wait_started(firmware_wait)

        # Then the clock, by its own name. play cannot pause, which is the
        # other half of what made start unusable from a script.
        if not self.playing:
            self.play()

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

    def run(self, simulated: timedelta, wait: timedelta = RUN_WAIT) -> None:
        """Advance the mesh's own clock by this much, and wait for it.

        Two clocks, one call, and they are not the same one:

        - ``simulated`` is the mesh's. ``timedelta(minutes=5)`` is five minutes
          of its time.
        - ``wait`` is yours: how long you are prepared to sit here before
          giving up. On 155 emulated nodes five simulated minutes is a great
          deal more than five of yours, which is why it is separate and
          generous.
        """
        total = int(as_seconds(simulated) * 1000)
        if total <= 0:
            raise ValueError("run() needs a length, e.g. timedelta(minutes=5)")
        self._wb.call("sim.run", {"for_ms": total})
        self.wait_stopped(wait)

    def wait_stopped(self, timeout: timedelta = RUN_WAIT) -> None:
        """Wait for a run to end. ``timeout`` is wall clock."""

        def check():
            st = self.state()
            if not st.playing:
                return True, ""
            return False, f"{st.now_ms / 1000:.1f}s of simulated time"

        wait_for(check, timeout, "the run to finish")

    def wait_until(self, at: timedelta, timeout: timedelta = RUN_WAIT) -> None:
        """Wait for the mesh's clock to reach a moment.

        ``at`` is simulated time; ``timeout`` is yours.
        """
        at_ms = int(as_seconds(at) * 1000)

        def check():
            st = self.state()
            if st.now_ms >= at_ms:
                return True, ""
            return False, f"{st.now_ms / 1000:.1f}s"

        wait_for(check, timeout, f"simulated time to reach {at_ms / 1000:.1f}s")


def _build_id(
    build: Build | str, board: Board | str = "", role: Role | str = ""
) -> dict[str, Any]:
    """Which build a call means, from either a Build or a bare label.

    A Build carries all three names and they are sent together, so the call
    cannot land on a different build that happens to share a label. A bare
    label sends only what was given, and the workbench refuses it when it is
    ambiguous rather than guessing.
    """
    if isinstance(build, Build):
        return {"version": build.version, "role": build.role, "board": build.board}
    p: dict[str, Any] = {"version": build}
    if role:
        p["role"] = role
    if board:
        p["board"] = board
    return p


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

    def find(self, version: str, board: Board | str = "") -> Build:
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

    def download(self, role: str, version: str, board: Board | str = "") -> None:
        """Fetch a published build.

        ``role`` is a plain string here and a Role everywhere else,
        deliberately: this one names a published release asset, and the
        catalogue's own spellings are not always the application names the
        verbs are keyed on.
        """
        p: dict[str, Any] = {"role": role, "version": version}
        if board:
            p["board"] = board
        self._wb.call("firmware.download", p)

    def import_(self, path: str, role: str, board: str = "", label: str = "") -> Build:
        """Take a build from a path - the one way a locally built image gets
        into the library.

        ``label`` is what the library will know it by and what a node pins.
        Left out it is a timestamp, so importing twice gives two builds rather
        than one that quietly replaced the other - which matters the moment you
        want to put the new one on a node and delete the old.
        """
        p: dict[str, Any] = {"path": path, "role": role}
        if board:
            p["board"] = board
        if label:
            p["label"] = label
        return Build.parse(self._wb.call("firmware.import", p) or {})

    def delete(self, build: Build) -> str:
        """Remove a build from the cache, and say what was removed.

        By path, and the workbench refuses any path outside the firmware
        cache. A build nodes are still pinned to will go: they keep the pin,
        which then cannot be honoured and fails at start - so move them onto
        the replacement first.
        """
        if not build.path:
            raise ValueError(f"{build} has no path on this machine to delete")
        got = self._wb.call("firmware.delete", {"path": build.path}) or {}
        return got.get("deleted", "")

    def details(
        self, build: Build | str, board: Board | str = "", role: Role | str = ""
    ) -> BuildDetails:
        """Everything known about one build: where it is, what it is, and what
        has been decided about it.

        Takes a Build or a bare label. A label alone is refused when it names
        more than one build - the same image imported for two boards, say -
        because acting on the wrong one is a rename of somebody else's image.
        """
        p: dict[str, Any] = dict(_build_id(build, board, role))
        return BuildDetails.parse(self._wb.call("firmware.details", p) or {})

    def update(
        self,
        build: Build | str,
        *,
        board: Board | str = "",
        role: Role | str = "",
        label: str | None = None,
        new_role: Role | str | None = None,
        new_board: Board | str | None = None,
        coproc_at_reset: bool | None = None,
        card_required: bool | None = None,
        notes: str | None = None,
    ) -> BuildDetails:
        """Rename a build, move it to another board or role, or change how it
        is run.

        Renaming moves the file, because the name is the identity: a board
        image is stored as ``<board>/<role>@<label>.bin`` and nothing else
        records what it is. Nodes pinned to the old name are repointed, or they
        would fail at their next start with "no image in the cache" about a
        build sitting in the library under its new name.

        Every argument left out is left alone, which is why they default to
        None rather than to "" or False: "leave this setting" and "turn it
        off" are different answers.
        """
        p: dict[str, Any] = dict(_build_id(build, board, role))
        if label is not None:
            p["label"] = label
        if new_role is not None:
            p["new_role"] = new_role
        if new_board is not None:
            p["new_board"] = new_board
        if coproc_at_reset is not None:
            p["coproc_at_reset"] = coproc_at_reset
        if card_required is not None:
            p["card_required"] = card_required
        if notes is not None:
            p["notes"] = notes
        got = self._wb.call("firmware.update", p) or {}
        return self.details(
            got.get("version", ""), got.get("board", ""), got.get("role", "")
        )

    def window(
        self, build: Build | str, board: Board | str = "", role: Role | str = ""
    ) -> None:
        """Open the build's own window - what a click on a library row does."""
        self._wb.call("firmware.window", dict(_build_id(build, board, role)))

    def build(
        self,
        checkout: str,
        role: Role | str = "",
        label: str = "",
        wait: timedelta = JOB_WAIT,
    ) -> list[Build]:
        """Compile a MeshCore checkout and put the results in the library.

        Both roles unless one is named, deliberately. A locally built repeater
        compiled against a stale shim once answered console output with 0x06
        where the host expects 0x07: it connected, misbehaved and exited. Two
        arms of a comparison built at different moments from different trees
        measure the build process rather than the firmware, so the easy thing
        here is the thing that builds them together.

        Blocks until it is done - a MeshCore build is a minute or two per
        role - and returns what the library now holds that was built locally.
        """
        p: dict[str, Any] = {"source": checkout}
        if role:
            p["role"] = role
        if label:
            p["label"] = label
        got = self._wb.call("firmware.build", p) or {}
        self._wb.job(got.get("job", "firmware-build")).wait(wait)
        return [b for b in self.library() if b.version.startswith("local-")]

    def use_what_is_here(self) -> dict[Role, Build]:
        """Pin every role that needs one to the newest build on this machine.

        What a script wants almost every time: this mesh, whatever this machine
        holds, rather than a version typed into the script that goes stale. A
        run refuses to start until every role is answered, so the alternative
        is the same loop written out in each one.

        Refuses by name when a role has nothing, because "no companion build"
        is a thing to go and fix rather than a reason to start a mesh with a
        silent hole in it.
        """
        have = [b for b in self.on_disk() if not b.board]
        chosen: dict[Role, Build] = {}
        for row in self.needed():
            role = row["role"]
            for b in have:
                if b.role == role:
                    chosen[role] = b
            if role not in chosen:
                raise errors.NotFound(
                    "firmware.needed",
                    f"no {role} build on this machine: "
                    f"meshbench firmware download {role}",
                    "not_found",
                )
            self.use_for_role(role, chosen[role])
        return chosen

    def use_for_role(self, role: Role, build: Build | str) -> None:
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

    def wait_started(self, timeout: timedelta = FIRMWARE_WAIT) -> None:
        """Wait for every node's firmware to be up. ``timeout`` is wall clock.

        ``nodes`` here is the nodes that *run* firmware, which is not every
        node - an SDR observer and an emitter never boot one. It used to be
        every node, so a fixture holding either reported "56 of 58" until the
        timeout and there was no way to see which two.

        Which is why the wait names the stragglers rather than counting them.
        Ten minutes of "56 of 58" tells you nothing; two node names tell you
        whether a build is missing or a board is wedged.
        """

        # The names cost a /proc read per node and this polls while firmware is
        # starting, which is the busiest moment there is - 58 nodes every
        # fiftieth of a second is how a diagnostic becomes the fault it was
        # meant to explain, and it timed the socket out. Once every ten
        # seconds is often enough for something a person only reads when the
        # wait fails.
        named = [0.0]

        def stragglers() -> str:
            now = time.monotonic()
            if now - named[0] < 10.0:
                return ""
            named[0] = now
            waiting = [s.name for s in self._wb.node_stats() if not s.running]
            if not waiting:
                return ""
            shown = ", ".join(waiting[:4])
            if len(waiting) > 4:
                shown += f" and {len(waiting) - 4} more"
            return f"; waiting on {shown}"

        last = [""]

        def check():
            st = self.state()
            running, nodes = st.get("running", 0), st.get("nodes", 0)
            if not st.get("starting") and nodes and running >= nodes:
                return True, ""
            if who := stragglers():
                last[0] = who
            return False, f"{running} of {nodes} running{last[0]}"

        wait_for(check, timeout, "firmware to come up")


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
        timeout: timedelta = EVENT_WAIT,
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
        wait_for(check, timeout, f"an event matching {want}")
        return found[0]


class Console:
    """One node's firmware console. Live.

    Two consoles, not one, and which you get depends on what the node is.

    A repeater has a text CLI and reads typed bytes. A companion does not: it
    speaks the framed companion protocol, and its command line is
    meshcore-cli's vocabulary - ``advert``, ``public <msg>``, ``chan <n>
    <msg>``. Typing text at a companion goes nowhere, is echoed locally, and
    reads exactly like a command that ran and did nothing.

    So this picks the right one from the node's kind. A caller should not have
    to know, and every caller that did know got it wrong at least once.
    """

    def __init__(self, wb: Workbench, node: str) -> None:
        self._wb = wb
        self.node = node

    @property
    def _framed(self) -> bool:
        """Whether this node's console is the framed protocol."""
        try:
            return self._wb.nodes.info(self.node).kind in (
                Kind.COMPANION,
                Kind.ROOM_SERVER,
            )
        except errors.MeshbenchError:
            # A node this client cannot see is not one to guess about; let the
            # verb below say so in its own words.
            return False

    def send(self, line: str) -> None:
        verb = "console.cli" if self._framed else "console.type"
        self._wb.call(verb, {"node": self.node, "command": line})

    def read(self) -> list[str]:
        """The scrollback, newest last.

        The lines come back under "tail" and "lines" is how many there are in
        total - so reading "lines" hands you an integer where you asked for
        text, and every use of it fails somewhere further along. The tail is
        the last 200; a node up for an hour has thousands and nobody reads the
        first one.
        """
        got = self._wb.call("console.read", {"node": self.node}) or {}
        return got.get("tail") or []

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
            sim.wait_until(
                timedelta(milliseconds=st.now_ms + steps * max(st.step_ms, 1)),
                timeout=timedelta(minutes=2),
            )
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

    def wait(self, timeout: timedelta = JOB_WAIT) -> None:
        """Wait for it to finish, and raise if it finished badly.

        ``timeout`` is wall clock. Ended is not the same as worked: a read that
        failed used to finish the job with the reason in its title and nothing
        else, so every caller either carried on as though it had succeeded or
        matched on the wording.
        """
        last: list[JobInfo] = []

        def check():
            info = self.info()
            if info is None:
                return True, ""
            last.append(info)
            if info.finished:
                return True, ""
            return False, f"{info.what}, {info.done} of {info.total}"

        wait_for(check, timeout, f"job {self.id}")
        if last and last[-1].failed:
            raise errors.Refused(
                "job", f"job {self.id} failed: {last[-1].what}", "internal"
            )
