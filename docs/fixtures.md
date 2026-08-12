# The shipped fixtures

Networks you can load and run without importing anything. Built from live
CoreScope, with the regions the real nodes actually hold, so a result on one of
these is a result about a real topology rather than about a lattice.

## fife, small

| | |
|---|---|
| built | 12 August 2026, from <https://scotmesh-corescope.mm7roq.compute.oarc.uk/> |
| boundary | Fife |
| nodes | 55 |
| kinds | 45 simple repeater, 9 companion, 1 SDR observer |
| regions | inferred from 20,500 packets over 168 hours, applied to 53 nodes |
| scopes seen | `#sco`, `#ioi` |
| radio | EU/UK (Narrow): 869.618 MHz, 62.5 kHz, SF8, CR4/8 |
| firmware | pinned per role at v1.17.0 |

The nine companions span 0.76 degrees of latitude and 0.69 of longitude, which
is most of Fife: they are where CoreScope found them, and they are not clustered.

### Two variants, and the difference matters

**`fixture-fife-strict`** carries the regions the real nodes hold and nothing
else. This is the one to use for "would this work on ScotMesh", because it
forwards what ScotMesh forwards and drops what ScotMesh drops.

**`fixture-fife-permissive`** is the same network with `region allowf *` applied
to all 54 repeaters, so a flood is forwarded whatever its scope. It exists so
that a first run works without anyone having to discover that scopes are written
`#sco` on the wire and `sco` at the console, and that a mesh with regions
inferred but not applied transmits everything and relays nothing while reporting
no error at all.

**It is more permissive than the real network.** A reach question answered on it
will be answered more generously than reality. Use strict for anything you plan
to believe.

## What is absent, and why

**No room server and no interferer.** `nodes.place` accepts repeater, companion
and observer, and refuses the rest: `unknown kind "emitter"; have repeater,
companion, observer`. Both kinds exist in the model and can be placed in the
application, so this is a gap in the control socket rather than in the
simulator, and it is why the fixture does not carry one of every kind as
intended.

**No sensor node.** There is no sensor kind in the model at all. MeshCore
publishes sensor builds and the firmware catalogue parses the role, but
`scenario.Kind` has repeaters, companion, room server, observer and emitter and
nothing else. Deliberately not added here.

**The medium and large fixtures are not built yet.** Scotland, and Scotland with
Ireland, are the same recipe with different boundaries and more waiting.

## How they were built

The order matters and every step in it has been skipped at least once, with the
failure looking like bad RF rather than a missing step:

    boundary.set → boundary.accept        once per region, the chosen set unions
    import.set_source → fetch → commit    strategy "replace-all"
    infer.run {hours:168} → infer.result → infer.apply
    firmware.set per node
    project.save

**`infer.apply` is the one that gets forgotten**, and it is the one that decides
whether anything relays.

**Pin firmware per node, not per role.** `firmware.set` with a role and no node
applies to every node that runs firmware *and sets its role*, so three calls in a
row do not pin three roles: they convert the whole mesh three times and the last
one wins. That happened while building this fixture and had to be repaired.
