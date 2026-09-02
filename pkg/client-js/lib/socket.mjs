// The wire: one JSON request per line, one reply.
//
// Everything above this is shape. This is the whole protocol, and it is small
// on purpose - a client that needed a framework to speak to a local socket
// would be a client nobody could debug.
//
// Two transports, because one does not travel:
//
// - A unix socket, where the operating system has one. The filesystem is the
//   access control, and the kernel enforces it.
// - Loopback TCP with a token, where it does not. The workbench binds 127.0.0.1
//   on an ephemeral port and writes the address and a 128-bit token to a 0600
//   file; this reads that file and presents the token before anything else.
//
// The choice is by operating system, not by language, so all three clients
// speak the same thing on the same machine.

import net from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { PROTOCOL, RELEASE } from "./pairing.mjs";
import { MeshbenchError, refusal } from "./errors.mjs";

/** Chooses where the workbench answers: a path, or "tcp", or "tcp:host:port". */
export const SOCKET_ENV = "MESHBENCH_CONTROL_SOCKET";

/** What to run when a client is asked to start a workbench and nothing named a
 *  binary. A checkout has one built but not installed, and every example and
 *  every test then needs the same three lines to find it. */
export const BINARY_ENV = "MESHBENCH_BINARY";

/** Chooses the file a TCP listener writes its address and token to. Per user by
 *  default, which is wrong for two runs at once - the second would overwrite
 *  the first's - so a client that starts a workbench gives it one of its own. */
export const RENDEZVOUS_ENV = "MESHBENCH_CONTROL_RENDEZVOUS";

/** Chooses the directory the session files live in, for a test or a CI job that
 *  wants a registry of its own rather than the user's. */
export const SESSIONS_ENV = "MESHBENCH_CONTROL_SESSIONS";

/** How long a call waits for a reply before it gives up, unless a caller says
 *  otherwise. Matches the Python client's socket timeout, so a script ported
 *  between the two waits the same length of time before it hears about a verb
 *  the workbench never answered. */
export const DEFAULT_CALL_TIMEOUT_MS = 300000;

/** The shortest sun_path any platform we run on allows: 108 on Linux, 104 on
 *  macOS and the BSDs. Matches the Go and Python clients exactly. */
export const MAX_UNIX_PATH = 104;

/** The per-user cache directory this OS already defines - the same one the Go
 *  and Python clients use, so all three read one rendezvous file. */
export function cacheDir() {
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

/** Where a TCP listener leaves its address and token. A named path wins over
 *  the environment, so a client that started a workbench with a rendezvous of
 *  its own reads that one rather than whatever else on this machine left one. */
export function rendezvousPath(named) {
  return named || process.env[RENDEZVOUS_ENV] || path.join(cacheDir(), "control.json");
}

/** The loopback address and token a workbench wrote for itself. */
export function readRendezvous(named) {
  const p = rendezvousPath(named);
  let raw;
  try {
    raw = fs.readFileSync(p, "utf8");
  } catch (e) {
    throw new MeshbenchError(`no workbench has left an address at ${p}: ${e.message}`);
  }
  let got;
  try {
    got = JSON.parse(raw);
  } catch (e) {
    throw new MeshbenchError(`${p} is not readable as an address: ${e.message}`);
  }
  return { address: got.address || "", token: got.token || "" };
}

/** The socket an address names, and the token to present on it. Not connected:
 *  the two decisions are separated so a caller can be refused for a path too
 *  long for sun_path before anything is dialled, which the raw OS error names
 *  neither the limit for nor what to do about. */
function plan(address, token, rendezvous) {
  if (address === "tcp" || address.startsWith("tcp:")) {
    let hostPort;
    if (address === "tcp") {
      const got = readRendezvous(rendezvous);
      hostPort = got.address;
      token = token || got.token;
    } else {
      hostPort = address.slice("tcp:".length);
      if (!hostPort.includes(":")) hostPort = "127.0.0.1:" + hostPort;
      // A port somebody named still needs the token, and without one in hand
      // the rendezvous file is the only place it exists.
      if (!token) token = readRendezvous(rendezvous).token;
    }
    const i = hostPort.lastIndexOf(":");
    return {
      connect: { host: hostPort.slice(0, i) || "127.0.0.1", port: Number(hostPort.slice(i + 1)) },
      token,
    };
  }
  const p = address.startsWith("unix:") ? address.slice("unix:".length) : address;
  if (p.length > MAX_UNIX_PATH) {
    throw new MeshbenchError(
      `${p} is ${p.length} bytes and a unix socket path may be at most ` +
      `${MAX_UNIX_PATH} - choose a shorter one, or use tcp`);
  }
  return { connect: { path: p }, token: "" };
}

/** One connection to a workbench, and the queue that keeps two callers from
 *  interleaving a half-frame on the wire.
 *
 *  The protocol has request ids but the workbench answers in order, so the
 *  simplest correct thing is one call at a time. Replies are still matched by
 *  id, because a client that trusted the order would be wrong the day the
 *  server stopped keeping it. */
export class Connection {
  constructor(sock, address, callTimeoutMs) {
    this._sock = sock;
    /** The address this connection was asked for, as the caller wrote it or as
     *  `defaultAddress()` chose it, so a script driving more than one workbench
     *  can say which of them refused. It is not re-read from the socket: with
     *  `tcp` it stays the word `tcp`, not the loopback port behind it. */
    this.address = address;
    /** Called with every frame that carries no id, which is how the socket says
     *  "this is not a reply". Set by a subscription; a request/reply client
     *  leaves it null and the frames are dropped. */
    this.onNotification = null;
    this._callTimeoutMs = callTimeoutMs;
    this._nextId = 0;
    this._buf = "";
    this._waiters = [];
    this._closed = null;
    sock.setEncoding("utf8");
    sock.on("data", (chunk) => this._onData(chunk));
    sock.on("error", (e) => this._fail(new MeshbenchError(e.message)));
    sock.on("close", () => this._fail(new MeshbenchError("connection closed")));
  }

  /** Open one, declaring nothing: the handshake is the Workbench's business,
   *  because a session probe wants a socket and not a paired client. */
  static open({ address, connectTimeoutMs = DEFAULT_CALL_TIMEOUT_MS,
    callTimeoutMs = DEFAULT_CALL_TIMEOUT_MS, token = "", rendezvous = "" } = {}) {
    const where = address || defaultAddress();
    let made;
    try {
      made = plan(where, token, rendezvous);
    } catch (e) {
      return Promise.reject(e);
    }
    return new Promise((resolve, reject) => {
      const sock = net.connect(made.connect);
      const timer = setTimeout(() => {
        sock.destroy();
        reject(new MeshbenchError(
          `timed out connecting to ${where} after ${connectTimeoutMs} ms`));
      }, connectTimeoutMs);
      sock.once("error", (e) => {
        clearTimeout(timer);
        reject(new MeshbenchError(`connecting to ${where}: ${e.message}`));
      });
      sock.once("connect", () => {
        clearTimeout(timer);
        sock.removeAllListeners("error");
        // The token first, before anything else on the wire, where the OS has
        // no unix socket to stand as the access control. Unix skips it, and
        // declares the same two things on its first request instead.
        if (made.token) {
          sock.write(JSON.stringify(
            { token: made.token, protocol: PROTOCOL, release: RELEASE }) + "\n");
        }
        resolve(new Connection(sock, where, callTimeoutMs));
      });
    });
  }

  /** Send one verb and resolve with its result, or reject with the refusal.
   *
   *  Pass `null` for `timeoutMs` to wait indefinitely, for a call known to take
   *  a while; anything else uses the connection's own budget. */
  call(verb, params, timeoutMs) {
    if (this._closed) return Promise.reject(this._closed);
    const id = ++this._nextId;
    const req = { id, method: verb };
    if (id === 1) {
      // Declared on the frame this client was already sending, so a workbench
      // that cannot serve this client refuses before any verb runs and without
      // a round trip of its own. Only the first: neither answer can change
      // while the connection is open.
      req.protocol = PROTOCOL;
      req.release = RELEASE;
    }
    if (params !== undefined && params !== null) req.params = params;
    const budget = timeoutMs === undefined ? this._callTimeoutMs : timeoutMs;
    return new Promise((resolve, reject) => {
      const waiter = { id, verb, resolve, reject, timer: null };
      if (budget) {
        waiter.timer = setTimeout(() => {
          const i = this._waiters.indexOf(waiter);
          if (i >= 0) this._waiters.splice(i, 1);
          reject(new MeshbenchError(`${verb} did not answer within ${budget} ms`));
        }, budget);
        // Never hold the process open only to time out a call nobody is
        // waiting on any more.
        if (typeof waiter.timer.unref === "function") waiter.timer.unref();
      }
      this._waiters.push(waiter);
      this._sock.write(JSON.stringify(req) + "\n");
    });
  }

  /** Hang up. Calls still in flight reject rather than wait on a socket nobody
   *  will answer on, so a script that closes early fails where it closed
   *  instead of hanging until its timeout. */
  close() {
    if (!this._closed) this._fail(new MeshbenchError("connection closed by caller"));
    this._sock.end();
    this._sock.destroy();
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
      this._deliver(msg);
    }
  }

  _deliver(msg) {
    // A frame that answers no request and carries an error is the connection
    // itself being refused: the token line on loopback TCP is turned away that
    // way, before any request exists to answer, so the refusal comes back with
    // id 0. Dropping it turned a sentence naming both releases into "connection
    // closed", which is the confusion the declaration exists to end. Failed
    // rather than delivered to a waiter, because there is no connection left to
    // make a second call on.
    if (!msg.id && msg.error) {
      this._fail(refusal("", msg.error, msg.code));
      return;
    }
    if (msg.id === undefined || msg.id === null) {
      if (this.onNotification) this.onNotification(msg);
      return;
    }
    const i = this._waiters.findIndex((w) => w.id === msg.id);
    if (i < 0) return;
    const [w] = this._waiters.splice(i, 1);
    if (w.timer) clearTimeout(w.timer);
    if (msg.error) w.reject(refusal(w.verb, msg.error, msg.code));
    else w.resolve(msg.result);
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
    if (this.onNotification) this.onNotification(null);
  }
}

/** Whether something is already answering there.
 *
 *  A connect rather than a stat: a socket file existing says nothing about
 *  whether anybody is behind it, and that difference is the whole question. */
export async function isLive(address, token = "", rendezvous = "") {
  try {
    const conn = await Connection.open(
      { address, token, rendezvous, connectTimeoutMs: 250 });
    conn.close();
    return true;
  } catch {
    return false;
  }
}
