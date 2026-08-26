> **Working note, last true on 12 August 2026.** Kept for the thinking in it, not maintained as a description of the code. Where this disagrees with the tree, the tree is right; the authority is the fixtures themselves, under `fixtures/`.

# The shipped fixtures

Networks you can load and run without importing anything. Built from live
CoreScope, with the regions the real nodes actually hold, so a result on one of
these is a result about a real topology rather than about a lattice.

Three sizes, two variants each, all six in `fixtures/`.

| fixture | boundary | nodes | repeaters | companions | regions applied |
|---|---|---|---|---|---|
| fife | Fife | 58 | 45 + 1 advanced | 9 | 53 |
| scotland | Scotland | 161 | 141 + 1 advanced | 16 | 118 |
| scotland-ireland | Scotland + Ireland | 311 | 272 + 1 advanced | 35 | 207 |

Every one carries **one of each node kind that exists**: simple repeater,
advanced repeater, companion, room server, SDR observer, emitter. The imported
network supplies the repeaters and companions, because that is what ScotMesh
has; the other four are placed.

## Provenance

| | |
|---|---|
| built | 12 August 2026, from <https://scotmesh-corescope.mm7roq.compute.oarc.uk/> |
| inference | 168 hours, 37,870 packets |
| scopes seen | `#sco`, `#ioi`, `#ioi-admin`, `#fif`, `#wls`, `#noc`, `#per`, `#gla` |
| radio | EU/UK (Narrow): 869.618 MHz, 62.5 kHz, SF8, CR4/8 |
| firmware | pinned per node, per role, at v1.17.0 |

**Imported nodes are filtered at ±1 km position uncertainty**, which is why
Scotland is 157 imported nodes rather than the several hundred CoreScope lists.
A node whose position is a guess would answer a reach question with a guess.

**The placed four hold what their neighbours hold.** Each takes the regions a
majority of its ten nearest imported neighbours hold, and the default scope most
of them use. Not the fixture's busiest region: on a mesh spanning two islands
that would give a repeater in Edinburgh the region held in Wicklow, and a node
holding a region its neighbours do not is as silent as one holding none.

**The emitter is keyed 0% of the time.** It is there so the fixture carries one
of every kind, and it changes nothing until somebody turns it up.

**No sensor node.** There is no sensor kind in the model at all. MeshCore
publishes sensor builds and the firmware catalogue parses the role, but
`scenario.Kind` has repeaters, companion, room server, observer and emitter and
nothing else. Deliberately not added.

## Two variants, and what is proven about the difference

**`-strict`** carries the regions the real nodes hold and nothing else. This is
the one to use for "would this work on ScotMesh", because it forwards what
ScotMesh forwards and drops what ScotMesh drops.

**`-permissive`** additionally sets `AllowAnyFlood` on every transmitting node,
which issues `region allowf *` at boot: the wildcard is the parent of every
region, so a flood is forwarded whatever its scope. It exists so a first run
works without anyone having to discover that scopes are written `#sco` on the
wire and `sco` at the console, and that a mesh with regions inferred but not
applied transmits everything, relays nothing, and reports no error at all.

**What is proven, and what is not.** The two files genuinely differ - 56 of 58
nodes in the Fife pair, against a first attempt where the "permissive" fixture
was byte-identical to the strict one. The firmware accepts `region allowf *`
and answers `OK`. But a controlled run has **not** shown the permissive variant
relaying more: flooding a scope only one node holds gave 51 transmissions and
521 receptions strict against 51 and 520 permissive, which is the same answer
twice and far inside the ±20% measurement floor. Either the wildcard needs
`region put *` first, or that experiment was insensitive to the difference.
Until that is settled, treat `-permissive` as *declared* permissive rather than
*demonstrated* permissive, and treat strict as the one to believe either way.

## Running one

    meshbench test -fixture fixtures/fixture-fife-strict.json -junit out.xml

Real firmware on every node, the fixture's assertions checked, JUnit written,
and a non-zero exit if anything failed. A permissive fixture says so on its
first line, every time.

**The assertion is deliberately one.** "At least ten unique deliveries" fails
the moment a mesh stops relaying, which is the regression worth gating on. A
duty-cycle assertion is absent on purpose: the runner adverts every node inside
the first thirty seconds so a run has traffic quickly, and a busy repeater
relaying fifty-six adverts in that window reached 37% duty - far above anything
a real network shows, where adverts are hours apart. Asserting on that would be
asserting on the harness.

## How they were built

The order matters and every step in it has been skipped at least once, with the
failure looking like bad RF rather than a missing step:

    boundary.set → boundary.accept        once per region, the chosen set unions
    import.set_source → fetch → commit    strategy "replace-all"
    boundary.prune
    firmware.set per node
    infer.run {hours:168} → wait for the job → infer.apply
    nodes.place + nodes.regions           the four kinds the import has none of
    nodes.allow_flood                     for the permissive variant only
    project.save

**`infer.apply` is the one that gets forgotten**, and it is the one that decides
whether anything relays.

**Pin firmware per node, not per role.** `firmware.set` with a role and no node
applies to every node that runs firmware *and sets its role*, so three calls in
a row do not pin three roles: they convert the whole mesh three times and the
last one wins.

**A sweep's `experiment.base repeater_version` overrides a per-node pin.** It
left the room server looking for `simple_room_server` inside `repeater-v1.17.0`,
which does not publish one, and one node of fifty-six failed to start. Pin the
roles you are not varying, or leave `experiment.base` alone.

The scripts that drove all of this are in the session, not the repository: the
fixtures are the artefact, and they load with no network and no re-inference.
