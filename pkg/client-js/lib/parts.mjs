// The rest of a scripted run: the project, what happened, and what a node said.

import { refusal } from "./errors.mjs";
import { Kind } from "./sets.mjs";
import { EVENT_WAIT_MS, JOB_WAIT_MS, waitFor } from "./wait.mjs";

/** Opening, saving, and starting over. Live. */
export class Project {
  constructor(wb) { this._wb = wb; }

  /** An empty network.
   *
   *  With a place it becomes the study area and the map is framed on it,
   *  because those are the same wish - and because a blank network with no
   *  place is a map in the middle of the Atlantic. */
  async new(place = "") {
    await this._wb.call("project.new", place ? { place } : {});
  }

  /** Load a fixture or a saved project. */
  async open(path) { await this._wb.call("project.open", path); }

  /** Write the current one out. Worth doing before anything that might restart
   *  the process: the scenario lives in the process, not on disk. */
  async save(name) {
    return ((await this._wb.call("project.save", { name })) || {}).path || "";
  }

  /** What has been saved. */
  async list() {
    return ((await this._wb.call("project.list")) || {}).projects || [];
  }
}

/** What the engine has done. Live. */
export class Events {
  constructor(wb) { this._wb = wb; }

  /** The tail - the events themselves, not a count of them.
   *
   *  A tail, and only a tail: the store keeps a bounded one because a long run
   *  has millions, so a script that needs all of them dumps per round rather
   *  than polling this. Reading only the tail after a busy flood samples the
   *  most congested moment of it, which is a mistake already made once here. */
  async recent(limit = 50) {
    return ((await this._wb.call("events.recent", { limit })) || {}).events || [];
  }

  /** How many there have been, which is the cheap question. */
  async total() {
    return ((await this._wb.call("events.recent", { limit: 1 })) || {}).total || 0;
  }

  /** Write every event held to a file, one JSON object per line. */
  async dump(path) {
    return ((await this._wb.call("events.dump", { path })) || {}).written || 0;
  }

  /** Wait for an event to match, and return it.
   *
   *  Empty fields match anything, so waiting for "any reception at Glenrothes"
   *  is `{kind: "rx", to: "Glenrothes"}` and not a predicate somebody has to
   *  write. */
  async wait({ kind = "", from = "", to = "", timeoutMs = EVENT_WAIT_MS } = {}) {
    const matches = (e) =>
      (!kind || e.kind === kind) && (!from || e.from === from) && (!to || e.to === to);
    let found = null;
    const want = [kind, from && `from ${from}`, to && `to ${to}`]
      .filter(Boolean).join(" ") || "anything";
    await waitFor(async () => {
      const evs = await this.recent(500);
      found = evs.find(matches) || null;
      if (found) return [true, ""];
      return [false, `${evs.length} events, none matching`];
    }, timeoutMs, `an event matching ${want}`);
    return found;
  }
}

/** One node's firmware console. Live.
 *
 *  Two consoles, not one, and which you get depends on what the node is.
 *
 *  A repeater has a text CLI and reads typed bytes. A companion does not: it
 *  speaks the framed companion protocol, and its command line is meshcore-cli's
 *  vocabulary - `advert`, `public <msg>`, `chan <n> <msg>`, and there is no
 *  `send`. Typing text at a companion goes nowhere, is echoed locally, and
 *  reads exactly like a command that ran and did nothing.
 *
 *  So this picks the right one from the node's kind. A caller should not have to
 *  know, and every caller that did know got it wrong at least once. */
export class Console {
  constructor(wb, node) {
    this._wb = wb;
    this.node = node;
  }

  /** Whether this node's console is the framed protocol.
   *
   *  A node this client cannot see is not one to guess about; the typed verb is
   *  the fallback and its refusal says so in its own words. */
  async _framed() {
    try {
      const info = await this._wb.nodes.info(this.node);
      return info.kind === Kind.COMPANION || info.kind === Kind.ROOM_SERVER;
    } catch {
      return false;
    }
  }

  /** Type a line at it. */
  async send(line) {
    const verb = (await this._framed()) ? "console.cli" : "console.type";
    await this._wb.call(verb, { node: this.node, command: line });
  }

  /** The scrollback, newest last - the lines themselves.
   *
   *  They come back under "tail" and "lines" is how many there are in total, so
   *  reading "lines" hands you a number where you asked for text. The tail is
   *  the last 200; a node up for an hour has thousands and nobody reads the
   *  first one. */
  async read() {
    return ((await this._wb.call("console.read", { node: this.node })) || {}).tail || [];
  }

  /** Send a line and wait for the node to answer it.
   *
   *  The important one. A node reads its serial input on its next loop and its
   *  loop only runs when the engine steps, so reading straight after sending
   *  reads the moment before the command was sent - every script that has done
   *  this by hand got an empty reply and concluded the console was broken. This
   *  gives the mesh its own time first, by stepping when the run is paused. */
  async ask(line, steps = 100) {
    const before = await this.read();
    await this.send(line);
    const sim = this._wb.sim;
    const st = await sim.state();
    if (st.playing) {
      // Already moving, so it will be answered on its own; give it the same
      // amount of the mesh's time a settle would.
      await sim.waitUntil((st.now_ms || 0) + steps * Math.max(st.step_ms || 1, 1),
        2 * 60_000);
    } else {
      await sim.settle(steps);
    }
    const after = await this.read();
    return after.length > before.length ? after.slice(before.length).join("\n") : "";
  }
}

/** A long operation the workbench is doing. Live: a handle to an id. */
export class Job {
  constructor(wb, id) {
    this._wb = wb;
    this.id = id;
  }

  /** Where this job has got to, or null when it is no longer listed - which
   *  means finished, because a job that has ended is removed. */
  async info() {
    return (await this._wb.jobs()).find((j) => j.id === this.id) || null;
  }

  /** Stop it, where whoever started it left a way to.
   *
   *  A job with no cancel refuses by name rather than silently doing nothing:
   *  an operator who asked deserves to be told, not left watching a bar that
   *  carries on. */
  async cancel() { await this._wb.call("job.cancel", { id: this.id }); }

  /** Wait for it to finish, and throw if it finished badly.
   *
   *  Ended is not the same as worked: a read that failed used to finish the job
   *  with the reason in its title and nothing else, so every caller either
   *  carried on as though it had succeeded or matched on the wording. */
  async wait(timeoutMs = JOB_WAIT_MS) {
    let last = null;
    await waitFor(async () => {
      const info = await this.info();
      if (info === null) return [true, ""];
      last = info;
      if (info.finished) return [true, ""];
      return [false, `${info.what}, ${info.done} of ${info.total}`];
    }, timeoutMs, `job ${this.id}`);
    if (last && last.failed) {
      throw refusal("job", `job ${this.id} failed: ${last.what}`, "internal");
    }
  }
}
