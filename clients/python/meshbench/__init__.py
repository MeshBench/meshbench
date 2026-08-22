"""Drive a MeshBench workbench from Python.

    from meshbench import Workbench

    with Workbench.headless(fixture="fife-strict", seed=9001) as wb:
        wb.sim.run(minutes=5)
        print(wb.provenance())
        print(wb.events.total(), "events")

Two layers. `wb.call(verb, params)` is the whole API and stays public, so a
verb this package has not shaped is one line away rather than a blocker; above
it sit `wb.nodes`, `wb.sim`, `wb.firmware`, `wb.events`, `wb.project` and a
node's own console.

Every wait is a method - `node.wait_running()`, `sim.run()`,
`firmware.wait_started()` - never a sleep in a script. They poll today and will
subscribe later, and no script changes when they do.

A wait measured in simulated time is not a wait measured in yours:
`sim.run(minutes=5)` is five minutes of the mesh's own clock, and on 155
emulated nodes that is a great deal longer than five of yours.
"""

from ._socket import (
    PROTOCOL,
    RENDEZVOUS_ENV,
    SOCKET_ENV,
    default_address,
    default_socket_path,
)
from .checks import Assertions, Check, Report, Schedule
from .errors import (
    BadParams,
    Closing,
    Conflict,
    MeshbenchError,
    NotFound,
    ProtocolMismatch,
    Refused,
    Timeout,
    Unavailable,
    UnknownVerb,
)
from .nodes import Node, Nodes
from .parts import Console, Events, Firmware, Job, Project, Sim
from .sets import DEFAULT_PRESET, Board, Kind, Preset
from .types import (
    ADVANCED_REPEATER,
    COMPANION,
    EMITTER,
    FLOOR,
    HALF_DUPLEX,
    INTERFERENCE,
    RECEIVED,
    ROOM_SERVER,
    SDR_OBSERVER,
    SENT,
    SIMPLE_REPEATER,
    Build,
    Event,
    Hello,
    JobInfo,
    NodeInfo,
    NodeStat,
    Provenance,
    SimState,
)
from .workbench import Workbench

__all__ = [
    "Schedule",
    "Report",
    "Preset",
    "Kind",
    "DEFAULT_PRESET",
    "Check",
    "Board",
    "Assertions",
    "ADVANCED_REPEATER",
    "COMPANION",
    "EMITTER",
    "FLOOR",
    "HALF_DUPLEX",
    "INTERFERENCE",
    "PROTOCOL",
    "RECEIVED",
    "ROOM_SERVER",
    "SDR_OBSERVER",
    "SENT",
    "SIMPLE_REPEATER",
    "RENDEZVOUS_ENV",
    "SOCKET_ENV",
    "BadParams",
    "Build",
    "Closing",
    "Conflict",
    "Console",
    "Event",
    "Events",
    "Firmware",
    "Hello",
    "Job",
    "JobInfo",
    "MeshbenchError",
    "Node",
    "NodeInfo",
    "NodeStat",
    "Nodes",
    "NotFound",
    "Project",
    "ProtocolMismatch",
    "Provenance",
    "Refused",
    "Sim",
    "SimState",
    "Timeout",
    "Unavailable",
    "UnknownVerb",
    "Workbench",
    "default_address",
    "default_socket_path",
]

__version__ = "0.1.0"
