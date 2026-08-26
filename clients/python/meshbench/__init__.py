"""Drive a MeshBench workbench from Python.

    from meshbench import Workbench

    with Workbench.headless(fixture="fife-strict", seed=9001) as wb:
        wb.sim.run(timedelta(minutes=5))
        print(wb.provenance())
        print(wb.events.total(), "events")

Two layers. `wb.call(verb, params)` is the whole API and stays public, so a
verb this package has not shaped is one line away rather than a blocker; above
it sit `wb.nodes`, `wb.sim`, `wb.firmware`, `wb.events`, `wb.project`,
`wb.live` and a node's own console.

Every wait is a method - `node.wait_running()`, `sim.run()`,
`firmware.wait_started()` - never a sleep in a script. They poll today and will
subscribe later, and no script changes when they do.

A wait measured in simulated time is not a wait measured in yours:
`sim.run(minutes=5)` is five minutes of the mesh's own clock, and on 155
emulated nodes that is a great deal longer than five of yours.
"""

from ._socket import (
    BINARY_ENV,
    PROTOCOL,
    RENDEZVOUS_ENV,
    SOCKET_ENV,
    default_address,
    default_socket_path,
)
from .boundary import Boundary
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
from .live import DEFAULT_WINDOW, Live
from .nodes import Node, Nodes
from .parts import Console, Events, Firmware, Job, Project, Sim
from .sets import (
    DEFAULT_PRESET,
    Board,
    Class,
    Kind,
    Preset,
    Role,
    Strategy,
    Tab,
    Transport,
)
from .subscribe import Notification, Subscription, subscribe
from .types import (
    Build,
    BuildDetails,
    CardSlot,
    Event,
    Hello,
    ImportPreview,
    JobInfo,
    NameMatch,
    NodeInfo,
    NodeStat,
    Provenance,
    SimState,
)
from .workbench import Workbench

__all__ = [
    "Schedule",
    "BINARY_ENV",
    "Boundary",
    "Transport",
    "Tab",
    "Strategy",
    "Role",
    "Class",
    "Live",
    "DEFAULT_WINDOW",
    "ImportPreview",
    "NameMatch",
    "Report",
    "Preset",
    "Kind",
    "DEFAULT_PRESET",
    "Check",
    "Board",
    "Assertions",
    "PROTOCOL",
    "RENDEZVOUS_ENV",
    "SOCKET_ENV",
    "BadParams",
    "Build",
    "BuildDetails",
    "CardSlot",
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
    "Notification",
    "Subscription",
    "subscribe",
    "Workbench",
    "default_address",
    "default_socket_path",
]

__version__ = "0.1.0"
