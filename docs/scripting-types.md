# The scripting API's types

Every object a script can hold, and every field on it. The companion documents
are [scripting-api.md](scripting-api.md), which is the design and the methods,
and [scripting-verbs.md](scripting-verbs.md), which is the wire underneath.

Written against `210d9ec`. Where a type mirrors one in `internal/app/state`,
the Go source is named so the two can be held side by side; the parity test in
[#213](https://github.com/MeshBench/meshbench/issues/213) is what will keep
them honest once it exists.

## How to read this

- **Go** names are `CamelCase`, **Python** names are `snake_case`. Only the Go
  name is given; the Python one follows mechanically and the generator does the
  conversion.
- **read-only** means the field is a fact about the session. Assigning to a
  **settable** field runs a verb.
- A **live** object re-reads the session each time it is used; a **snapshot**
  object is a value taken once and never changes afterwards. Which one a type
  is decides whether a script can hold it across a `sim.run` and still be
  looking at the truth, so it is stated on every type.

---

## Workbench

**live.** The connection. Everything reachable hangs off one of these.

| field | type | meaning |
|---|---|---|
| `Nodes` | `NodeSet` | every node in the scenario |
| `Sim` | `Sim` | the clock and the run |
| `Project` | `Project` | open, save, list |
| `Firmware` | `FirmwareLibrary` | what this machine can run |
| `Links` | `LinkSet` | the measured pairs |
| `Study` | `Study` | coverage, budgets, energy, planning |
| `Events` | `EventLog` | what the engine has done |
| `Packets` | `Packets` | one transmission, dissected |
| `Experiment` | `Experiment` | the A/B matrix |
| `Sweep` | `Sweep` | the parameter sweep |
| `Validate` | `Validation` | the model against a real network |
| `RF` | `RFModel` | which physics, and how kind |
| `Radio` | `RadioSettings` | modem presets |
| `Boundary` | `Boundary` | study areas |
| `Import` | `Import` | pulling a real network in |
| `Feed` | `Feed` | live traffic in |
| `Fleet` | `Fleet` | one command to many nodes |
| `Schedule` | `Schedule` | what gets sent, when |
| `Assertions` | `Assertions` | what has to be true |
| `Provisioning` | `Provisioning` | what nodes are told at boot |
| `Boards` | `Boards` | the hardware profiles |
| `Resources` | `Resources` | what is downloaded at runtime |
| `Terrain` | `Terrain` | the elevation cache |
| `GPU` | `GPUState` | the hardware, and what it last did |
| `Jobs` | `JobSet` | long operations in flight |
| `Endpoints` | `[]Endpoint` | companions and observers served out |
| `Log` | `Log` | the session log |
| `UI` | `UI` | the window — `nil` in a headless session |
| `Hello` | `Hello` | what this connection is talking to |

## Hello

**snapshot.** The answer to `session.hello`
([#212](https://github.com/MeshBench/meshbench/issues/212)). Read once at
connect and cached; a client compares it before doing anything else.

| field | type | meaning |
|---|---|---|
| `Protocol` | `int` | wire version, bumped only on a breaking change |
| `Version` | `string` | the build, as `version.Detail()` gives it |
| `Mode` | `string` | `workbench` or `headless` |
| `Socket` | `string` | the path being answered on |
| `Verbs` | `int` | how many are registered |
| `PID` | `int` | so a restart is visible |
| `StartedAt` | `time.Time` | likewise |

---

# The network

## Node

**live.** The one type most scripts spend their time in.

It merges what the tree keeps in two: `state.Node` is what a network *is* and
`state.NodeStat` is what it is *doing*, split so a counter moving does not
republish the whole network. That reason is about the store's publishing cost
and does not survive into a client, where a script asking `node.running` should
not have to know which slice of the snapshot the answer lives in. The façade
reads both and presents one object. **The split stays where it earns its
keep** — the two verbs are still separate on the wire, and the raw snapshot
still carries two lists.

### What it is — from `state.Node`

| field | type | settable | meaning |
|---|---|---|---|
| `Name` | `string` | no | its name, and its identity everywhere |
| `Kind` | `Kind` | no | see [Kind](#kind) |
| `Lat`, `Lon` | `float64` | via `Move` | where it is |
| `HeightM` | `float64` | no¹ | antenna height above ground |
| `TxDBm` | `float64` | no¹ | transmit power |
| `Board` | `string` | yes² | the hardware it is, `""` for a host build; `node.Firmware.Board` is the same fact seen from the build |
| `Firmware` | `Build` | yes | the build it will run — version, board and role together |
| `Regions` | `[]string` | yes | the regions it relays for |
| `DefaultScope` | `string` | no | the region it originates under |
| `TrueRF` | `bool` | yes | takes waveform verdicts whatever the run's mode |
| `AllowFlood` | `bool` | yes | relays floods it has no region for |
| `Selected` | `bool` | via `Select` | part of the current selection |
| `Pattern` | `[]float64` | no | antenna gain in dBi every 10° from north |
| `Antenna` | `Antenna` | yes³ | what the antenna is and where it points |

¹ No verb sets height or power on an existing node today; both are settable at
`Place`. Named here as a gap rather than left to be discovered — it belongs
with the other missing verbs in
[#216](https://github.com/MeshBench/meshbench/issues/216).

² `node.set_firmware` takes `board` and `role` beside the version as of
`210d9ec`, because a board image is not a build on its own — "wadamesh" means
nothing until it is wadamesh for a LilyGo_TDeck, built as a companion. What is
still missing is a board on `nodes.place`, which is #216.

³ Through `SetAntenna` and `Aim`, not by assignment. The fleet-level form is
`wb.nodes.set_antenna(...)`, which is what a 58-node scenario needs: the same
verb with the node filter left off.

### What it is doing — from `state.NodeStat`

| field | type | meaning |
|---|---|---|
| `Running` | `bool` | a firmware process is up |
| `State` | `string` | `running`, `stopped`, `stopping`, `provisioning`, `starting` — a boolean cannot say "changing firmware" |
| `Backend` | `string` | `native`, `emulated`, or `""` for a node running nothing |
| `PID` | `int` | the process |
| `RSSBytes` | `int64` | resident memory |
| `CPUPct` | `float64` | share of one core since the last sample |
| `CPUms` | `int64` | total processor time since it started |
| `Sent`, `Heard` | `int` | packets |
| `LastSentMs`, `LastHeardMs` | `uint32` | when, in simulated time; zero means never |
| `LastSentTo`, `LastHeardFrom` | `string` | and with whom |
| `IRQReads`, `BusyReads` | `uint32` | the chip's own counters — the only way to tell a busy mesh from a radio that cries busy |
| `BusyMs`, `Spurious` | `uint32` | likewise |
| `Radio` | `RadioState` | what the chip is actually set to |
| `Screen` | `*Screen` | the last picture its display sent, or `nil` |

### Methods

| call | verb | notes |
|---|---|---|
| `Start()` | `node.start` | |
| `Stop()` | `node.stop` | |
| `Delete()` | `nodes.delete` | rebuilds the scenario and re-warms the matrix |
| `Move(lat, lon)` | `nodes.move` | |
| `Select(add=False)` | `nodes.select` / `nodes.add_to_selection` | |
| `SetFirmware(build, apply=True)` | `node.set_firmware` / `_only` | `apply` stops, provisions and restarts; without it the build is recorded and takes effect at the next start |
| `AdoptRadio()` | `node.radio_adopt` | take the chip's reported power as the scenario's |
| `Antenna()` → `Antenna` | `node.antenna` | which sort, its numbers, and which way it faces |
| `SetAntenna(change)` | `nodes.antenna` | what is left nil is left alone, so turning a beam does not restate the beam |
| `Aim(at)` → `Aimed` | `node.aim` | point it at another node; the reply says what the turn won |
| `Inject(payload=None)` | `sim.inject` | originate without firmware: exercises the radio model, not relaying |
| `Energy()` → `Energy` | `node.energy` | |
| `Coverage()` → `Coverage` | `coverage.compute` | |
| `Provisioning()` → `[]ProvisionLine` | `node.provisioning` | |
| `Serve(kind)` → `Endpoint` | `bench.serve` | `tcp` or `serial` |
| `Unserve()` | `bench.drop` | |
| `ServeSDR()` → `SDRSource` | `sdr.serve` | observers only |
| `WaitRunning(timeout)` | polls `firmware.state` | see the waiting rules in the API document |

### Sub-objects

| field | type | present when |
|---|---|---|
| `Console` | `Console` | the node runs a firmware with a text console |
| `Companion` | `Companion` | `Kind == Companion` |
| `Board` | `Board` | the node's board declares a screen, lamps, buttons or a keyboard |

### Kind

From `scenario.Kind`. A string on the wire; a generated enum in both clients,
alongside `Board`, `Preset`, `Role`, `Class`, `Tab`, `Strategy` and
`Transport`. All eight come out of `tools/clientgen`, so the two clients cannot
drift and CI fails if either is stale — a constant naming a board the simulator
has never heard of is worse than a string, because it looks checked.

| value | what it is |
|---|---|
| `simple-repeater` | forwards, nothing else |
| `advanced-repeater` | forwards, serves clients, holds state |
| `companion` | a user's device — the thing a phone connects to |
| `room-server` | holds posts for clients to collect and **does not forward** — a mesh that treats it as a repeater overstates its own reach |
| `sdr-observer` | runs no firmware, transmits nothing, hands back IQ |
| `emitter` | external interference: a mast carrying something that is not MeshCore |

### Antenna

**snapshot.** `node.Antenna()`. What a node stands under, in the same words the
verb that sets it takes, so what comes back can be handed straight back in.

| field | type | meaning |
|---|---|---|
| `Pattern` | `string` | `isotropic`, `dipole`, `collinear`, `yagi`, or `""` for a node with no antenna at all |
| `GainDBiPeak` | `float64` | the headline figure, for a collinear or a yagi |
| `BeamwidthDeg` | `float64` | a yagi's horizontal half-power beamwidth |
| `FrontToBackDB` | `float64` | how far down its back is on its front |
| `BearingDeg` | `float64` | compass bearing of boresight, 0 at north |
| `DowntiltDeg` | `float64` | degrees the beam is tilted below the horizon |
| `Polarisation` | `string` | `vertical`, `horizontal`, `circular`, or `""` for unstated |
| `FeedlineDB` | `float64` | cable and connector loss, a positive number |
| `PeakDBi` | `float64` | what the pattern manages on its own boresight, before the feedline |

`""` for the pattern is not an omni at 0 dBi. It is a node that has no antenna,
which the engine credits no gain and the map draws no overlay for, and it is
reported rather than filled in.

Polarisation is priced against the far end, not on its own: unstated costs
nothing, 3 dB circular against linear, 20 dB vertical against horizontal. See
`docs/shortcomings.md` §1.8 for what that figure is and is not.

### Aimed

**snapshot.** What `node.Aim(at)` answers with.

| field | type | meaning |
|---|---|---|
| `Node`, `At` | `string` | which antenna was turned, and at what |
| `BearingDeg` | `float64` | where it now points |
| `DistanceKm` | `float64` | how far away the far end is |
| `GainDBi` | `float64` | what this node now manages towards it, feedline deducted |

`GainDBi` is the point of it. On an omni the answer is the same as before, and
a call that reported success while changing nothing would be one to distrust.

## NodeSet

**live.** `wb.Nodes`. Iterable, indexable by name, filterable.

| call | verb | notes |
|---|---|---|
| iterate / `len` | `nodes.list` | |
| `["Name"]` | — | from the snapshot |
| `Place(name, kind, lat, lon, height_m=, tx_dbm=, board=)` | `nodes.place` | `board` is a `Board`, refused if nothing matches |
| `PlaceMany([...])` | `nodes.place` × n | one warm at the end, not one per node |
| `Delete(*names)` | `nodes.delete_many` | all or none: a name that is not there removes nothing |
| `Keep(*names)` | `nodes.keep` | the complement, worked out at the workbench |
| `Search(query, limit)` | `nodes.search` | ranked, emoji and accents folded; `NameMatch` |
| `Find(query)` | `nodes.search` | the one it meant, or a refusal naming the near misses |
| `Near(node, count)` | `nodes.near` | the closest, on the same great circle the path losses use |
| `Select(*names, add=False)` | `nodes.select_many` | |
| `Selected` | — | who is selected now |
| `OfKind(kind)`, `Running`, `Stopped` | — | filters, evaluated client-side |
| `Stats()` | `nodes.stats` | the rows, not a count of them |
| `SetAntenna(kind, change)` | `nodes.antenna` | the fleet-level default: every node, or every node of one kind |

Names and handles are interchangeable wherever a node is named: `Search` and
`Near` hand back handles and every verb takes a name, so the client converts
rather than making each caller do it.

## Link

**snapshot.** From `state.Link`. Indices resolved to names by the façade,
because an index into a list a script does not hold is not an answer.

| field | type | meaning |
|---|---|---|
| `A`, `B` | `string` | the two ends, by name |
| `MarginDB` | `float64` | the **weaker** direction's margin above what that end needs |
| `AtoB`, `BtoA` | `float64` | the two directions separately |
| `Known` | `bool` | false when nothing has computed a margin — not the same as zero, and must never be drawn as one |

**Reachability is asymmetric.** `MarginDB` is the weaker of the two and is
exactly the number that hides the asymmetry, so the façade returns all three
and any helper that prints a link prints both directions.

## Budget

**snapshot.** From `state.Budget`. One direction of one link, broken down.

| field | type | meaning |
|---|---|---|
| `From`, `To` | `string` | the direction this is |
| `Terms` | `[]BudgetTerm` | `{Name string; DB float64}`, in order |
| `MarginDB` | `float64` | the running total after every term |
| `Provenance` | `Provenance` | what model produced it |

Carried as terms rather than a total because the total is the one thing already
on the map; what a budget is *for* is which term is the reason.

## Profile

**snapshot.** From `state.Profile`. The cut-through between one pair.

| field | type | meaning |
|---|---|---|
| `From`, `To` | `string` | |
| `DistanceKm` | `float64` | |
| `AtoB`, `BtoA` | `float64` | the one-way margins |
| `Samples` | `[]ProfileSample` | `{DistM, GroundM, BulgedM, LOSm, FresnelM}` — `BulgedM` is the ground with the earth's curvature in it |
| `Edges` | `[]ProfileEdge` | `{DistM, LossDB}` — a decomposition, never an addition |
| `Worst` | `ProfileWorst` | `{DistM, ClearanceM, FresnelPct, Blocked}` — where the path comes closest to failing |
| `Verdict` | `string` | |
| `LowM`, `HighM` | `float64` | the vertical extent, for drawing |
| `Assumed` | `string` | which loss model these margins came from — a margin whose provenance is silent reads as measured |

---

# Running it

## Sim

**live.** The clock.

| field | type | settable | meaning |
|---|---|---|---|
| `Playing` | `bool` | via `Play`/`Pause` | the engine is advancing |
| `NowMs` | `uint32` | no | simulated time, which is not wall time |
| `RunUntilMs` | `uint32` | no | where an open `Run` stops; zero means open |
| `StepMs` | `uint32` | yes | how much simulated time one tick advances |
| `Seed` | `uint64` | yes | same seed, same scenario, same result |
| `RealtimeX` | `float64` | no | how fast the run is moving against the wall clock |
| `RealFirmware` | `bool` | yes | whether play starts MeshCore on every node |
| `FirmwareRunning` | `int` | no | how many processes are up |
| `FirmwareStarting` | `bool` | no | a start is in progress |

Methods: `Start`, `Play`, `Pause`, `Toggle`, `Step`, `Run(ms=|seconds=|minutes=)`,
`Settle(steps=)`, `Reset`, `Faster`, `Slower`, `State`.

## Event

**snapshot.** From `state.Event`. One thing the engine did.

| field | type | meaning |
|---|---|---|
| `AtMs` | `uint32` | simulated time |
| `Kind` | `string` | `tx`, `rx`, and the rest |
| `From`, `To` | `string` | |
| `MessageID`, `PacketID` | `uint64` | a message survives relaying; a packet is one transmission of it |
| `SNRdB` | `float64` | |
| `Detail` | `string` | |
| `Class` | `string` | `sent`, `received`, `half-duplex`, `interference`, `collision`, `receiver-busy`, `floor`, `unclassified` |

The frame bytes are deliberately absent: a hundred thousand events each
carrying a frame is real memory for something only the packet view opens. Ask
`wb.Packets.Open(id)` for the one you want.

## EventCounts

**snapshot.** `{Sent, Received, HalfDuplex, Interference, Collision, ReceiverBusy,
Floor, Unclassified int}` plus `Total()`. Counted by the engine as they happen,
never by walking the log. `Floor` means the signal was measured under the
demodulator's threshold, and nothing else is counted there: a miss whose cause
the engine did not establish is `Unclassified`, because reading it as a weak
signal is what sends somebody out to buy antennas for a busy channel.

## Packet

**snapshot.** From `state.Packet`. One transmission, dissected, with everywhere
it went — the view a real capture cannot produce, because no observer is
everywhere.

| field | type | meaning |
|---|---|---|
| `ID`, `MessageID` | `uint64` | |
| `Origin` | `string` | |
| `AtMs` | `uint32` | |
| `Heard`, `Missed` | `int` | |
| `Malformed` | `string` | the dissection's complaint, empty when it parsed |
| `RouteType`, `PayloadType`, `Version`, `Transport` | `string` | the header in the dissector's words |
| `Path` | `[]string` | hop hashes resolved to names where the run knows them — approximate by construction, and labelled where it fails |
| `Hops` | `int` | the frame's own count, off the path-length byte; **not** `len(Path)` |
| `PayloadFields`, `PathFields` | `[]PacketField` | |
| `PayloadNote` | `string` | what to say when the payload carries nothing readable |
| `Spans` | `[]PacketSpan` | the frame's shape, tiling every byte |
| `Readable` | `string` | how much of this payload type can be read at all |
| `Scope` | `PacketScope` | |
| `Raw` | `[]byte` | the frame |
| `RawLines` | `[]string` | it, as a hex dump |
| `Fates` | `[]PacketFate` | what every node that logged an event did |
| `Ledger` | `[]PacketReception` | radio-level truth per receiver, collapsed to the one answer that matters |
| `LedgerFull` | `[]PacketReception` | every attempt, uncollapsed |
| `Journey` | `[]PacketHop` | the message followed across every relay |
| `Transmissions`, `Reached` | `int` | |

### PacketField

`{Name, Value, Decoded, Description string; Offset, Size int}` — `Decoded` is
empty when the raw value is already the readable one.

### PacketSpan

`{Name string; Offset, Size int; Detail string}`.

### PacketScope

| field | type | meaning |
|---|---|---|
| `Scoped` | `bool` | the frame carries a scope code at all |
| `Name` | `string` | the candidate whose key reproduces the code, when one did |
| `Code` | `string` | the code itself |
| `Candidates` | `int` | how many names were checked |
| `Note` | `string` | codes `{0,0}`, which the firmware treats as addressed to nowhere |

A region key is never in the packet, so a scope can only be **confirmed**
against a candidate name. Not matching means we did not hold the name — which
is not the same as the packet having no scope.

### PacketFate

`{AtMs uint32; Node, Kind string; SNRdB float64; What string}`.

### PacketReception

| field | type | meaning |
|---|---|---|
| `Node`, `From` | `string` | |
| `Offered` | `bool` | anything measurable arrived |
| `RSSIdBm`, `SNRdB` | `float64` | |
| `Demod`, `CRCOK` | `bool` | |
| `Firmware` | `string` | `accepted`, `dropped`, `never saw it` |
| `Why` | `string` | the engine's own words when it was offered and still failed |
| `Hop` | `int` | which transmission of the journey this answers for |

This is the type to reach for when a test wants to say *why* something did not
arrive. It is the reception ledger, and it is the reason a scripted assertion
can fail with a diagnosis rather than a boolean.

### PacketHop

`{AtMs uint32; By string; Hops int; Heard, MissedBy, MissWhy []string; Missed int; PacketID uint64}`.
`MissWhy` is parallel to `MissedBy`.

---

# Firmware and hardware

## Build

**snapshot.** From `state.FirmwareRow` merged with `state.Build` — the library
row and the file on disk are one thing to a script.

| field | type | meaning |
|---|---|---|
| `Role` | `string` | `repeater`, `companion`, `room-server` |
| `Version` | `string` | the tag or the imported name |
| `Board` | `string` | empty for a host build; set means an emulated image |
| `Bytes` | `int64` | |
| `OnDisk` | `bool` | |
| `Path` | `string` | where it lives, empty when not on disk |
| `InUse` | `int` | how many nodes in this scenario run it |
| `Unavailable` | `bool` | exists only because nodes are pinned to it — nothing on disk, nothing published |
| `Native` | `bool` | a host build, the inverse of having a board |

`Unavailable` matters to a script: pinning a node to such a build succeeds and
then fails at start, which reads as the library losing builds rather than as a
pin nobody can honour.

## FirmwareLibrary

**live.** `wb.Firmware`.

| call | verb |
|---|---|
| iterate / `len` | `firmware.library` |
| `Installed()` | `firmware.installed` |
| `Scan()` | `firmware.rescan` |
| `Needed()` → `[]RoleNeed` | `firmware.needed` |
| `Download(role, version, board=)` → `Job` | `firmware.download` |
| `Import(path, role, board=)` → `Build` | `firmware.import` |
| `Build(checkout, roles=)` → `map[Role]Build` | **new**, #216 |
| `Delete(build)` | `firmware.delete` |
| `Wipe()` | `firmware.wipe` |
| `Use(version, role=|node=)` | `firmware.set` |
| `Start()` → `Job` | `firmware.start` |
| `State()` | `firmware.state` |
| `WaitStarted(timeout)` | polls `firmware.state` |

`RoleNeed` is `{Role string; Nodes int; Choices []string}`.

## RadioState

**snapshot.** From `state.RadioState`. What a node's chip is actually set to,
as the firmware left it — not what the board profile claims it can do. The two
diverge whenever the firmware has a fault, and until this existed there was
nowhere to see that they had.

| field | type | meaning |
|---|---|---|
| `Reported` | `bool` | the node has said anything at all — a node that has not come up must not read as one configured to zero |
| `GainReg` | `uint8` | `0x08AC`: `0x96` boosted, `0x94` power saving |
| `Boosted` | `bool` | |
| `TxPowerDBm` | `int8` | |
| `FemLive` | `bool` | the front-end module's enable line now |
| `FemAtTx` | `uint8` | where it stood when this node last began transmitting — the one that decides how much power left the board |
| `Mode` | `uint8` | 0 standby, 1 rx, 2 tx, 3 cad |
| `SF`, `CR` | `uint8` | |
| `FreqHz`, `BandwidthHz` | `uint32` | |
| `PreambleSyms` | `uint16` | |
| `IRQMask`, `IRQFlags` | `uint16` | what was allowed to raise DIO1, and what is raised now — the pair tells a node stuck on a flag from one with nothing to say |

Reported raw and presented raw. The question this answers is "is this node set
to what I think it is", and a value translated on the way loses the ability to
answer it.

## Board

**live.** `node.Board`, present when the node's board declares anything to
interact with.

| field | type | meaning |
|---|---|---|
| `Name` | `string` | the profile, e.g. `LilyGo_TDeck` |
| `MCU`, `Radio`, `Vendor` | `string` | |
| `HasScreen`, `HasKeyboard`, `HasTouch` | `bool` | |
| `Buttons` | `[]BoardButton` | `{Name string; Pin int; ActiveLow bool}` |
| `Screen` | `*Screen` | the last picture, or `nil` |

| call | verb |
|---|---|
| `Press(pin, down)` | `board.press` |
| `Tap(pin, hold=)` | `board.press` × 2 |
| `Type(text)` | `board.key` |
| `Touch(x, y, down)` | `board.touch` |
| `TapAt(x, y)` | `board.touch` × 2 |

Held rather than clicked, because the firmware behind these pins cares:
MeshCore wakes a sleeping display on a press and powers the board off on a long
one, and a call that could only produce a tap could reach neither.

## Screen

**snapshot.** From `state.Screen`.

| field | type | meaning |
|---|---|---|
| `Width`, `Height` | `int` | |
| `On` | `bool` | the display's own power state |
| `BPP` | `int` | 1 monochrome, 16 colour (RGB565) |
| `Bits` | `[]byte` | as the controller holds them: byte *n* is eight vertical pixels of column *n%Width* in page *n/Width* |

Helpers: `Lit(x, y) bool` for monochrome, `At(x, y) (r, g, b, ok)` for colour,
and `PNG()` for a script that wants to save it.

A blank picture with `On == false` is a sleeping board; a blank one with
`On == true` is a board that cleared its screen. Reporting them the same way
calls the first broken.

## Boards

**live.** `wb.Boards`: `List()`, `Matrix(version)` → `[]BoardRow`,
`Probe(board, version)` → `Job`.

`BoardRow` is `{Board, Version string; Cells []BoardCapabilityCell; Stale bool;
MeasuredAt string}`, and a cell is `{Capability, State, Detail string}` where
state is `untested`, `passed`, `failed` or `n/a`.

**Probes run one at a time.** Several emulated boards at once will take a
twelve-core machine down; the façade serialises them and says so rather than
letting a script fan out.

---

# Companion

## Companion

**live.** `node.Companion`. From `state.Companion`.

| field | type | meaning |
|---|---|---|
| `Connected` | `bool` | the workbench holds the port |
| `Name`, `Key` | `string` | what the node said about itself, which can differ from what the scenario believes — and when it does, the node is right |
| `FreqKHz`, `BWKHz` | `uint32` | |
| `SF`, `CR`, `TxDBm` | `uint8` | |
| `MaxTxDBm` | `uint8` | the ceiling the radio reports |
| `PathHashBytes` | `int` | 1, 2 or 3; **zero means the node has not said** — firmware older than v10 does not report it |
| `Scope` | `string` | the region it sends under |
| `Scoped` | `bool` | empty scope and unknown scope are different things |
| `Channels` | `[]CompanionChannel` | |
| `Contacts` | `[]CompanionContact` | |
| `Messages` | `[]CompanionMessage` | oldest first |
| `Serving` | `Endpoint` | set when the port has been handed to an outside client |

| call | verb |
|---|---|
| `Connect()` / `Disconnect()` | `companion.connect` / `.disconnect` |
| `Refresh()` | `companion.refresh` |
| `Send(text, channel=, path_hash=)` | `companion.send` |
| `Advert(flood=False)` | `companion.advert` |
| `AddChannel(index)` | `companion.add_channel` |
| `Configure(name=, lat=, lon=, tx_dbm=, path_hash=)` | `companion.configure` |
| `Raw(bytes)` | `companion.raw` |
| `CLI(line)` | `console.cli` |
| `Scope = name` | `companion.scope` |

### CompanionChannel

`{Index uint8; Name string; Unread int}`.

### CompanionContact

`{Name, Key string; Hops int; LastSeen time.Time}` — `Hops` is `-1` when no
path is established. A contact with no path is still a contact.

### CompanionMessage

| field | type | meaning |
|---|---|---|
| `Channel` | `bool` | channel message rather than direct |
| `ChannelIdx` | `uint8` | |
| `From`, `Text` | `string` | |
| `At` | `time.Time` | |
| `Mine` | `bool` | this client sent it — nothing echoes a sent message, so without this a conversation stays empty |
| `SNRdB` | `float64` | |
| `Hops` | `int` | |
| `Receipt` | `string` | what became of a sent message: how far it went, how many heard it. The simulator can answer this and a real phone cannot |
| `Failed` | `bool` | the receipt is bad news |

## Console

**live.** `node.Console`.

| call | verb | notes |
|---|---|---|
| `Send(line)` | `console.type` | |
| `Read()` → `[]string` | `console.read` | the scrollback |
| `Tail` | `console.read` | the last line |
| `Ask(line, timeout=)` → `string` | `console.type` + step + `console.read` | the whole point: a node answers on its **next loop**, and its loop only runs when the engine steps |

`Ask` exists because reading straight after sending reads the moment before the
command was sent. Every script that has done this by hand has got an empty
reply and concluded the console was broken.

## Fleet

**live.** `wb.Fleet.Send(command, kind=|node=)` → `[]FleetReply`.

`FleetReply` is `{Node, Reply string}` — per node rather than merged, because
the answer worth having is which node disagreed with the others.

The façade waits for the replies. A fleet command is answered on each node's
next loop, so `fleet.send` returns before anything has been said and
`fleet.replies` is what collects them; a script should never see that seam.

---

# Study

## Coverage

**snapshot.** From `state.Coverage`.

| field | type | meaning |
|---|---|---|
| `Node` | `string` | where it is coverage *from* |
| `South`, `North`, `West`, `East` | `float64` | the raster's bounds |
| `Cells` | `int` | |
| `NoDataCells` | `int` | had no elevation to answer with |
| `Image` | image | as PNG bytes to a client |
| `Provenance` | `Provenance` | |

`NoDataCells` is carried because "no coverage" and "no data" look identical on
a map and are not the same claim.

## Energy

**snapshot.** From `state.Energy`.

| field | type | meaning |
|---|---|---|
| `Node` | `string` | |
| `DutyPct` | `float64` | |
| `SoC` | `[]float64` | the **daily minimum** state of charge, one per day |
| `WorstSoC`, `WorstDay` | `float64`, `int` | |
| `DeadDays` | `int` | |
| `AutonomyDays` | `float64` | |

Daily minimum, not daily mean: a pack that averages half full and empties every
night at three is a pack that does not work, and the mean is the number that
hides it.

## Route

**snapshot.** `{NewSites, Hops int; LongestHopKm float64; Through string}`.

## Study

**live.** `wb.Study`.

| call | verb |
|---|---|
| `MarginKm = n` | `study.margin` |
| `Coverage(node=|mode=)` → `Coverage` | `coverage.compute` / `.start` / `.combined` |
| `CoverageCells = n` | `coverage.resolution` |
| `ClearCoverage()` | `coverage.clear` |
| `Energy(node=)` → `Energy` | `energy.for_selection` |
| `Plan(a, b)` → `[]Route` | `plan.routes` |

---

# Measurement against reality

## Observed

**snapshot.** From `state.Observed`. One reception on the real network.

`{At time.Time; Receiver, Origin, Transmitter string; HopCount int;
HasSNR bool; SNRdB float64; PacketID string}`.

`Transmitter` is who put *this copy* on the air — the RF endpoint the SNR
belongs to, whatever the hop count.

## Residuals

**snapshot.** From `state.Residuals`. The model measured against those
receptions: a bias and a spread rather than a verdict, because "3 dB optimistic
on this network" is something somebody can correct for and "validation failed"
is not.

| field | type | meaning |
|---|---|---|
| `Matched`, `Unmatched` | `int` | |
| `OffScenario` | `int` | named a node this scenario has not got — a scope problem |
| `NoLink` | `int` | both nodes here but no measured link — a warm-up problem |
| `Censored` | `int` | prediction past the modem's reporting ceiling: a bound, not a number, so counted and left out of the fit |
| `MedianDB` | `float64` | positive means the model predicts more margin than was observed |
| `IQRdB` | `float64` | half the residuals within this |

`OffScenario` and `NoLink` are separate because their sum looking like one
number is how a matching failure stayed undiagnosed.

---

# Experiments

## ArmSummary

**snapshot.** One arm, averaged over its seeds.

| field | type | meaning |
|---|---|---|
| `Arm` | `string` | |
| `Runs` | `int` | |
| `TX`, `RX`, `Delivered`, `Redundant`, `Collided` | `float64` | |
| `AirtimeMs` | `float64` | |
| `RXSpread` | `float64` | how much the seeds disagree, as a fraction of the arm's own mean — half the range, so it reads as a ± |
| `PerSecond` | `[]int` | receptions in each second after the burst: the shape of the flood rather than its total |

`RXSpread` of zero means every seed returned the same number, which is **one
draw repeated rather than a spread**. A difference between arms cannot be
called larger than a noise nobody has measured, and the façade's `Compare`
refuses to call a winner without it.

## RunRow

`{Arm string; Seed uint64; State string; Result string}` where state is
`queued`, `running`, `done` or `failed`. Per run rather than per arm, because a
sweep is watched while it runs and an arm summary says nothing until every seed
has finished.

## Matrix

`{Metric string; Arms []string; Seeds []uint64; Values []float64}`, row-major,
arms down and seeds across. **NaN marks a cell that was not run** — not zero: a
run that did not happen and a run that measured nothing are different claims.

---

# The machine

## Job

**live.** From `state.Job`. A long operation in flight.

| field | type | meaning |
|---|---|---|
| `ID` | `string` | |
| `What` | `string` | |
| `Done`, `Total` | `int` | |
| `Finished` | `bool` | |
| `Cancellable` | `bool` | |

Methods: `Cancel()` (`job.cancel`), `Wait(timeout)`. A job with no cancel is
one that cannot be interrupted safely, and the verb refuses by name rather than
silently doing nothing.

## GPUState

**snapshot.** `{Enabled, Present bool; Device, Backend, Why string; Used bool;
Pairs int; CellM float64; Ms int64}`.

`Used` and the rest describe the **last warm**, not the setting. "GPU
acceleration: on" over a run that quietly fell back to the cores is exactly the
claim this project does not make, so a script reads `Used`.

## ResourceRow

`{Kind, Name, Version, Path string; Bytes int64; Estimated bool; State, Why
string; Auto, Fetchable, Licensed bool}`. `Estimated` says whether `Bytes` was
measured or guessed. `Auto` is whether the application may fetch it unasked —
not the same question as whether it is present, which is `State`.

## Endpoint

`{Node, Kind, Addr string; Addrs []string; Attached bool}`. `Addr` is where to
point a client — the machine's own address, never the `0.0.0.0` a listener was
bound to. `Addrs` is every address it answers on, because which one the far end
can reach is not this program's to guess.

## SDRSource

`{Node, Addr string; RateHz float64; Attached bool}`.

---

# Honesty

## Provenance

**snapshot.** Attached to every result that is a measurement.

| field | type | meaning |
|---|---|---|
| `RFMode` | `string` | `calculated` or `waveform` |
| `Realism` | `RFRealism` | all zero means the kind default |
| `ExcessLossDB` | `float64` | the calibration term in force |
| `Calibrated` | `bool` | fitted against real receptions, or left at the default |
| `Environment` | `string` | the building-tile directory, `""` for bare earth |
| `Permissive` | `bool` | the fixture answers reach more generously than the real network |
| `Seed` | `uint64` | |
| `Caveats` | `[]string` | the standing ones from `docs/shortcomings.md` |

This type is the reason the whole API exists in the shape it does. A scripted
result gets pasted into a report with the caveats stripped, so the caveats have
to be **in the value**, not beside it. `Provenance.String()` is a one-line
summary meant to be printed above any number a script emits, and the pytest
plugin puts it in the failure output whether or not the test asked.

`RFRealism` is `{OscPPM, MultipathDB, FadingHz, ImplLossDB, SaturationDBm
float64}`.

## Report

**snapshot.** What `wb.Assertions.Check()` returns.

| field | type | meaning |
|---|---|---|
| `Passed`, `Total` | `int` | |
| `Results` | `[]AssertionResult` | |
| `Provenance` | `Provenance` | |

`AssertionResult` is `{Kind, Node string; Pass bool; Got, Want string}`.
Methods: `WriteJUnit(path)`, `Why` — the latter builds a diagnosis out of the
reception ledger rather than reporting `False`.

**An assertion whose kind this build does not understand fails.** It does not
quietly pass; that is a green run that checked nothing. Kinds today are
`delivered` / `deliveries` / `unique_deliveries`, and `sent` / `transmissions`.

---

# The window

Everything here exists only in a windowed session. In a headless one the verbs
already refuse with *"this session has no interface attached, so there is
nothing to show"* — 23 of them, all gated on the same check in
`internal/app/session/ui.go` — and `wb.UI` is `nil`, so a script that touches
it fails at the client rather than at the socket.

## UI

`View`, `Panels`, `Panel(name)`, `Window(name)`, `Layouts`, `Scale`, `Map`,
`State()`.

Views: `Plan`, `Run`, `Debug`, `Validate`, `Bench`, `App`.

## Map

`Fit()`, `Centre(node=|lat=, lon=, zoom=)`, `Zoom(factor)`, `Layers` (a
settable mapping), `Filter`, `Basemap`, `Tool`, `Shade()`.

Layers: `Regions`, `Links`, `Coverage`, `Terrain`, `Buildings`, `Antenna`,
`Measure`. Tools: `move`, `place`, `link`, `measure`.

---

# Enumerations

Constants in both clients, so a typo is a compile error in Go and an
autocompletion in Python rather than a verb refusing at run time.

| set | values |
|---|---|
| `Kind` | `simple-repeater`, `advanced-repeater`, `companion`, `room-server`, `sdr-observer`, `emitter` |
| `RFMode` | `calculated`, `waveform` |
| `Backend` | `native`, `emulated`, `""` |
| `NodeState` | `running`, `stopped`, `stopping`, `provisioning`, `starting` |
| `EventClass` | `sent`, `received`, `half-duplex`, `interference`, `collision`, `receiver-busy`, `floor`, `unclassified` |
| `AssertionKind` | `delivered`, `deliveries`, `unique_deliveries`, `sent`, `transmissions` |
| `ServeKind` | `tcp`, `serial` |
| `View` | `Plan`, `Run`, `Debug`, `Validate`, `Bench`, `App` |
| `Board` | 25 profiles — `Ebyte_EoRa-S3`, `Generic_E22_sx1262`, `Heltec_E213`, `Heltec_E290`, `Heltec_mesh_solar`, `Heltec_t096`, `Heltec_t114`, `heltec_tracker_v2`, `Heltec_v2`, `Heltec_v3`, `heltec_v4`, `Heltec_Wireless_Paper`, `Heltec_Wireless_Tracker`, `Heltec_WSL3`, `LilyGo_T3S3_sx1262`, `LilyGo_TBeam_1W`, `LilyGo_TDeck`, `RAK_3112`, `RAK_4631`, `Station_G2`, `Station_G3_ESP32`, `Tbeam_SX1262`, `Xiao_nrf52`, `Xiao_S3`, `Xiao_S3_WIO` |
| `RadioPreset` | 20 community presets, default `EU/UK (Narrow)` — the list is an *agreement between operators*, so it is baked in rather than fetched |

Board and preset lists are generated from `internal/world/scenario`, not
transcribed. A hand-copied list of boards is a list that is wrong the week
somebody adds one.
