# meshbench

Drive a [MeshBench](https://github.com/MeshBench/meshbench) workbench from
Python: real MeshCore firmware over a sample-accurate LoRa channel, scripted.

```python
from meshbench import Workbench

with Workbench.headless(fixture="fife-strict", seed=9001) as wb:
    wb.sim.run(minutes=5)
    print(wb.provenance())
    print(wb.events.total(), "events")
```

`headless` needs no display, no GPU and no toolkit. `Workbench.attach()`
connects to a workbench somebody is already looking at and never closes it.

## Installing

```
pip install meshbench
```

You also need the `meshcoresim` binary on `PATH` — the package drives it, it
does not contain it.

## What it looks like

```python
wb.project.new(place="Fife")
wb.nodes.place_many(
    [
        {"name": "R1", "kind": Kind.SIMPLE_REPEATER, "lat": 56.20, "lon": -3.20},
        {"name": "C1", "kind": Kind.COMPANION, "lat": 56.19, "lon": -3.17},
    ]
)
wb.sim.start()
wb.firmware.wait_started("10m")

node = wb.nodes["C1"]
node.firmware = wb.firmware.find("companion-v1.17.0")
print(node.console.ask("get region"))
```

`wb.call(verb, params)` is the whole API underneath, and stays public: anything
this package has not shaped is one line away rather than a blocker.

## Testing firmware with it

The package registers a pytest plugin, so there is no `conftest.py` to copy:

```python
def test_the_flood_reaches_glenrothes(meshbench):
    meshbench.project.open("fixtures/fixture-fife-strict.json")
    meshbench.sim.run(minutes=5)
    assert meshbench.events.total() > 0
```

One workbench for the whole run — starting firmware on a real mesh is minutes,
not milliseconds — with the scenario cleared between tests so reuse does not
leak. `--meshbench-socket` attaches to one you are already running instead.

**A failing test prints the provenance**, whether or not it asked. Somebody
reading a failed assertion about a mesh is deciding whether their firmware
change broke something, and they need to know what the run assumed first.

## Two things that will bite otherwise

**Simulated time is not your time.** `sim.run(minutes=5)` is five minutes of
the mesh's own clock. On 155 emulated nodes that is a great deal longer than
five of yours, which is why the wait is a separate argument.

**A node answers on its next loop.** Its loop only runs when the engine steps,
so reading a console straight after writing to it reads the moment *before* the
command was sent. Use `console.ask()`, which gives the mesh its own time first.

## Honesty

Every result comes out of a simulator that is **kinder than the air**: no
multipath, no body loss, no oscillator error. The measured biases are nearly
all in one direction, which is what makes a result usable — treat it as a best
case. `wb.provenance()` says what a given run assumed, and it is meant to be
printed above any number you publish.

## Licence

GPL-3.0-or-later, with the rest of MeshBench.
