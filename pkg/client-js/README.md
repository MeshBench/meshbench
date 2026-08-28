# MeshBench Node client

Drive a running MeshBench workbench from Node, over the same control socket the
[Go](../client-go) and [Python](../client-python) clients speak. The three are
peers — a companion app or a dashboard in the JavaScript world talks to a
workbench without shelling out to another language.

One ES module, no dependencies, no build. It uses Node's own `net`.

```js
import { Workbench } from "@meshbench/client"; // or "./meshbench.mjs" from a checkout

const wb = await Workbench.attach();
await wb.hello();                              // refuse a protocol mismatch up front

await wb.call("project.new", { place: "Fife" });
await wb.call("nodes.place", { name: "Alpha", kind: "simple-repeater", lat: 56.3, lon: -3.3 });

const state = await wb.call("sim.state");
console.log(state);

await wb.close();
```

`call(verb, params)` is the whole API; everything the workbench can do is a
verb, and the [control-socket reference](https://meshbench.github.io/meshbench-docs/reference-control.html)
lists them all. A verb the workbench refuses throws a `WorkbenchError` carrying
its `code`, so you can tell "no such node" from "the workbench is closing"
without matching prose.

## Connecting

`Workbench.attach()` finds the workbench the same way the other clients do, so
all three agree on one machine:

- `MESHBENCH_CONTROL_SOCKET` if set — a path, `tcp`, or `tcp:host:port`.
- otherwise a unix socket at `$XDG_RUNTIME_DIR/meshbench.sock` (Linux) or the
  per-user cache directory (macOS).
- Windows has no unix socket, so the workbench listens on loopback TCP and
  writes its address and a token to a rendezvous file; this reads that file and
  presents the token. Pass `{ socket: "tcp" }` or set the env var.

```js
const wb = await Workbench.attach({ socket: "/run/user/1000/meshbench.sock", timeoutMs: 5000 });
```

## Running

```bash
node examples/small-mesh-with-traffic.mjs   # needs a workbench running, and a display
node --test meshbench.test.mjs              # the client's own tests, no workbench needed
```

The tests stand up a fake workbench on a unix socket and check the wire — no
real workbench, no network.

## What a scripted result is

A number this prints is a simulated number, kinder than the air, exactly as it
is from the application. The limits travel with it — see
[what it does not do](https://meshbench.github.io/meshbench-docs/what-it-does-not-do.html).
