// Which workbenches are running on this machine.
//
// `Workbench.attach()` goes to one address, which is enough while there is one
// session per user. Two runs side by side - a soak beside the workbench
// somebody is watching, two jobs on one CI runner - need a second address, and
// until this existed the only record of where it was lived in the head of
// whoever typed it.
//
// A module function rather than a method, because the question comes before a
// connection: a script asks what is running in order to decide what to attach
// to.
//
// # Telling a live session from what a dead one left behind
//
// A workbench killed with SIGKILL cannot clean up after itself, and neither
// obvious check survives that. A unix socket file outlives the process that
// bound it; a pid is reused, so a pid that exists today may name somebody
// else's program. Both would report a dead session as running. So the check is
// a connect to the address itself, which is the same check the workbench makes
// before it takes an address, and the leftover file is removed when nothing
// answers. A session's own tidying up shortens this directory; it is not what
// makes the answer right.
//
// Windows works the same way: there the address is a loopback host and port and
// the check is a TCP connect. Nothing here is unix-only.

import fs from "node:fs";
import path from "node:path";

import { Connection, SESSIONS_ENV, cacheDir } from "./socket.mjs";

/** How long a live session is given to describe itself. Generous, because the
 *  answer is not worth a wrong row: a session in the middle of something slow is
 *  still running and is listed either way, with its description missing. */
export const DETAIL_WAIT_MS = 2000;

/** The per-user directory the session files live in. */
export function sessionsDir() {
  return process.env[SESSIONS_ENV] || path.join(cacheDir(), "sessions");
}

/** The workbenches running on this machine, oldest first.
 *
 *  A session that has died is not listed, however it died, and what it left
 *  behind is removed on the way past.
 *
 *  Each row is `{address, pid, startedAt, token, version, mode, project,
 *  nodes}`. Pass one to `Workbench.attach({session})`: that is the way to reach
 *  a second TCP session, whose token sits beside its address in its own file
 *  where the per-user rendezvous file two of them share has only one. */
export async function sessions() {
  const dir = sessionsDir();
  let entries;
  try {
    entries = fs.readdirSync(dir).filter((f) => f.endsWith(".json")).sort();
  } catch {
    return []; // no directory is no sessions, not a failure to report
  }
  const found = await Promise.all(entries.map(async (name) => {
    const file = path.join(dir, name);
    const row = read(file);
    if (row === null) return null;
    const detail = await describe(row);
    if (detail === null) {
      // Nothing is answering there, so nothing is running there.
      try { fs.rmSync(file); } catch { /* somebody else got there first */ }
      return null;
    }
    return { ...row, ...detail };
  }));
  // Oldest first, so two runs listed twice come back in the same order. The
  // timestamps are RFC 3339 written by one program on one machine, so the text
  // sorts in time order, and the address settles a tie whatever happens.
  return found.filter(Boolean).sort((a, b) =>
    (a.startedAt + a.address).localeCompare(b.startedAt + b.address));
}

function read(file) {
  let got;
  try {
    got = JSON.parse(fs.readFileSync(file, "utf8"));
  } catch {
    // A file this package cannot read is a file it did not write, and refusing
    // to list anything because of one would make the whole answer hostage to it.
    return null;
  }
  if (!got.address) return null;
  return {
    address: got.address,
    pid: got.pid || 0,
    startedAt: String(got.started_at || ""),
    token: String(got.token || ""),
  };
}

/** One connection, both answers: whether anything is there, and what it is
 *  running. A connection that opens is a live session whether or not it finds a
 *  moment to describe itself. */
async function describe(row) {
  let conn;
  try {
    conn = await Connection.open({
      address: row.address, token: row.token,
      connectTimeoutMs: DETAIL_WAIT_MS, callTimeoutMs: DETAIL_WAIT_MS,
    });
  } catch {
    return null;
  }
  try {
    const got = (await conn.call("session.hello")) || {};
    return {
      version: String(got.version || ""),
      mode: String(got.mode || ""),
      project: String(got.project || ""),
      nodes: got.nodes || 0,
    };
  } catch {
    return {};
  } finally {
    conn.close();
  }
}
