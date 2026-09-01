// Tests for the Node client, run against a fake workbench: a unix-socket server
// that speaks the control protocol (one JSON request per line, one reply). No
// real workbench and no network - `node --test pkg/client-js`.

import { test } from "node:test";
import assert from "node:assert/strict";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import fs from "node:fs";

import {
  Workbench, WorkbenchError, ProtocolMismatch, VersionMismatch,
  PROTOCOL, RELEASE, defaultAddress,
} from "./meshbench.mjs";

// A fake workbench. `reply` maps a method to a function of its params
// returning {result} or {error, code}; unknown methods get a coded error.
// Returning `false` means never answer, for the timeout tests. It records
// every request it saw. Answers out of order on purpose, to prove the client
// matches replies by id rather than by arrival - except session.hello, which
// attach() sends before a caller has any other request in flight to pair it
// with, so buffering it out of order would deadlock attach() itself.
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
        const r = reply(req.method, req.params);
        if (r === false) continue; // simulate a verb the workbench never answers
        const frame = JSON.stringify({ id: req.id, ...(r || {}) }) + "\n";
        if (outOfOrder && req.method !== "session.hello") pending.push(frame);
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

/** A reply function that answers the handshake with a matching protocol and
 *  falls back to another function for everything else. */
function withHello(rest) {
  return (method, params) => {
    if (method === "session.hello") return { result: { protocol: PROTOCOL } };
    return rest(method, params);
  };
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
  const wb = await fakeWorkbench(withHello((method, params) => {
    if (method === "nodes.place") return { result: { placed: params.name } };
    return { error: `no such verb ${method}`, code: "no-verb" };
  }));
  const c = await Workbench.attach({ socket: wb.sockPath });
  const got = await c.call("nodes.place", { name: "Alpha", lat: 56.3 });
  assert.deepEqual(got, { placed: "Alpha" });
  const req = wb.seen.find((r) => r.method === "nodes.place");
  assert.deepEqual(req.params, { name: "Alpha", lat: 56.3 });
  await c.close();
  wb.server.close();
});

test("params is omitted when none is given", async () => {
  const wb = await fakeWorkbench(withHello(() => ({ result: "ok" })));
  const c = await Workbench.attach({ socket: wb.sockPath });
  await c.call("sim.state");
  const req = wb.seen.find((r) => r.method === "sim.state");
  assert.ok(!("params" in req), "params should be absent");
  await c.close();
  wb.server.close();
});

test("an error reply rejects with the code preserved", async () => {
  const wb = await fakeWorkbench(withHello(() => ({ error: "no node named X", code: "no-node" })));
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
    withHello((method) => ({ result: method })),
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
  await assert.rejects(Workbench.attach({ socket: wb.sockPath }), (e) => {
    assert.ok(e instanceof ProtocolMismatch);
    assert.equal(e.code, "protocol_mismatch");
    assert.equal(e.workbench, 99);
    return true;
  });
  wb.server.close();
});

// The pairing rule, from this end. A client and the workbench it drives must be
// the same release, and a script has to be able to tell that refusal from a
// verb declining what it was asked - hence its own class rather than a code on
// the general one.
test("attach declares the release this client belongs to", async () => {
  const wb = await fakeWorkbench(withHello(() => ({ result: "ok" })));
  const c = await Workbench.attach({ socket: wb.sockPath });
  assert.equal(wb.seen[0].protocol, PROTOCOL);
  assert.equal(wb.seen[0].release, RELEASE);
  await c.close();
  wb.server.close();
});

test("a workbench from another release is refused at connect", async () => {
  const wb = await fakeWorkbench(() => ({
    result: { protocol: PROTOCOL, release: "9.9.9", version: "v9.9.9" },
  }));
  await assert.rejects(Workbench.attach({ socket: wb.sockPath }), (e) => {
    assert.ok(e instanceof VersionMismatch);
    assert.equal(e.code, "version_mismatch");
    assert.equal(e.workbench, "9.9.9");
    // Both releases and what to do about it: a bare "version mismatch" leaves
    // a reader to work out which of the two things they have installed to
    // change, and they cannot act until they know.
    assert.match(e.message, new RegExp(`MeshBench ${RELEASE}`));
    assert.match(e.message, /MeshBench 9\.9\.9/);
    assert.match(e.message, /must be the same release/);
    return true;
  });
  wb.server.close();
});

// The workbench refuses the pair itself, on the frame the client declared it
// on, so this client has to report that refusal as the mismatch it is rather
// than as session.hello failing.
test("the workbench's own refusal arrives as a version mismatch", async () => {
  const wb = await fakeWorkbench(() => ({
    error: "this client is from MeshBench 1.0.0 and this workbench is MeshBench 2.0.0",
    code: "version_mismatch",
  }));
  await assert.rejects(Workbench.attach({ socket: wb.sockPath }), (e) => {
    assert.ok(e instanceof VersionMismatch);
    assert.match(e.message, /MeshBench 2\.0\.0/);
    return true;
  });
  wb.server.close();
});

// On loopback TCP the workbench refuses the token line itself, before any
// request exists to answer, so the refusal comes back with id 0 and no request
// of its own. Dropping it as "not a reply" turned a sentence naming both
// releases into "connection closed".
test("a refusal that answers no request is not dropped", async () => {
  const sockPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "mb-")), "c.sock");
  const server = net.createServer((c) => {
    c.write(JSON.stringify({
      id: 0,
      error: "this client is from MeshBench 1.0.0 and this workbench is MeshBench 2.0.0",
      code: "version_mismatch",
    }) + "\n");
    c.end();
  });
  await new Promise((r) => server.listen(sockPath, r));
  await assert.rejects(Workbench.attach({ socket: sockPath }), (e) => {
    assert.ok(e instanceof VersionMismatch);
    assert.match(e.message, /MeshBench 2\.0\.0/);
    return true;
  });
  server.close();
});

test("a workbench that is a development build is served, and says so", async () => {
  const wb = await fakeWorkbench(withHello(() => ({ result: "ok" })));
  const c = await Workbench.attach({ socket: wb.sockPath });
  assert.match(c.versionCheck, /release check skipped/);
  assert.match(c.versionCheck, /development build/);
  await c.close();
  wb.server.close();
});

test("attach folds the handshake in, so a script gets protocol protection unasked", async () => {
  const wb = await fakeWorkbench(withHello(() => ({ result: "ok" })));
  const c = await Workbench.attach({ socket: wb.sockPath });
  assert.equal(wb.seen[0].method, "session.hello");
  await c.close();
  wb.server.close();
});

test("a unix socket path over the limit is refused before it is dialled", async () => {
  const long = "/tmp/" + "x".repeat(200) + "/control.sock";
  await assert.rejects(Workbench.attach({ socket: long }), (e) => {
    assert.match(e.message, /may be at most 104/);
    return true;
  });
});

test("a call rejects on its own timeout, without blocking calls behind it", async () => {
  const wb = await fakeWorkbench(withHello((method) => {
    if (method === "stuck") return false; // the workbench never answers this one
    return { result: "ok" };
  }));
  const c = await Workbench.attach({ socket: wb.sockPath });
  await assert.rejects(c.call("stuck", null, 50), (e) => {
    assert.match(e.message, /stuck did not answer within 50 ms/);
    return true;
  });
  // The connection is still good - only the call that timed out gave up.
  await c.call("sim.state");
  await c.close();
  wb.server.close();
});

test("call's default timeout matches attach's callTimeoutMs", async () => {
  const wb = await fakeWorkbench(withHello((method) => {
    if (method === "stuck") return false;
    return { result: "ok" };
  }));
  const c = await Workbench.attach({ socket: wb.sockPath, callTimeoutMs: 50 });
  await assert.rejects(c.call("stuck"), (e) => {
    assert.match(e.message, /did not answer within 50 ms/);
    return true;
  });
  await c.close();
  wb.server.close();
});
