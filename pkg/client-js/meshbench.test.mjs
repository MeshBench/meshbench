// Tests for the Node client, run against a fake workbench: a unix-socket server
// that speaks the control protocol (one JSON request per line, one reply). No
// real workbench and no network - `node --test pkg/client-js`.

import { test } from "node:test";
import assert from "node:assert/strict";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import fs from "node:fs";

import { Workbench, WorkbenchError, defaultAddress } from "./meshbench.mjs";

// A fake workbench. `reply` maps a method to a function of its params returning
// {result} or {error, code}; unknown methods get a coded error. It records
// every request it saw. Answers out of order on purpose, to prove the client
// matches replies by id rather than by arrival.
function fakeWorkbench(reply, { outOfOrder = false } = {}) {
  const sockPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "mb-")), "c.sock");
  const seen = [];
  const server = net.createServer((c) => {
    c.setEncoding("utf8");
    let buf = "";
    const pending = [];
    c.on("data", (chunk) => {
      buf += chunk;
      let nl;
      while ((nl = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, nl);
        buf = buf.slice(nl + 1);
        if (!line.trim()) continue;
        const req = JSON.parse(line);
        if (req.token !== undefined) continue; // the TCP auth line, if any
        seen.push(req);
        const r = reply(req.method, req.params) || {};
        const frame = JSON.stringify({ id: req.id, ...r }) + "\n";
        if (outOfOrder) pending.push(frame);
        else c.write(frame);
      }
      if (outOfOrder && pending.length === 2) {
        c.write(pending.pop()); // second request answered first
        c.write(pending.pop());
      }
    });
  });
  return new Promise((resolve) => {
    server.listen(sockPath, () => resolve({ sockPath, server, seen }));
  });
}

test("defaultAddress honours the env override", () => {
  const saved = process.env.MESHBENCH_CONTROL_SOCKET;
  process.env.MESHBENCH_CONTROL_SOCKET = "/tmp/whatever.sock";
  try {
    assert.equal(defaultAddress(), "/tmp/whatever.sock");
  } finally {
    if (saved === undefined) delete process.env.MESHBENCH_CONTROL_SOCKET;
    else process.env.MESHBENCH_CONTROL_SOCKET = saved;
  }
});

test("call sends {id, method, params} and returns the result", async () => {
  const wb = await fakeWorkbench((method, params) => {
    if (method === "nodes.place") return { result: { placed: params.name } };
    return { error: `no such verb ${method}`, code: "no-verb" };
  });
  const c = await Workbench.attach({ socket: wb.sockPath });
  const got = await c.call("nodes.place", { name: "Alpha", lat: 56.3 });
  assert.deepEqual(got, { placed: "Alpha" });
  assert.equal(wb.seen[0].method, "nodes.place");
  assert.deepEqual(wb.seen[0].params, { name: "Alpha", lat: 56.3 });
  assert.equal(wb.seen[0].id, 1);
  await c.close();
  wb.server.close();
});

test("params is omitted when none is given", async () => {
  const wb = await fakeWorkbench(() => ({ result: "ok" }));
  const c = await Workbench.attach({ socket: wb.sockPath });
  await c.call("sim.state");
  assert.ok(!("params" in wb.seen[0]), "params should be absent");
  await c.close();
  wb.server.close();
});

test("an error reply rejects with the code preserved", async () => {
  const wb = await fakeWorkbench(() => ({ error: "no node named X", code: "no-node" }));
  const c = await Workbench.attach({ socket: wb.sockPath });
  await assert.rejects(c.call("node.output", { node: "X" }), (e) => {
    assert.ok(e instanceof WorkbenchError);
    assert.equal(e.code, "no-node");
    assert.match(e.message, /no node named X/);
    return true;
  });
  await c.close();
  wb.server.close();
});

test("replies are matched by id, not by arrival order", async () => {
  const wb = await fakeWorkbench(
    (method) => ({ result: method }),
    { outOfOrder: true });
  const c = await Workbench.attach({ socket: wb.sockPath });
  const [a, b] = await Promise.all([c.call("first"), c.call("second")]);
  assert.equal(a, "first");
  assert.equal(b, "second");
  await c.close();
  wb.server.close();
});

test("hello refuses a protocol this client does not speak", async () => {
  const wb = await fakeWorkbench(() => ({ result: { protocol: 99, version: "x" } }));
  const c = await Workbench.attach({ socket: wb.sockPath });
  await assert.rejects(c.hello(), (e) => e.code === "protocol");
  await c.close();
  wb.server.close();
});
