"""The wire: one JSON request per line, one reply.

Everything above this is shape. This is the whole protocol, and it is small on
purpose - a client that needed a framework to speak to a local socket would be
a client nobody could debug.

Two transports, because one does not travel:

- A **unix socket**, where the operating system has one. The filesystem is the
  access control, and the kernel enforces it.
- **Loopback TCP with a token**, where it does not. Windows is the case:
  CPython has never exposed ``socket.AF_UNIX`` there, so a unix socket is not
  merely awkward from Python on Windows, it is unreachable. The workbench binds
  127.0.0.1 on an ephemeral port and writes the address and a 128-bit token to
  a 0600 file; this reads that file and presents the token before anything
  else. Any local process can open a loopback port, so the token is what stands
  where the kernel stood.

The choice is by operating system, not by language, so the Go client and this
one always speak the same thing on the same machine.
"""

from __future__ import annotations

import contextlib
import json
import os
import socket
import sys
import threading
from pathlib import Path
from typing import Any

#: Chooses where the workbench answers: a path, or "tcp", or "tcp:host:port".
SOCKET_ENV = "MESHBENCH_CONTROL_SOCKET"

#: What to run when a client is asked to start a workbench and nothing named a
#: binary. A checkout has one built but not installed, and every example and
#: every test then needs the same three lines to find it - so the variable the
#: test harness already used is honoured by the clients too.
BINARY_ENV = "MESHBENCH_BINARY"

#: Chooses the file a TCP listener writes its address and token to. Per user by
#: default, which is wrong for two runs at once - the second would overwrite the
#: first's - so a client that starts a workbench gives it one of its own.
RENDEZVOUS_ENV = "MESHBENCH_CONTROL_RENDEZVOUS"

#: The wire version this client speaks. A workbench answering anything else is
#: refused at connect rather than halfway through a script.
PROTOCOL = 1

#: The shortest sun_path any platform we run on allows: 108 on Linux, 104 on
#: macOS and the BSDs.
MAX_UNIX_PATH = 104

#: Whether this Python can speak to a unix socket at all.
HAVE_AF_UNIX = hasattr(socket, "AF_UNIX")


def _cache_dir() -> Path:
    """The per-user directory this OS already defines.

    What os.getuid() was standing in for, less portably - it does not exist on
    Windows at all, so the old default crashed there before it could be wrong.
    """
    if sys.platform == "win32":
        base = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
    elif sys.platform == "darwin":
        base = os.path.expanduser("~/Library/Caches")
    else:
        base = os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache")
    d = Path(base) / "meshbench"
    d.mkdir(parents=True, exist_ok=True)
    return d


def rendezvous_path() -> Path:
    """Where a TCP listener leaves its address and token."""
    named = os.environ.get(RENDEZVOUS_ENV)
    if named:
        p = Path(named)
        p.parent.mkdir(parents=True, exist_ok=True)
        return p
    return _cache_dir() / "control.json"


def default_address() -> str:
    """Where a workbench answers on this operating system unless told otherwise."""
    env = os.environ.get(SOCKET_ENV)
    if env:
        return env
    if sys.platform == "win32" or not HAVE_AF_UNIX:
        return "tcp"
    # Linux keeps exactly the path it has always had: scripts, the MCP server
    # and tools/soak all name it, and moving it would break them for no gain.
    runtime = os.environ.get("XDG_RUNTIME_DIR")
    if runtime:
        return os.path.join(runtime, "meshcoresim.sock")
    # Everywhere else, the per-user cache directory - which on macOS is also
    # short enough to stay inside sun_path, where $TMPDIR would not be.
    return str(_cache_dir() / "control.sock")


#: Kept for callers written against the old name, which meant the same thing
#: while a unix socket was the only thing there was.
default_socket_path = default_address


def _read_rendezvous() -> tuple[str, str]:
    path = rendezvous_path()
    try:
        got = json.loads(path.read_text())
    except OSError as e:
        raise ConnectionError(f"no workbench has left an address at {path}: {e}") from e
    except ValueError as e:
        raise ConnectionError(f"{path} is not readable as an address: {e}") from e
    return got.get("address", ""), got.get("token", "")


def _connect(address: str, timeout: float | None) -> tuple[socket.socket, str]:
    """Open the socket the address names, and return it with any token."""
    if address == "tcp" or address.startswith("tcp:"):
        token = ""
        if address == "tcp":
            host_port, token = _read_rendezvous()
        else:
            host_port = address[len("tcp:") :]
            if ":" not in host_port:
                host_port = "127.0.0.1:" + host_port
            # A port somebody named still needs the token, and the file is the
            # only place it exists.
            _, token = _read_rendezvous()
        host, _, port = host_port.rpartition(":")
        s = socket.create_connection((host or "127.0.0.1", int(port)), timeout=timeout)
        return s, token

    path = address[len("unix:") :] if address.startswith("unix:") else address
    if not HAVE_AF_UNIX:
        raise ConnectionError(
            f"{sys.platform} has no unix socket this Python can open, and "
            f"{path} is one. Start the workbench with -control-socket tcp"
        )
    if len(path) > MAX_UNIX_PATH:
        raise ConnectionError(
            f"{path} is {len(path)} bytes and a unix socket path may be at "
            f"most {MAX_UNIX_PATH} - choose a shorter one, or use tcp"
        )
    s = socket.socket(socket.AF_UNIX)
    s.settimeout(timeout)
    s.connect(path)
    return s, ""


class Connection:
    """One socket, and the lock that keeps two threads from interleaving.

    The protocol has request ids but the workbench answers in order, so the
    simplest correct thing is one call at a time. A client that pipelined would
    have to demultiplex, and nothing here needs the throughput.
    """

    def __init__(self, address: str | None = None, timeout: float | None = 300.0):
        self.address = address or default_address()
        self._sock, token = _connect(self.address, timeout)
        self._file = self._sock.makefile("rb")
        self._lock = threading.Lock()
        self._next_id = 0
        if token:
            # The token first, before anything else on the wire. A loopback
            # port is reachable by any local process, so this is what stands in
            # for the permissions a unix socket would have had.
            self._sock.sendall((json.dumps({"token": token}) + "\n").encode())

    def call(self, verb: str, params: Any = None) -> dict[str, Any]:
        """Send one verb and return the whole reply, errors included."""
        with self._lock:
            self._next_id += 1
            req: dict[str, Any] = {"id": self._next_id, "method": verb}
            if params is not None:
                req["params"] = params
            self._sock.sendall((json.dumps(req) + "\n").encode())
            line = self._file.readline()
            if not line:
                raise ConnectionError(
                    f"the workbench at {self.address} closed the connection"
                )
            return json.loads(line.decode())

    def shutdown(self) -> None:
        """Break a read in progress so a streaming reader on another thread can
        be closed. A plain close would deadlock: readline holds the buffer's
        lock while it blocks on the socket, and close wants that same lock. A
        socket shutdown makes the blocked read return instead, and then close is
        uncontended. Errors are ignored - the socket may already be gone.
        """
        with contextlib.suppress(OSError):
            self._sock.shutdown(socket.SHUT_RDWR)

    def close(self) -> None:
        try:
            self._file.close()
        finally:
            self._sock.close()


def is_live(address: str) -> bool:
    """Whether something is already answering there.

    A connect rather than a stat: a socket file existing says nothing about
    whether anybody is behind it, and that difference is the whole question.
    """
    try:
        s, _ = _connect(address, 0.25)
        s.close()
        return True
    except (OSError, ConnectionError, ValueError):
        return False
