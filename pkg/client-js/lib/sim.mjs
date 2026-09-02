// The clock, and the run.

import { MeshbenchError } from "./errors.mjs";
import {
  FIRMWARE_WAIT_MS, JOB_WAIT_MS, RUN_WAIT_MS, secs, waitFor,
} from "./wait.mjs";

/** The clock, and the run. Live. */
export class Sim {
  constructor(wb) { this._wb = wb; }

  /** What the clock is doing. */
  async state() { return (await this._wb.call("sim.state")) || {}; }

  async playing() { return Boolean((await this.state()).playing); }

  async nowMs() { return (await this.state()).now_ms || 0; }

  /** Bring the run up: wait out the warm, start every node, and play.
   *
   *  Deliberately not one call to `sim.start`. That verb is the play button's
   *  own handler and answers four ways - it pauses if already playing, declines
   *  while links are being measured, or starts firmware and does not play - so
   *  a script pressing it once gets whichever of those the moment happens to be
   *  in.
   *
   *  Worse, it only starts firmware when no node is running. Pin a build onto
   *  two nodes of a fifty-eight node fixture and it considers the mesh started,
   *  plays with fifty-six of them down, and says nothing.
   *
   *  So this asks for the three things it actually wants, in order, and checks
   *  each one. */
  async start({ warmMs = JOB_WAIT_MS, firmwareMs = FIRMWARE_WAIT_MS } = {}) {
    // The links first. Nothing that follows means anything against a matrix
    // that is still being measured.
    await this._wb.waitIdle(warmMs);
    // Idle is not the same as measured. A warm that stopped to ask permission
    // to download terrain finishes its own job row, so the wait above returns
    // in a moment having waited for nothing: no link was measured, and every
    // study after this would answer over free space.
    const held = await this.state();
    if (held.warm_held) {
      const note = (held.ground || {}).note ||
        "call terrain.allow to answer the question either way";
      throw new MeshbenchError(
        "the link measurement is held: no terrain has been downloaded and no " +
        `link has been measured. ${note}`);
    }
    // Then every node that is not up, which firmware.start does and sim.start
    // does only when none of them are.
    const st = await this._wb.firmware.state();
    if ((st.running || 0) < (st.nodes || 0)) {
      await this._wb.firmware.start();
      await this._wb.firmware.waitStarted(firmwareMs);
    }
    // Then the clock, by its own name. play cannot pause, which is the other
    // half of what made sim.start unusable from a script.
    if (!(await this.playing())) await this.play();
  }

  async play() { await this._wb.call("sim.play"); }

  async pause() { await this._wb.call("sim.pause"); }

  /** Press the play button as the window presses it, and answer as it answers.
   *
   *  Almost never what a script means - `start()` is - and here because the
   *  verb exists and a client that hid it would send somebody back to `call`. */
  async toggle() { return (await this._wb.call("sim.toggle")) || {}; }

  /** Advance one tick, which is `step_ms` of simulated time. */
  async step() { await this._wb.call("sim.step"); }

  /** Put the clock and the counters back to the start of the run. */
  async reset() { await this._wb.call("sim.reset"); }

  /** Step a paused run, which is how a command gets the time it needs to be
   *  answered without starting the clock. */
  async settle(steps = 60) { await this._wb.call("sim.settle", { steps }); }

  /** Fix the run. Same seed, same scenario, same result - which is what makes a
   *  changed result mean something. */
  async setSeed(seed) { await this._wb.call("sim.seed", { seed }); }

  /** How much simulated time one tick advances. */
  async setStepMs(ms) { await this._wb.call("sim.speed", { step_ms: ms }); }

  /** Whether play starts MeshCore on every node, or runs the channel with
   *  nothing behind it. */
  async setRealFirmware(on = true) { await this._wb.call("sim.kind", { real: on }); }

  /** Advance the mesh's own clock by this much, and wait for it.
   *
   *  Two clocks, one call, and they are not the same one. `simulatedMs` is the
   *  mesh's: five minutes here is five minutes of its time. `waitMs` is yours -
   *  how long you are prepared to sit here before giving up. On 155 emulated
   *  nodes five simulated minutes is a great deal more than five of yours,
   *  which is why the second is separate and generous. */
  async run(simulatedMs, { waitMs = RUN_WAIT_MS } = {}) {
    if (!(simulatedMs > 0)) {
      throw new MeshbenchError("run() needs a length in simulated milliseconds");
    }
    await this._wb.call("sim.run", { for_ms: simulatedMs });
    await this.waitStopped(waitMs);
  }

  /** Wait for a run to end. `timeoutMs` is wall clock. */
  waitStopped(timeoutMs = RUN_WAIT_MS) {
    return waitFor(async () => {
      const st = await this.state();
      if (!st.playing) return [true, ""];
      return [false, `${secs(st.now_ms || 0)} of simulated time`];
    }, timeoutMs, "the run to finish");
  }

  /** Wait for the mesh's clock to reach a moment. `atMs` is simulated time;
   *  `timeoutMs` is yours. */
  waitUntil(atMs, timeoutMs = RUN_WAIT_MS) {
    return waitFor(async () => {
      const st = await this.state();
      if ((st.now_ms || 0) >= atMs) return [true, ""];
      return [false, secs(st.now_ms || 0)];
    }, timeoutMs, `simulated time to reach ${secs(atMs)}`);
  }
}
