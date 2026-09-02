// Starting a workbench of one's own, and waiting for it to answer.
//
// Separate from the Workbench itself because it is process work rather than
// protocol work: which binary, which address, and how long to wait before
// deciding it is not coming.

import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { MeshbenchError } from "./errors.mjs";
import {
  BINARY_ENV, Connection, MAX_UNIX_PATH, RENDEZVOUS_ENV,
} from "./socket.mjs";

/** How long to give a workbench to answer before deciding it is not coming.
 *  A national fixture takes a while to open and a small one does not. */
export const START_TIMEOUT_MS = 90_000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** Start `meshbench <command>` and connect to it.
 *
 *  Resolves to the connection, the child, and the rendezvous file the child was
 *  told to write - which the connection had to be pointed at, because the
 *  per-user one names whatever else on this machine last left an address. */
export async function launch(command, {
  fixture = "", seed = 0, socket = "", binary = "", args = [],
  startTimeoutMs = START_TIMEOUT_MS, callTimeoutMs, stderr = "inherit",
} = {}) {
  const chosen = address(socket);
  const exe = binary || process.env[BINARY_ENV] || "meshbench";
  const argv = [command, "-control-socket", chosen.address];
  if (fixture) argv.push("-fixture", fixture);
  if (seed) argv.push("-seed", String(seed));
  argv.push(...args);

  const env = { ...process.env };
  if (chosen.rendezvous) env[RENDEZVOUS_ENV] = chosen.rendezvous;
  let child;
  try {
    child = spawn(exe, argv, { env, stdio: ["ignore", "inherit", stderr] });
  } catch (e) {
    throw new MeshbenchError(`could not start ${exe}: ${e.message}`);
  }
  let exited = null;
  child.on("error", (e) => { exited = e.message; });
  child.on("exit", (code) => { exited ??= `exited with ${code}`; });

  // Wait for the socket rather than for a fixed moment: a sleep long enough for
  // a national fixture is wasted on every run of a small one.
  const deadline = Date.now() + startTimeoutMs;
  for (;;) {
    if (exited) {
      throw new MeshbenchError(
        `${exe} ${command} ${exited} before answering at ${chosen.address}`);
    }
    try {
      const conn = await Connection.open({
        address: chosen.address, rendezvous: chosen.rendezvous,
        connectTimeoutMs: 1000, callTimeoutMs,
      });
      return { conn, child, rendezvous: chosen.rendezvous };
    } catch (e) {
      if (Date.now() > deadline) {
        child.kill("SIGKILL");
        throw new MeshbenchError(
          `${exe} ${command} did not answer at ${chosen.address} within ` +
          `${Math.round(startTimeoutMs / 1000)}s: ${e.message}`);
      }
      await sleep(50);
    }
  }
}

/** An address of its own unless one was named, so launching two of these does
 *  not have them fight over the per-user default.
 *
 *  Two reasons that path may not do. Windows has no unix socket a Node client
 *  can reach, and a temporary directory on macOS is long enough on its own to
 *  exceed sun_path. Either way, loopback - with a rendezvous file of its own
 *  too, or two sessions would overwrite each other's port and token in the
 *  per-user one. */
function address(named) {
  if (named) return { address: named, rendezvous: "" };
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "meshbench"));
  const sock = path.join(dir, "control.sock");
  if (process.platform === "win32" || sock.length > MAX_UNIX_PATH) {
    return { address: "tcp", rendezvous: path.join(dir, "control.json") };
  }
  return { address: sock, rendezvous: "" };
}

/** Stop a workbench this client started.
 *
 *  An interrupt asks the run to stop its firmware on the way out, which on
 *  fifty-eight emulated nodes is not instant. Windows has no SIGINT to send a
 *  process that is not sharing a console, so it is killed there and whatever
 *  the run was holding is the operating system's problem. */
export async function stop(child, graceMs = 20_000) {
  if (child.exitCode !== null || child.signalCode !== null) return;
  child.kill(process.platform === "win32" ? "SIGKILL" : "SIGINT");
  const deadline = Date.now() + graceMs;
  while (child.exitCode === null && child.signalCode === null) {
    if (Date.now() > deadline) {
      child.kill("SIGKILL");
      return;
    }
    await sleep(50);
  }
}
