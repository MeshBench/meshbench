# MeshBench Node client

Drive a running MeshBench workbench from Node, over the same control socket the
[Go](../client-go) and [Python](../client-python) clients speak. The three are
peers on the wire — one protocol, one handshake, one set of transports — but
not in shape: Go and Python each carry a generated façade with typed methods
and closed enums, and this one does not. See "What kind of client this is"
below for why, and what that means for a script.

One ES module, no dependencies, no build. It uses Node's own `net`.

```js
import { Workbench } from "@meshbench/client"; // or "./meshbench.mjs" from a checkout

const wb = await Workbench.attach();           // does the handshake itself

await wb.call("project.new", { place: "Fife" });
await wb.call("nodes.place", { name: "Alpha", kind: "simple-repeater", lat: 56.3, lon: -3.3 });

const state = await wb.call("sim.state");
console.log(state);

await wb.close();
```

`call(verb, params)` is the whole API; everything the workbench can do is a
verb, and the [control-socket reference](https://meshbench.github.io/docs/reference-control.html)
lists them all. A verb the workbench refuses throws a `WorkbenchError` carrying
its `code`, so you can tell "no such node" from "the workbench is closing"
without matching prose. Every call carries a timeout - see below - so a verb
the workbench never answers rejects instead of hanging the script forever.

## Installing

```
npm install @meshbench/client
```

You also need the `meshbench` binary: this package drives a workbench, it does
not contain one. Unlike the Go and Python clients it cannot start one either,
so put `meshbench` on `PATH`, run `meshbench workbench` or `meshbench headless`,
and attach to that. An `attach()` that times out on a machine with no workbench
running is this, and not a bug in the socket path.

## Connecting

`Workbench.attach()` finds the workbench the same way the other clients do, so
all three agree on one machine:

- `MESHBENCH_CONTROL_SOCKET` if set — a path, `tcp`, or `tcp:host:port`.
- otherwise a unix socket at `$XDG_RUNTIME_DIR/meshbench.sock` (Linux) or the
  per-user cache directory (macOS).
- Windows has no unix socket, so the workbench listens on loopback TCP and
  writes its address and a token to a rendezvous file; this reads that file and
  presents the token. Pass `{ socket: "tcp" }` or set the env var.

A unix socket path over 104 bytes is refused before it ever reaches `connect`,
the same limit and the same message the Go and Python clients give - the raw
OS error names neither the limit nor what to do about it.

`attach()` also does the protocol handshake itself, the same as Go and Python:
a workbench speaking a version this client does not understand is refused at
connect, not on whichever call happens to notice. `hello()` stays public for a
script that wants to re-check.

```js
const wb = await Workbench.attach({
  socket: "/run/user/1000/meshbench.sock",
  timeoutMs: 5000,       // how long to wait for the connection itself
  callTimeoutMs: 20000,  // the default budget for every call() on it
});

// A single call known to take longer gets its own budget, or none:
await wb.call("firmware.build", { checkout: "main" }, 20 * 60_000);
await wb.call("sim.run", { for_ms: 10 * 60_000 }, null); // no timeout
```

`callTimeoutMs` defaults to 300000 (five minutes), matching the Python
client's socket timeout, so a script ported between the two waits the same
length of time before it hears about a verb the workbench never answered.

## What kind of client this is

Go and Python generate `Kind`, `Board`, `Preset`, `Role`, `Class`, `Tab`,
`Strategy` and `Transport` from `internal/world/scenario` (`tools/clientgen`),
so a typo in a board name is a compile error or an editor squiggle rather than
a verb refusing three calls later. This client does not, on purpose: it has no
compiler and no build step to catch the typo before it reaches the wire either
way, so a generated enum here would buy an import to keep in sync and nothing
a plain string does not already give a Node script. Every verb parameter is a
free string, and the workbench itself is still the thing that refuses a board
it has never heard of - the same refusal Go and Python get, just discovered at
the call rather than before it.

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
[what it does not do](https://meshbench.github.io/docs/what-it-does-not-do.html).
