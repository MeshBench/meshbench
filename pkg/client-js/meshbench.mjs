// The MeshBench workbench, driven from Node.
//
// Speaks the same control socket as pkg/client-go and pkg/client-python, on
// the same machine - so a companion app or a dashboard in the JavaScript
// world drives a running workbench without shelling out to another language.
// Zero build and zero dependencies: it is one ES module on Node's own `net`,
// because a client that needed a framework to speak to a local socket would
// be a client nobody could debug.
//
// Deliberately thinner than the other two: Go and Python generate a façade of
// typed methods and closed enums from the tree (tools/clientgen); this one is
// `call(verb, params)` and nothing else. See pkg/client-js/README.md for why -
// briefly, a generated enum buys compile-time safety that a client with no
// compiler cannot spend.
//
//   import { Workbench } from "./meshbench.mjs";
//   const wb = await Workbench.attach();
//   await wb.call("nodes.place", { name: "Alpha", kind: "simple-repeater", lat: 56.3, lon: -3.3 });
//   await wb.close();
//
// `call` is the whole API; everything else is a shape over it. The workbench
// answers in order, so this sends one request at a time - the simplest correct
// thing, and nothing here needs the throughput of pipelining.
//
// `attach` does the protocol handshake itself and refuses a workbench this
// client cannot speak to, the same as the Go and Python clients - a script
// that does not remember to call `hello()` gets the protection anyway. Every
// call carries a timeout, so a verb the workbench never answers rejects
// instead of hanging the script forever.

import net from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/** The wire version this client speaks. A workbench answering anything else is
 *  refused rather than failing halfway through a script. */
export const PROTOCOL = 1;

/** How long a call waits for a reply before it gives up, unless a caller says
 *  otherwise. Matches the Python client's socket timeout, so a script ported
 *  between the two waits the same length of time before it hears about a
 *  verb the workbench never answered. */
export const DEFAULT_CALL_TIMEOUT_MS = 300000;

/** The shortest sun_path any platform we run on allows: 108 on Linux, 104 on
 *  macOS and the BSDs. Matches the Go and Python clients exactly. */
export const MAX_UNIX_PATH = 104;

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
  /** Carries the code beside the message rather than folding it into the
   *  prose, because a caller that has to match on prose breaks the day the
   *  workbench rewords a refusal. */
  constructor(message, code) {
    super(message);
    /** Always `WorkbenchError`, so a refusal is distinguishable from a
     *  connection or a programming fault in a log line that has only the name
     *  to go on. */
    this.name = "WorkbenchError";
    /** How the refusal was classified, so a caller can branch on it instead of
     *  on prose: the workbench's own code (`not_found`, `conflict`, `closing`
     *  and the rest the control socket defines) when a verb was refused, and
     *  `protocol` when this client was the end that refused, at the handshake.
     *  Empty when a refusal arrived without one, so test it rather than assume
     *  it is set. */
    this.code = code || "";
  }
}

/** One connection to a workbench, and the queue that keeps two callers from
 *  interleaving a half-frame on the wire. */
export class Workbench {
  /** Wraps a socket that is already connected. Scripts call `attach()` instead:
   *  a connection built here has not been through the handshake, so it would
   *  find out on some later verb that the workbench speaks a protocol this
   *  client does not.
   *  @param {net.Socket} sock @param {string} address @param {number} callTimeoutMs */
  constructor(sock, address, callTimeoutMs) {
    this._sock = sock;
    /** The address this connection was asked for, as the caller wrote it or as
     *  `defaultAddress()` chose it, so a script driving more than one workbench
     *  can say which of them refused. It is not re-read from the socket: with
     *  `tcp` it stays the word `tcp`, not the loopback port behind it. */
    this.address = address;
    this._callTimeoutMs = callTimeoutMs;
    this._nextId = 0;
    this._buf = "";
    this._waiters = []; // FIFO: the workbench answers in order.
    this._closed = null;
    sock.setEncoding("utf8");
    sock.on("data", (chunk) => this._onData(chunk));
    sock.on("error", (e) => this._fail(e));
    sock.on("close", () => this._fail(new Error("connection closed")));
  }

  /** Open a connection to a running workbench, and do the protocol handshake
   *  before handing it back - a workbench speaking a version this client does
   *  not understand is refused here, not on whichever call happens to notice.
   *  @param {{socket?: string, timeoutMs?: number, callTimeoutMs?: number}} [opts]
   *  @returns {Promise<Workbench>} */
  static attach(opts = {}) {
    const address = opts.socket || defaultAddress();
    const timeoutMs = opts.timeoutMs ?? DEFAULT_CALL_TIMEOUT_MS;
    const callTimeoutMs = opts.callTimeoutMs ?? DEFAULT_CALL_TIMEOUT_MS;
    const isTCP = address === "tcp" || address.startsWith("tcp:");
    if (!isTCP) {
      const p = address.startsWith("unix:") ? address.slice("unix:".length) : address;
      // Checked here rather than left to the connect call, which fails with a
      // raw ENAMETOOLONG that names neither the limit nor what to do about it.
      if (p.length > MAX_UNIX_PATH) {
        return Promise.reject(new Error(
          `${p} is ${p.length} bytes and a unix socket path may be at most ` +
          `${MAX_UNIX_PATH} - choose a shorter one, or use tcp`));
      }
    }
    return new Promise((resolve, reject) => {
      let sock;
      let token = "";
      if (isTCP) {
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
        const wb = new Workbench(sock, address, callTimeoutMs);
        wb.hello().then(
          () => resolve(wb),
          (err) => {
            sock.destroy();
            reject(err);
          });
      });
    });
  }

  /** Run one verb and return its result.
   *
   *  Rejects if the workbench has not answered within `timeoutMs` - the
   *  default is `DEFAULT_CALL_TIMEOUT_MS`, set at `attach()` via
   *  `callTimeoutMs`, the same length Python's socket timeout defaults to.
   *  Pass `null` to wait indefinitely for a call known to take a while.
   *  @param {string} verb @param {any} [params] @param {number|null} [timeoutMs]
   *  @returns {Promise<any>} */
  call(verb, params, timeoutMs) {
    if (this._closed) return Promise.reject(this._closed);
    const id = ++this._nextId;
    const req = { id, method: verb };
    if (params !== undefined && params !== null) req.params = params;
    const budget = timeoutMs === undefined ? this._callTimeoutMs : timeoutMs;
    return new Promise((resolve, reject) => {
      const waiter = { id, resolve, reject, timer: null };
      if (budget) {
        waiter.timer = setTimeout(() => {
          const i = this._waiters.indexOf(waiter);
          if (i >= 0) this._waiters.splice(i, 1);
          reject(new Error(`${verb} did not answer within ${budget} ms`));
        }, budget);
        // Never hold the process open only to time out a call nobody is
        // waiting on any more.
        if (typeof waiter.timer.unref === "function") waiter.timer.unref();
      }
      this._waiters.push(waiter);
      this._sock.write(JSON.stringify(req) + "\n");
    });
  }

  /** Ask the workbench what it is, and refuse a protocol this client does not
   *  speak. `attach()` calls this itself before handing back a connection, so
   *  calling it again is only useful to re-check. */
  async hello() {
    const h = await this.call("session.hello");
    if (h && h.protocol !== undefined && h.protocol !== PROTOCOL) {
      throw new WorkbenchError(
        `workbench speaks protocol ${h.protocol}, this client speaks ${PROTOCOL}`,
        "protocol");
    }
    return h;
  }

  /** Close the connection. Calls still in flight reject rather than wait on a
   *  socket nobody will answer on, so a script that closes early fails where it
   *  closed instead of hanging until its timeout. */
  close() {
    if (!this._closed) this._fail(new Error("connection closed by caller"));
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
      if (w.timer) clearTimeout(w.timer);
      if (msg.error) w.reject(new WorkbenchError(msg.error, msg.code));
      else w.resolve(msg.result);
    }
  }

  _fail(err) {
    if (this._closed) return;
    this._closed = err;
    const waiters = this._waiters;
    this._waiters = [];
    for (const w of waiters) {
      if (w.timer) clearTimeout(w.timer);
      w.reject(err);
    }
  }
}
