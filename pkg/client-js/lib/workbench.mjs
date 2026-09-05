// The connection, and everything hanging off it.

import {
  ProtocolMismatch, Unavailable, VersionMismatch, asMismatch,
} from "./errors.mjs";
import { PROTOCOL, RELEASE, pairedRelease, pairingNote } from "./pairing.mjs";
import { Connection, DEFAULT_CALL_TIMEOUT_MS, defaultAddress } from "./socket.mjs";
import { launch, stop } from "./launch.mjs";
import { JOB_WAIT_MS, waitFor } from "./wait.mjs";
import { Provenance } from "./values.mjs";
import { Assertions, Schedule } from "./checks.mjs";
import { Boundary } from "./boundary.mjs";
import { Live } from "./live.mjs";
import { Nodes, Node } from "./nodes.mjs";
import { Console, Events, Job, Project } from "./parts.mjs";
import { Firmware } from "./firmware.mjs";
import { Sim } from "./sim.mjs";
import { Subscription } from "./subscribe.mjs";

/** A running session.
 *
 *  `launch` and `headless` own the process they started and stop it on the way
 *  out; `attach` never does - a script must not be able to close the workbench
 *  somebody is looking at by falling off the end of a function. */
export class Workbench {
  constructor(conn, child = null) {
    this._conn = conn;
    this._child = child;
    // What this connection is talking to, read at connect. Kept private and
    // re-asked by hello(), where the Go and Python clients hold it as a field:
    // a method is what JavaScript has for something worth reading again, and
    // hello() was already this client's public way of re-checking.
    this._hello = {};
    /** What became of the release check at connect: empty when the two ends
     *  compared equal, and a sentence naming what was skipped and why when one
     *  of them was not a release build. */
    this.versionCheck = "";
  }

  // ---- connecting ------------------------------------------------------

  /** Connect to a workbench that is already running, and do the handshake
   *  before handing it back.
   *
   *  Takes an address, or a row from `sessions()`. A row is the way to reach a
   *  second TCP session: its token sits beside its address in its own file,
   *  where the per-user rendezvous file two of them share has only one.
   *
   *  There is deliberately no "attach to whatever is running". Where several
   *  are up and none was named, guessing is how a script ends up driving the
   *  session somebody else was watching. */
  static async attach(opts = {}) {
    const from = typeof opts === "string" ? { socket: opts } : opts;
    const session = from.session;
    const conn = await Connection.open({
      address: session ? session.address : (from.socket || defaultAddress()),
      token: session ? session.token : "",
      rendezvous: from.rendezvous || "",
      connectTimeoutMs: from.timeoutMs ?? DEFAULT_CALL_TIMEOUT_MS,
      callTimeoutMs: from.callTimeoutMs ?? DEFAULT_CALL_TIMEOUT_MS,
    });
    return Workbench._greet(new Workbench(conn));
  }

  /** Start a session with no window, and own it.
   *
   *  The one to use from a test or from CI: no display, no GPU, no toolkit. */
  static headless(opts = {}) {
    return Workbench._spawn("headless", opts);
  }

  /** Open the desktop workbench and own it. Needs a display. */
  static launch(opts = {}) {
    return Workbench._spawn("workbench", opts);
  }

  /** Use the session that is running, or start one with no window.
   *
   *  For a script somebody runs repeatedly by hand: the second run carries on
   *  from the first rather than clearing everything down. Note which half you
   *  got - `ownsProcess` says - because attaching leaves the session running at
   *  the end and starting one does not. */
  static attachOrHeadless(opts = {}) {
    return Workbench._attachOr(Workbench.headless, opts);
  }

  /** The windowed half of the pair, so a re-run can put something back on
   *  screen. Needs a display. */
  static attachOrLaunch(opts = {}) {
    return Workbench._attachOr(Workbench.launch, opts);
  }

  static async _attachOr(start, opts) {
    // The session that is started is started at the address that was just
    // tried, which is the whole point of the pair. launch() and headless()
    // called directly invent a private address so two of them do not fight over
    // the per-user default; inheriting that here made attachOr useless - every
    // run failed to attach, started a session somewhere nobody would look
    // again, and the next run did the same.
    const named = { ...opts, socket: opts.socket || defaultAddress() };
    try {
      return await Workbench.attach(named);
    } catch (e) {
      if (e instanceof ProtocolMismatch || e instanceof VersionMismatch) throw e;
      return start(named);
    }
  }

  static async _spawn(command, opts) {
    const { conn, child } = await launch(command, opts);
    let wb;
    try {
      wb = await Workbench._greet(new Workbench(conn, child));
    } catch (e) {
      conn.close();
      await stop(child);
      throw e;
    }
    if (opts.fixture) {
      // The socket answers before the fixture is open. The windowed build loads
      // it on a worker so the window appears first, so a client can connect,
      // ask what is going on, and be told nothing - an empty job list, no
      // nodes, and a waitIdle that returns instantly having waited for work
      // that had not been queued yet.
      try {
        await wb.waitForNodes(opts.startTimeoutMs);
      } catch (e) {
        await wb.close();
        throw e;
      }
    }
    return wb;
  }

  static async _greet(wb) {
    try {
      await wb.hello();
    } catch (e) {
      wb._conn.close();
      throw e;
    }
    return wb;
  }

  /** Ask the workbench what it is, and refuse a build this client may not
   *  drive: a protocol it does not speak, or a release it was not shipped with.
   *  Connecting calls this itself, so calling it again is only useful to
   *  re-check.
   *
   *  Refused at both ends. The workbench has already turned away a version it
   *  will not serve, on the frame this client declared it on, so the
   *  comparisons here look redundant. They are not: a workbench old enough to
   *  predate the declaration ignores it and serves the connection anyway, and
   *  this end is then the only one left that can notice. */
  async hello() {
    let h;
    try {
      h = await this.call("session.hello");
    } catch (e) {
      throw asMismatch(e);
    }
    this._hello = h || {};
    if (this._hello.protocol !== undefined && this._hello.protocol !== PROTOCOL) {
      throw new ProtocolMismatch(
        `this client speaks control protocol ${PROTOCOL} and the workbench at ` +
        `${this._conn.address} speaks ${this._hello.protocol}. Upgrade whichever is older`,
        { workbench: this._hello.protocol });
    }
    const theirs = this._hello.release || "";
    if (!pairedRelease(RELEASE, theirs)) {
      throw new VersionMismatch(
        `this client is from MeshBench ${RELEASE} and this workbench is ` +
        `MeshBench ${theirs}. A client and the workbench it drives must be the ` +
        `same release: install the ${theirs} client, or run the ${RELEASE} workbench`,
        { workbench: theirs });
    }
    this.versionCheck = pairingNote(RELEASE, theirs);
    return this._hello;
  }

  // ---- lifetime --------------------------------------------------------

  /** Where this connection was made, as the caller wrote it. */
  get address() { return this._conn.address; }

  /** Whether closing this will stop the workbench, or only hang up on it. */
  get ownsProcess() { return this._child !== null; }

  /** Whether this session has no interface, so a caller can check once rather
   *  than learn it from a dozen refusals. */
  get isHeadless() { return this._hello.mode === "headless"; }

  /** Hang up, and stop the process if this client started it. */
  async close() {
    this._conn.close();
    if (this._child) await stop(this._child);
  }

  // ---- the wire --------------------------------------------------------

  /** Run one verb and return its result.
   *
   *  Public and documented, not an escape hatch to be ashamed of: the shaped
   *  API will never cover every verb the socket answers, and a verb added
   *  tomorrow should be usable today. Ask `verbs()` for the list this build
   *  actually offers.
   *
   *  Pass `null` for `timeoutMs` to wait indefinitely on a call known to take a
   *  while. */
  call(verb, params, timeoutMs) {
    return this._conn.call(verb, params, timeoutMs);
  }

  /** Stream server-pushed notifications for the given topics, rather than
   *  polling. Opens a second connection to this same workbench, so closing the
   *  returned Subscription hangs up only that stream.
   *
   *  Topics today: "status" (a new console line) and "snapshot" (a compact
   *  summary after each publish, coalesced by the server so a busy run cannot
   *  flood a slow reader). */
  subscribe(...topics) {
    return Subscription.open(this._conn.address, topics);
  }

  /** The whole session as the socket summarises it. */
  async snapshot() { return (await this.call("session.snapshot")) || {}; }

  /** The cheap summary: nodes, seed, time, whether it is playing. */
  async describe() { return (await this.call("session.describe")) || {}; }

  /** Every command this workbench has been driven with, newest last, and when
   *  the process started - so a session picked up cold can be told how the
   *  world got here, and whether it has been restarted. */
  async journal() { return (await this.call("session.journal")) || {}; }

  /** Every method this build answers. */
  async verbs() { return ((await this.call("session.verbs")) || {}).verbs || []; }

  /** Leave a line in the session's log, for whoever is watching. */
  async say(text) { await this.call("ui.said", text); }

  /** Freeze the whole session under a name - the network, how it is being run,
   *  and where the clock had got to - so it can be taken back here. */
  async checkpoint(name) {
    return (await this.call("session.checkpoint", { name })) || {};
  }

  /** Rebuild a checkpoint and replay to the moment it was taken. Returns as
   *  soon as the replay is under way; the sim reaching `target_ms` is when it
   *  has actually arrived. Deterministic, so it comes back to exactly where it
   *  was, at the cost of the replay taking the run's own time. */
  async restore(name) {
    return (await this.call("session.restore", { name })) || {};
  }

  /** What can be restored, by name. */
  async checkpoints() {
    return ((await this.call("session.checkpoints")) || {}).checkpoints || [];
  }

  /** What else is running on this machine, this session included.
   *
   *  The same list `sessions()` reads from disk, asked of the workbench
   *  instead. Two differences: the row for this session has `self` set and
   *  describes itself from the inside, and no row carries a token, because a
   *  token belongs in the 0600 file it came from and not in a reply. So these
   *  rows are for choosing by; pass one from `sessions()` to `attach`. */
  async sessions() {
    return ((await this.call("session.list")) || {}).sessions || [];
  }

  /** Whether a panel opened in its own window stays above the main one. Reads
   *  the preference when called with nothing, sets it when given a value, and
   *  returns what it now is.
   *
   *  The preference exists for Linux under Wayland, where no client may ask a
   *  normal window to stay above others. What can be asked for is a layer-shell
   *  surface, and that is a different kind of window: no title bar, no taskbar
   *  entry and no minimise, so the window draws its own bar. On macOS and
   *  Windows always-on-top costs nothing and the preference does not apply. */
  async keepAbove(on) {
    const got = await this.call("ui.keep_above", on === undefined ? {} : { on });
    return (got || {}).on ?? true;
  }

  /** Open a node's own window, on a named tab, and return the tab it opened on.
   *
   *  Windowed sessions only, and it says so here rather than appearing to work:
   *  a headless run has nothing to open, and a script that "opened the Hardware
   *  tab" in CI and saw no error will be written to assume it did. */
  async window(node, tab = "") {
    if (this.isHeadless) {
      throw new Unavailable("node.window",
        "this session has no interface attached, so there is nothing to show",
        "unavailable");
    }
    const got = await this.call("node.window", { node: String(node), tab });
    return (got || {}).tab || "";
  }

  /** Open a node's bring-up window and return the board it checks.
   *
   *  What the board's profile declares, beside what the firmware left in the
   *  chip, and where the two differ. Windowed sessions only, for the same
   *  reason window() is, and refused for a node running a host build: there is
   *  no board to check it against. */
  async bringup(node) {
    if (this.isHeadless) {
      throw new Unavailable("node.bringup",
        "this session has no interface attached, so there is nothing to show",
        "unavailable");
    }
    const got = await this.call("node.bringup", { node: String(node) });
    return (got || {}).board || "";
  }

  // ---- the shape -------------------------------------------------------

  get nodes() { return new Nodes(this); }
  get sim() { return new Sim(this); }
  get project() { return new Project(this); }
  get firmware() { return new Firmware(this); }
  get events() { return new Events(this); }
  get schedule() { return new Schedule(this); }
  get assertions() { return new Assertions(this); }

  /** The study area: which nodes are in the question being asked. Set it before
   *  importing, because the import filters at fetch time. */
  get boundary() { return new Boundary(this); }

  /** A live deployment feed - CoreScope and the rest - and the import chain
   *  that brings one in. */
  get live() { return new Live(this); }

  /** A handle, without checking it exists - so one can be named before it is
   *  placed. Every method on it will say so if it does not. */
  node(name) { return new Node(this, name); }

  console(node) { return new Console(this, String(node)); }

  job(id) { return new Job(this, id); }

  /** Everything long-running that is in flight. */
  async jobs() { return (await this.snapshot()).jobs || []; }

  /** Sample every node and return what it found - the rows, not a count of
   *  them.
   *
   *  A sample, not a read: it costs a /proc read per node, which is why the
   *  window only does it while somebody is looking at the panel. */
  async nodeStats() {
    return ((await this.call("nodes.stats")) || {}).stats || [];
  }

  /** What this session's measurements are being made under.
   *
   *  Read from the session rather than carried on each result, for now: the
   *  verbs do not return it yet, and inventing it here would be a claim this
   *  client is not entitled to make. */
  async provenance() { return new Provenance(await this.snapshot()); }

  // ---- waiting ---------------------------------------------------------

  /** Wait until the session has a network in it.
   *
   *  For a fixture opened at startup, which happens on a worker: the socket
   *  answers first, so everything asked before the open lands describes an
   *  empty session and is believed. */
  waitForNodes(timeoutMs = JOB_WAIT_MS) {
    return waitFor(async () => {
      const n = (await this.describe()).nodes || 0;
      return n ? [true, ""] : [false, "no nodes yet"];
    }, timeoutMs, "the fixture to open");
  }

  /** Wait for every job to finish - the honest way to wait out a warm, which is
   *  what most of them are.
   *
   *  Finished jobs are ignored rather than waited for: some are removed when
   *  they end and some are only marked - infer.run's is marked - so waiting for
   *  the list to empty waits for ever on half of them. That is a difference
   *  between the verbs, and not a caller's to know about. */
  waitIdle(timeoutMs = JOB_WAIT_MS) {
    return waitFor(async () => {
      const running = (await this.jobs()).filter((j) => !j.finished);
      if (running.length === 0) return [true, ""];
      const first = running[0];
      return [false, `${running.length} still running, first is ` +
        `"${first.what}" (${first.done} of ${first.total})`];
    }, timeoutMs, "the workbench to go idle");
  }
}
