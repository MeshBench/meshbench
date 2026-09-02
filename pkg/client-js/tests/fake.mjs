// A fake workbench: a unix-socket server that speaks the control protocol - one
// JSON request per line, one reply. No real workbench and no network.
//
// In `tests/` rather than beside the test files under a `test/` directory,
// because node --test collects everything under a directory of that name and
// this is a helper, not a suite.

import net from "node:net";
import os from "node:os";
import path from "node:path";
import fs from "node:fs";

import { PROTOCOL } from "../meshbench.mjs";

/** Stand one up.
 *
 *  `reply` maps a method to a function of its params returning {result} or
 *  {error, code}; returning `false` means never answer, for the timeout tests.
 *  It records every request it saw.
 *
 *  Answers out of order when asked to, to prove the client matches replies by
 *  id rather than by arrival - except session.hello, which attach() sends
 *  before a caller has any other request in flight to pair it with, so
 *  buffering it out of order would deadlock attach() itself. */
export function fakeWorkbench(reply, { outOfOrder = false } = {}) {
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
        if (r === false) continue; // a verb the workbench never answers
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
export function withHello(rest, hello = {}) {
  return (method, params) => {
    if (method === "session.hello") {
      return { result: { protocol: PROTOCOL, mode: "headless", ...hello } };
    }
    return rest(method, params);
  };
}

/** A reply function built from a table of verb to result, which is what most of
 *  the facade tests want: they care what was asked and in what order, not about
 *  inventing a whole workbench. A verb not in the table is refused by name, so a
 *  helper that reached for one nobody expected fails loudly. */
export function answers(table, { hello = {}, onCall } = {}) {
  return withHello((method, params) => {
    if (onCall) onCall(method, params);
    if (!(method in table)) {
      return { error: `no such verb ${method}`, code: "unknown_verb" };
    }
    const v = table[method];
    const got = typeof v === "function" ? v(params) : v;
    if (got === false) return false;
    if (got && got.error) return got;
    return { result: got };
  }, hello);
}

/** Attach, run the body, and take everything down again - so a test that throws
 *  does not leave a listening socket behind for the next one to trip over. */
export async function withFake(reply, body, opts = {}) {
  const { Workbench } = await import("../meshbench.mjs");
  const wb = await fakeWorkbench(reply, opts);
  const client = await Workbench.attach({ socket: wb.sockPath });
  try {
    return await body(client, wb);
  } finally {
    await client.close();
    wb.server.close();
  }
}
