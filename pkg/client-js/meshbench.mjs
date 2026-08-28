// The MeshBench workbench, driven from Node.
//
// The peer of pkg/client-go and pkg/client-python, speaking the same control
// socket on the same machine - so a companion app or a dashboard in the
// JavaScript world drives a running workbench without shelling out to another
// language. Zero build and zero dependencies: it is one ES module on Node's own
// `net`, because a client that needed a framework to speak to a local socket
// would be a client nobody could debug.
//
//   import { Workbench } from "./meshbench.mjs";
//   const wb = await Workbench.attach();
//   await wb.call("nodes.place", { name: "Alpha", kind: "simple-repeater", lat: 56.3, lon: -3.3 });
//   await wb.close();
//
// `call` is the whole API; everything else is a shape over it. The workbench
// answers in order, so this sends one request at a time - the simplest correct
// thing, and nothing here needs the throughput of pipelining.

import net from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/** The wire version this client speaks. A workbench answering anything else is
 *  refused rather than failing halfway through a script. */
export const PROTOCOL = 1;

const SOCKET_ENV = "MESHBENCH_CONTROL_SOCKET";
const RENDEZVOUS_ENV = "MESHBENCH_CONTROL_RENDEZVOUS";

/** The per-user cache directory this OS already defines - the same one the Go
 *  and Python clients use, so all three read one rendezvous file. */
function cacheDir() {
  let base;
  if (process.platform === "win32") base = process.env.LOCALAPPDATA || os.homedir();
  else if (process.platform === "darwin") base = path.join(os.homedir(), "Library", "Caches");
  else base = process.env.XDG_CACHE_HOME || path.join(os.homedir(), ".cache");
  return path.join(base, "meshbench");
}

/** Where a workbench answers on this operating system unless told otherwise.
 *  Matches the Go and Python clients exactly, because the choice is by OS, not
 *  by language: all three must name the same address on one machine. */
export function defaultAddress() {
  const env = process.env[SOCKET_ENV];
  if (env) return env;
  if (process.platform === "win32") return "tcp"; // no AF_UNIX on Windows
  const runtime = process.env.XDG_RUNTIME_DIR;
  if (runtime) return path.join(runtime, "meshbench.sock");
  return path.join(cacheDir(), "control.sock");
}

/** Where a TCP listener leaves its address and token. */
function rendezvousPath() {
  return process.env[RENDEZVOUS_ENV] || path.join(cacheDir(), "control.json");
}

/** The loopback address and token a Windows workbench wrote for itself. */
function readRendezvous() {
  const p = rendezvousPath();
  let raw;
  try {
    raw = fs.readFileSync(p, "utf8");
  } catch (e) {
    throw new Error(`no workbench has left an address at ${p}: ${e.message}`);
  }
  let got;
  try {
    got = JSON.parse(raw);
  } catch (e) {
    throw new Error(`${p} is not readable as an address: ${e.message}`);
  }
  return { address: got.address || "", token: got.token || "" };
}

/** A verb the workbench refused, carrying its classification so a caller can
 *  tell "no such node" from "the workbench is closing" without matching prose. */
export class WorkbenchError extends Error {
  constructor(message, code) {
    super(message);
    this.name = "WorkbenchError";
    this.code = code || "";
  }
}

/** One connection to a workbench, and the queue that keeps two callers from
 *  interleaving a half-frame on the wire. */
export class Workbench {
  /** @param {net.Socket} sock @param {string} address */
  constructor(sock, address) {
    this._sock = sock;
    this.address = address;
    this._nextId = 0;
    this._buf = "";
    this._waiters = []; // FIFO: the workbench answers in order.
    this._closed = null;
    sock.setEncoding("utf8");
    sock.on("data", (chunk) => this._onData(chunk));
    sock.on("error", (e) => this._fail(e));
    sock.on("close", () => this._fail(new Error("connection closed")));
  }

  /** Open a connection to a running workbench.
   *  @param {{socket?: string, timeoutMs?: number}} [opts]
   *  @returns {Promise<Workbench>} */
  static attach(opts = {}) {
    const address = opts.socket || defaultAddress();
    const timeoutMs = opts.timeoutMs ?? 300000;
    return new Promise((resolve, reject) => {
      let sock;
      let token = "";
      if (address === "tcp" || address.startsWith("tcp:")) {
        let hostPort;
        if (address === "tcp") ({ address: hostPort, token } = readRendezvous());
        else {
          hostPort = address.slice("tcp:".length);
          if (!hostPort.includes(":")) hostPort = "127.0.0.1:" + hostPort;
          ({ token } = readRendezvous()); // a named port still needs the token
        }
        const i = hostPort.lastIndexOf(":");
        const host = hostPort.slice(0, i) || "127.0.0.1";
        const port = Number(hostPort.slice(i + 1));
        sock = net.connect({ host, port });
      } else {
        const p = address.startsWith("unix:") ? address.slice("unix:".length) : address;
        sock = net.connect({ path: p });
      }
      const timer = setTimeout(() => {
        sock.destroy();
        reject(new Error(`timed out connecting to ${address} after ${timeoutMs} ms`));
      }, timeoutMs);
      sock.once("error", (e) => {
        clearTimeout(timer);
        reject(new Error(`connecting to ${address}: ${e.message}`));
      });
      sock.once("connect", () => {
        clearTimeout(timer);
        sock.removeAllListeners("error");
        // The token first, before anything else on the wire, where the OS has
        // no unix socket to stand as the access control. Unix skips it.
        if (token) sock.write(JSON.stringify({ token }) + "\n");
        resolve(new Workbench(sock, address));
      });
    });
  }

  /** Run one verb and return its result.
   *  @param {string} verb @param {any} [params] @returns {Promise<any>} */
  call(verb, params) {
    if (this._closed) return Promise.reject(this._closed);
    const id = ++this._nextId;
    const req = { id, method: verb };
    if (params !== undefined && params !== null) req.params = params;
    return new Promise((resolve, reject) => {
      this._waiters.push({ id, resolve, reject });
      this._sock.write(JSON.stringify(req) + "\n");
    });
  }

  /** Ask the workbench what it is, and refuse a protocol this client does not
   *  speak - at connect rather than halfway through a script. */
  async hello() {
    const h = await this.call("session.hello");
    if (h && h.protocol !== undefined && h.protocol !== PROTOCOL) {
      throw new WorkbenchError(
        `workbench speaks protocol ${h.protocol}, this client speaks ${PROTOCOL}`,
        "protocol");
    }
    return h;
  }

  /** Close the connection. Any calls still in flight reject. */
  close() {
    if (!this._closed) this._fail(new Error("connection closed by caller"), true);
    this._sock.end();
    return Promise.resolve();
  }

  _onData(chunk) {
    this._buf += chunk;
    let nl;
    while ((nl = this._buf.indexOf("\n")) >= 0) {
      const line = this._buf.slice(0, nl);
      this._buf = this._buf.slice(nl + 1);
      if (line.trim() === "") continue;
      let msg;
      try {
        msg = JSON.parse(line);
      } catch {
        continue; // a frame this client cannot parse is not a reply to fail on
      }
      // Notifications (from session.subscribe) carry no id; this request/reply
      // client does not subscribe, so anything without one is ignored.
      if (msg.id === undefined || msg.id === null) continue;
      const i = this._waiters.findIndex((w) => w.id === msg.id);
      if (i < 0) continue;
      const [w] = this._waiters.splice(i, 1);
      if (msg.error) w.reject(new WorkbenchError(msg.error, msg.code));
      else w.resolve(msg.result);
    }
  }

  _fail(err, quiet) {
    if (this._closed) return;
    this._closed = err;
    const waiters = this._waiters;
    this._waiters = [];
    if (!quiet) for (const w of waiters) w.reject(err);
    else for (const w of waiters) w.reject(err);
  }
}
