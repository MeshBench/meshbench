// Waiting, in one place.
//
// Every wait in this package is a method, never a sleep in a script. That is
// not tidiness: tools/soak hand-wrote the same poll loop three times in
// seventy-two lines, each with its own interval and its own timeout, and its
// own header records having sampled the wrong moment because of it.
//
// They poll. When the socket learns to push, this file changes and no caller
// does - which is the whole reason the clients are built before the events.
//
// # Which clock
//
// Two clocks appear in this API and they are not the same one. Simulated is the
// mesh's own: `sim.run`, `schedule.add`, `sim.waitUntil`. Wall is yours: every
// `timeoutMs`. Both are milliseconds, so the name is what tells them apart, and
// nothing that means the mesh's clock is called a timeout.

import { Timeout } from "./errors.mjs";

/** Firmware coming up on a whole mesh. Real firmware is minutes; emulated
 *  boards are longer. */
export const FIRMWARE_WAIT_MS = 10 * 60_000;

/** A run of simulated time finishing, measured on your clock. */
export const RUN_WAIT_MS = 30 * 60_000;

/** A long job - a warm, a download, a build - finishing. */
export const JOB_WAIT_MS = 30 * 60_000;

/** One event arriving. */
export const EVENT_WAIT_MS = 5 * 60_000;

/** Where a wait's polling starts and the slowest it gets.
 *
 *  It backs off between the two. Something about to happen is noticed promptly;
 *  something that takes ten minutes is not asked four thousand times on the
 *  way. nodes.stats in particular costs a /proc read per node, so polling it at
 *  ten hertz on a 155-node mesh is fifteen hundred reads a second - during
 *  firmware startup, which is the busiest moment there is. */
export const POLL_FIRST_MS = 50;
export const POLL_SLOWEST_MS = 1000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** Poll until `check` says yes, or the time runs out.
 *
 *  `check` resolves to `[done, saw]`: whether it is finished and, if not, what
 *  it saw - which is what the Timeout reports. A rejection from `check` stops
 *  the wait rather than being retried: a verb refusing because a node does not
 *  exist will refuse the same way in ten seconds. */
export async function waitFor(check, timeoutMs, what) {
  const deadline = Date.now() + timeoutMs;
  let interval = POLL_FIRST_MS;
  let last = "";
  for (;;) {
    const [done, saw] = await check();
    if (done) return;
    if (saw) last = saw;
    if (Date.now() > deadline) throw new Timeout(what, timeoutMs, last);
    await sleep(interval);
    interval = Math.min(interval * 1.5, POLL_SLOWEST_MS);
  }
}

/** A simulated moment, said the way a person reads it. */
export function secs(ms) {
  return `${(ms / 1000).toFixed(1)}s`;
}
